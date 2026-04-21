package testsuite

import (
	"context"

	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
	"github.com/stretchr/testify/suite"
)

// PlanCommentThreadRepositorySuite tests PlanCommentThreadRepository implementations
type PlanCommentThreadRepositorySuite struct {
	suite.Suite
	Repo           repository.PlanCommentThreadRepository
	PlanDocRepo    repository.PlanDocumentRepository
	ProjectRepo    repository.ProjectRepository
	UserRepo       repository.UserRepository
	Cleanup        func()
	projectCreated map[string]string
	planDocCreated map[string]string
}

func (s *PlanCommentThreadRepositorySuite) createTestProject(gitRepoSuffix string) string {
	if s.ProjectRepo == nil {
		return ""
	}
	if s.projectCreated == nil {
		s.projectCreated = make(map[string]string)
	}
	if id, ok := s.projectCreated[gitRepoSuffix]; ok {
		return id
	}
	ctx := context.Background()
	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/test/" + gitRepoSuffix,
	}
	err := s.ProjectRepo.Create(ctx, project)
	if err != nil {
		return ""
	}
	s.projectCreated[gitRepoSuffix] = project.ID
	return project.ID
}

func (s *PlanCommentThreadRepositorySuite) createTestPlanDocument(planDocSuffix string) string {
	if s.PlanDocRepo == nil {
		return ""
	}
	if s.planDocCreated == nil {
		s.planDocCreated = make(map[string]string)
	}
	if id, ok := s.planDocCreated[planDocSuffix]; ok {
		return id
	}
	ctx := context.Background()
	projectID := s.createTestProject("project-" + planDocSuffix)
	if projectID == "" {
		return ""
	}
	doc := &domain.PlanDocument{
		ProjectID:   projectID,
		Description: "Test Plan " + planDocSuffix,
		Body:        "Test body content for " + planDocSuffix,
		Status:      domain.PlanDocumentStatusPlanning,
	}
	err := s.PlanDocRepo.Create(ctx, doc)
	if err != nil {
		return ""
	}
	s.planDocCreated[planDocSuffix] = doc.ID
	return doc.ID
}

func (s *PlanCommentThreadRepositorySuite) TearDownTest() {
	if s.Cleanup != nil {
		s.Cleanup()
	}
}

func (s *PlanCommentThreadRepositorySuite) TestCreate() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-create")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	thread := &domain.PlanCommentThread{
		PlanDocumentID: planDocID,
		TargetText:     "test target text",
		ContextBefore:  "before context",
		ContextAfter:   "after context",
		Status:         domain.PlanCommentThreadStatusActive,
	}

	err := s.Repo.Create(ctx, thread)
	s.Require().NoError(err)
	s.NotEmpty(thread.ID)
	s.False(thread.CreatedAt.IsZero())
	s.False(thread.UpdatedAt.IsZero())
}

func (s *PlanCommentThreadRepositorySuite) TestFindByID() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-findbyid")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	thread := &domain.PlanCommentThread{
		PlanDocumentID: planDocID,
		TargetText:     "findbyid target",
		Status:         domain.PlanCommentThreadStatusActive,
	}
	err := s.Repo.Create(ctx, thread)
	s.Require().NoError(err)

	found, err := s.Repo.FindByID(ctx, thread.ID)
	s.Require().NoError(err)
	s.Equal(thread.ID, found.ID)
	s.Equal(planDocID, found.PlanDocumentID)
	s.Equal("findbyid target", found.TargetText)
	s.Equal(domain.PlanCommentThreadStatusActive, found.Status)
}

func (s *PlanCommentThreadRepositorySuite) TestFindByID_NotFound() {
	ctx := context.Background()

	// Use valid UUID format for PostgreSQL compatibility
	found, err := s.Repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000")
	s.NoError(err)
	s.Nil(found)
}

func (s *PlanCommentThreadRepositorySuite) TestFindByPlanDocumentID() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-findbyplandoc")
	otherPlanDocID := s.createTestPlanDocument("thread-findbyplandoc-other")
	if planDocID == "" || otherPlanDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	// Create threads for target plan
	for i := 0; i < 3; i++ {
		thread := &domain.PlanCommentThread{
			PlanDocumentID: planDocID,
			TargetText:     "target " + string(rune('a'+i)),
			Status:         domain.PlanCommentThreadStatusActive,
		}
		err := s.Repo.Create(ctx, thread)
		s.Require().NoError(err)
	}

	// Create thread for other plan
	otherThread := &domain.PlanCommentThread{
		PlanDocumentID: otherPlanDocID,
		TargetText:     "other target",
		Status:         domain.PlanCommentThreadStatusActive,
	}
	err := s.Repo.Create(ctx, otherThread)
	s.Require().NoError(err)

	threads, err := s.Repo.FindByPlanDocumentID(ctx, planDocID, nil)
	s.Require().NoError(err)
	s.Len(threads, 3)

	for _, t := range threads {
		s.Equal(planDocID, t.PlanDocumentID)
	}
}

func (s *PlanCommentThreadRepositorySuite) TestFindByPlanDocumentID_WithStatusFilter() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-statusfilter")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	// Create threads with different statuses
	statuses := []domain.PlanCommentThreadStatus{
		domain.PlanCommentThreadStatusActive,
		domain.PlanCommentThreadStatusActive,
		domain.PlanCommentThreadStatusResolved,
		domain.PlanCommentThreadStatusOutdated,
	}
	for i, status := range statuses {
		thread := &domain.PlanCommentThread{
			PlanDocumentID: planDocID,
			TargetText:     "target " + string(rune('a'+i)),
			Status:         status,
		}
		err := s.Repo.Create(ctx, thread)
		s.Require().NoError(err)
	}

	// Filter by active status
	activeStatus := domain.PlanCommentThreadStatusActive
	activeThreads, err := s.Repo.FindByPlanDocumentID(ctx, planDocID, &activeStatus)
	s.Require().NoError(err)
	s.Len(activeThreads, 2)

	// Filter by resolved status
	resolvedStatus := domain.PlanCommentThreadStatusResolved
	resolvedThreads, err := s.Repo.FindByPlanDocumentID(ctx, planDocID, &resolvedStatus)
	s.Require().NoError(err)
	s.Len(resolvedThreads, 1)

	// Filter by outdated status
	outdatedStatus := domain.PlanCommentThreadStatusOutdated
	outdatedThreads, err := s.Repo.FindByPlanDocumentID(ctx, planDocID, &outdatedStatus)
	s.Require().NoError(err)
	s.Len(outdatedThreads, 1)
}

func (s *PlanCommentThreadRepositorySuite) TestCountActiveByPlanDocumentID() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-count-active")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	statuses := []domain.PlanCommentThreadStatus{
		domain.PlanCommentThreadStatusActive,
		domain.PlanCommentThreadStatusActive,
		domain.PlanCommentThreadStatusActive,
		domain.PlanCommentThreadStatusResolved,
		domain.PlanCommentThreadStatusOutdated,
	}
	for i, status := range statuses {
		thread := &domain.PlanCommentThread{
			PlanDocumentID: planDocID,
			TargetText:     "count target " + string(rune('a'+i)),
			Status:         status,
		}
		s.Require().NoError(s.Repo.Create(ctx, thread))
	}

	count, err := s.Repo.CountActiveByPlanDocumentID(ctx, planDocID)
	s.Require().NoError(err)
	s.Equal(3, count)
}

func (s *PlanCommentThreadRepositorySuite) TestCountActiveByPlanDocumentID_Empty() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-count-empty")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	count, err := s.Repo.CountActiveByPlanDocumentID(ctx, planDocID)
	s.Require().NoError(err)
	s.Equal(0, count)
}

func (s *PlanCommentThreadRepositorySuite) TestUpdate() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-update")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	thread := &domain.PlanCommentThread{
		PlanDocumentID: planDocID,
		TargetText:     "original target",
		Status:         domain.PlanCommentThreadStatusActive,
	}
	err := s.Repo.Create(ctx, thread)
	s.Require().NoError(err)

	// Update status
	thread.Status = domain.PlanCommentThreadStatusResolved
	err = s.Repo.Update(ctx, thread)
	s.Require().NoError(err)

	// Verify update
	found, err := s.Repo.FindByID(ctx, thread.ID)
	s.Require().NoError(err)
	s.Equal(domain.PlanCommentThreadStatusResolved, found.Status)
}

func (s *PlanCommentThreadRepositorySuite) TestDelete() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-delete")
	if planDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	thread := &domain.PlanCommentThread{
		PlanDocumentID: planDocID,
		TargetText:     "delete target",
		Status:         domain.PlanCommentThreadStatusActive,
	}
	err := s.Repo.Create(ctx, thread)
	s.Require().NoError(err)

	err = s.Repo.Delete(ctx, thread.ID)
	s.Require().NoError(err)

	found, err := s.Repo.FindByID(ctx, thread.ID)
	s.NoError(err)
	s.Nil(found)
}

func (s *PlanCommentThreadRepositorySuite) TestMarkOutdatedByPlanDocumentID() {
	ctx := context.Background()

	planDocID := s.createTestPlanDocument("thread-markoutdated")
	otherPlanDocID := s.createTestPlanDocument("thread-markoutdated-other")
	if planDocID == "" || otherPlanDocID == "" {
		s.T().Skip("PlanDocRepo not available, skipping test")
	}

	// Create active threads
	for i := 0; i < 3; i++ {
		thread := &domain.PlanCommentThread{
			PlanDocumentID: planDocID,
			TargetText:     "target " + string(rune('a'+i)),
			Status:         domain.PlanCommentThreadStatusActive,
		}
		err := s.Repo.Create(ctx, thread)
		s.Require().NoError(err)
	}

	// Create resolved thread (should not be affected)
	resolvedThread := &domain.PlanCommentThread{
		PlanDocumentID: planDocID,
		TargetText:     "resolved target",
		Status:         domain.PlanCommentThreadStatusResolved,
	}
	err := s.Repo.Create(ctx, resolvedThread)
	s.Require().NoError(err)

	// Create thread for other plan (should not be affected)
	otherThread := &domain.PlanCommentThread{
		PlanDocumentID: otherPlanDocID,
		TargetText:     "other target",
		Status:         domain.PlanCommentThreadStatusActive,
	}
	err = s.Repo.Create(ctx, otherThread)
	s.Require().NoError(err)

	// Mark outdated
	err = s.Repo.MarkOutdatedByPlanDocumentID(ctx, planDocID)
	s.Require().NoError(err)

	// Verify: active threads should be outdated
	outdatedStatus := domain.PlanCommentThreadStatusOutdated
	outdatedThreads, err := s.Repo.FindByPlanDocumentID(ctx, planDocID, &outdatedStatus)
	s.Require().NoError(err)
	s.Len(outdatedThreads, 3)

	// Verify: resolved thread should still be resolved
	resolvedStatus := domain.PlanCommentThreadStatusResolved
	resolvedThreads, err := s.Repo.FindByPlanDocumentID(ctx, planDocID, &resolvedStatus)
	s.Require().NoError(err)
	s.Len(resolvedThreads, 1)

	// Verify: other plan's thread should not be affected
	activeStatus := domain.PlanCommentThreadStatusActive
	otherActiveThreads, err := s.Repo.FindByPlanDocumentID(ctx, otherPlanDocID, &activeStatus)
	s.Require().NoError(err)
	s.Len(otherActiveThreads, 1)
}
