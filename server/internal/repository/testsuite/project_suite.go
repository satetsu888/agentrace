package testsuite

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
	"github.com/stretchr/testify/suite"
)

// ProjectRepositorySuite tests ProjectRepository implementations
type ProjectRepositorySuite struct {
	suite.Suite
	Repo    repository.ProjectRepository
	Repos   *repository.Repositories // Full repos for HasRelatedData tests
	Cleanup func()
}

func (s *ProjectRepositorySuite) SetupTest() {
	// Cleanup is optional - some implementations may need it
}

func (s *ProjectRepositorySuite) TearDownTest() {
	if s.Cleanup != nil {
		s.Cleanup()
	}
}

func (s *ProjectRepositorySuite) TestCreate() {
	ctx := context.Background()

	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/repo1",
	}

	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// ID should be auto-generated
	s.NotEmpty(project.ID)

	// CreatedAt should be set
	s.False(project.CreatedAt.IsZero())

	// Verify by finding
	found, err := s.Repo.FindByID(ctx, project.ID)
	s.Require().NoError(err)
	s.Require().NotNil(found)
	s.Equal(project.CanonicalGitRepository, found.CanonicalGitRepository)
}

func (s *ProjectRepositorySuite) TestCreate_WithID() {
	ctx := context.Background()

	customID := uuid.New().String()
	project := &domain.Project{
		ID:                     customID,
		CanonicalGitRepository: "https://github.com/example/repo2",
	}

	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// ID should remain as specified
	s.Equal(customID, project.ID)

	// Verify by finding
	found, err := s.Repo.FindByID(ctx, project.ID)
	s.Require().NoError(err)
	s.Require().NotNil(found)
	s.Equal(customID, found.ID)
}

func (s *ProjectRepositorySuite) TestFindByID() {
	ctx := context.Background()

	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/repo3",
	}
	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// Find existing
	found, err := s.Repo.FindByID(ctx, project.ID)
	s.Require().NoError(err)
	s.Require().NotNil(found)
	s.Equal(project.ID, found.ID)
	s.Equal(project.CanonicalGitRepository, found.CanonicalGitRepository)
}

func (s *ProjectRepositorySuite) TestFindByID_NotFound() {
	ctx := context.Background()

	// Find non-existing (use valid UUID format)
	nonExistingID := uuid.New().String()
	found, err := s.Repo.FindByID(ctx, nonExistingID)
	s.NoError(err)
	s.Nil(found)
}

func (s *ProjectRepositorySuite) TestFindByIDs() {
	ctx := context.Background()

	p1 := &domain.Project{CanonicalGitRepository: "https://github.com/example/batch1"}
	p2 := &domain.Project{CanonicalGitRepository: "https://github.com/example/batch2"}
	p3 := &domain.Project{CanonicalGitRepository: "https://github.com/example/batch3"}
	s.Require().NoError(s.Repo.Create(ctx, p1))
	s.Require().NoError(s.Repo.Create(ctx, p2))
	s.Require().NoError(s.Repo.Create(ctx, p3))

	result, err := s.Repo.FindByIDs(ctx, []string{p1.ID, p3.ID, p1.ID})
	s.Require().NoError(err)
	s.Len(result, 2)
	s.Equal(p1.CanonicalGitRepository, result[p1.ID].CanonicalGitRepository)
	s.Equal(p3.CanonicalGitRepository, result[p3.ID].CanonicalGitRepository)
	_, hasP2 := result[p2.ID]
	s.False(hasP2)
}

func (s *ProjectRepositorySuite) TestFindByIDs_Empty() {
	ctx := context.Background()

	result, err := s.Repo.FindByIDs(ctx, nil)
	s.Require().NoError(err)
	s.Empty(result)
}

func (s *ProjectRepositorySuite) TestFindByCanonicalGitRepository() {
	ctx := context.Background()

	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/repo4",
	}
	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// Find by canonical git repo
	found, err := s.Repo.FindByCanonicalGitRepository(ctx, "https://github.com/example/repo4")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	s.Equal(project.ID, found.ID)
}

func (s *ProjectRepositorySuite) TestFindByCanonicalGitRepository_NotFound() {
	ctx := context.Background()

	found, err := s.Repo.FindByCanonicalGitRepository(ctx, "https://github.com/non-existing/repo")
	s.NoError(err)
	s.Nil(found)
}

func (s *ProjectRepositorySuite) TestFindOrCreateByCanonicalGitRepository_Create() {
	ctx := context.Background()

	// Should create new project
	project, err := s.Repo.FindOrCreateByCanonicalGitRepository(ctx, "https://github.com/example/new-repo")
	s.Require().NoError(err)
	s.Require().NotNil(project)
	s.NotEmpty(project.ID)
	s.Equal("https://github.com/example/new-repo", project.CanonicalGitRepository)
}

func (s *ProjectRepositorySuite) TestFindOrCreateByCanonicalGitRepository_Find() {
	ctx := context.Background()

	// Create first
	original := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/existing-repo",
	}
	err := s.Repo.Create(ctx, original)
	s.Require().NoError(err)

	// FindOrCreate should return existing
	found, err := s.Repo.FindOrCreateByCanonicalGitRepository(ctx, "https://github.com/example/existing-repo")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	s.Equal(original.ID, found.ID)
}

func (s *ProjectRepositorySuite) TestFindAll() {
	ctx := context.Background()

	// Create multiple projects
	for i := 0; i < 5; i++ {
		project := &domain.Project{
			CanonicalGitRepository: "https://github.com/example/findall-repo-" + string(rune('a'+i)),
		}
		time.Sleep(1 * time.Millisecond) // Ensure different CreatedAt
		err := s.Repo.Create(ctx, project)
		s.Require().NoError(err)
	}

	// Find all with limit (cursor-based pagination)
	projects, nextCursor, err := s.Repo.FindAll(ctx, 3, "")
	s.Require().NoError(err)
	s.Len(projects, 3)
	s.NotEmpty(nextCursor) // More items available

	// Verify order (newest first)
	for i := 0; i < len(projects)-1; i++ {
		s.True(projects[i].CreatedAt.After(projects[i+1].CreatedAt) || projects[i].CreatedAt.Equal(projects[i+1].CreatedAt))
	}
}

func (s *ProjectRepositorySuite) TestFindAll_WithCursor() {
	ctx := context.Background()

	// Create multiple projects
	for i := 0; i < 5; i++ {
		project := &domain.Project{
			CanonicalGitRepository: "https://github.com/example/cursor-repo-" + string(rune('a'+i)),
		}
		time.Sleep(1 * time.Millisecond)
		err := s.Repo.Create(ctx, project)
		s.Require().NoError(err)
	}

	// First page
	projects1, nextCursor, err := s.Repo.FindAll(ctx, 2, "")
	s.Require().NoError(err)
	s.Len(projects1, 2)
	s.NotEmpty(nextCursor)

	// Second page using cursor
	projects2, _, err := s.Repo.FindAll(ctx, 2, nextCursor)
	s.Require().NoError(err)
	s.Len(projects2, 2)

	// No overlap between pages
	for _, p1 := range projects1 {
		for _, p2 := range projects2 {
			s.NotEqual(p1.ID, p2.ID)
		}
	}
}

func (s *ProjectRepositorySuite) TestGetDefaultProject() {
	ctx := context.Background()

	defaultProject, err := s.Repo.GetDefaultProject(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(defaultProject)
	s.Equal(domain.DefaultProjectID, defaultProject.ID)
	s.Empty(defaultProject.CanonicalGitRepository)
}

func (s *ProjectRepositorySuite) TestDelete() {
	ctx := context.Background()

	// Create a project
	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/delete-repo",
	}
	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// Verify it exists
	found, err := s.Repo.FindByID(ctx, project.ID)
	s.Require().NoError(err)
	s.Require().NotNil(found)

	// Delete it
	err = s.Repo.Delete(ctx, project.ID)
	s.Require().NoError(err)

	// Verify it's gone
	found, err = s.Repo.FindByID(ctx, project.ID)
	s.NoError(err)
	s.Nil(found)
}

func (s *ProjectRepositorySuite) TestDelete_NonExistent() {
	ctx := context.Background()

	// Delete non-existent project should not error
	err := s.Repo.Delete(ctx, uuid.New().String())
	s.NoError(err)
}

func (s *ProjectRepositorySuite) TestHasRelatedData_NoData() {
	if s.Repos == nil {
		s.T().Skip("Repos not provided, skipping HasRelatedData test")
	}
	ctx := context.Background()

	// Create a project with no related data
	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/no-related-data",
	}
	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// Should have no related data
	hasData, err := s.Repo.HasRelatedData(ctx, project.ID)
	s.Require().NoError(err)
	s.False(hasData)
}

func (s *ProjectRepositorySuite) TestHasRelatedData_WithSession() {
	if s.Repos == nil {
		s.T().Skip("Repos not provided, skipping HasRelatedData test")
	}
	ctx := context.Background()

	// Create a project
	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/with-session",
	}
	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// Create a session linked to the project
	session := &domain.Session{
		ClaudeSessionID: uuid.New().String(),
		ProjectID:       project.ID,
	}
	err = s.Repos.Session.Create(ctx, session)
	s.Require().NoError(err)

	// Should have related data
	hasData, err := s.Repo.HasRelatedData(ctx, project.ID)
	s.Require().NoError(err)
	s.True(hasData)
}

func (s *ProjectRepositorySuite) TestHasRelatedData_WithPlanDocument() {
	if s.Repos == nil {
		s.T().Skip("Repos not provided, skipping HasRelatedData test")
	}
	ctx := context.Background()

	// Create a project
	project := &domain.Project{
		CanonicalGitRepository: "https://github.com/example/with-plan",
	}
	err := s.Repo.Create(ctx, project)
	s.Require().NoError(err)

	// Create a plan document linked to the project
	plan := &domain.PlanDocument{
		ProjectID:   project.ID,
		Description: "Test plan",
		Body:        "Test body",
	}
	err = s.Repos.PlanDocument.Create(ctx, plan)
	s.Require().NoError(err)

	// Should have related data
	hasData, err := s.Repo.HasRelatedData(ctx, project.ID)
	s.Require().NoError(err)
	s.True(hasData)
}
