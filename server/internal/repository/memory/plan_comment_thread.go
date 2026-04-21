package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/satetsu888/agentrace/server/internal/domain"
)

type PlanCommentThreadRepository struct {
	mu      sync.RWMutex
	threads map[string]*domain.PlanCommentThread
}

func NewPlanCommentThreadRepository() *PlanCommentThreadRepository {
	return &PlanCommentThreadRepository{
		threads: make(map[string]*domain.PlanCommentThread),
	}
}

func (r *PlanCommentThreadRepository) Create(ctx context.Context, thread *domain.PlanCommentThread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if thread.ID == "" {
		thread.ID = uuid.New().String()
	}
	now := time.Now()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	if thread.UpdatedAt.IsZero() {
		thread.UpdatedAt = now
	}
	if thread.Status == "" {
		thread.Status = domain.PlanCommentThreadStatusActive
	}

	r.threads[thread.ID] = thread
	return nil
}

func (r *PlanCommentThreadRepository) FindByID(ctx context.Context, id string) (*domain.PlanCommentThread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	thread, ok := r.threads[id]
	if !ok {
		return nil, nil
	}
	return thread, nil
}

func (r *PlanCommentThreadRepository) FindByPlanDocumentID(ctx context.Context, planDocumentID string, status *domain.PlanCommentThreadStatus) ([]*domain.PlanCommentThread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var threads []*domain.PlanCommentThread
	for _, t := range r.threads {
		if t.PlanDocumentID != planDocumentID {
			continue
		}
		if status != nil && t.Status != *status {
			continue
		}
		threads = append(threads, t)
	}

	// Sort by created_at ascending
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].CreatedAt.Before(threads[j].CreatedAt)
	})

	return threads, nil
}

func (r *PlanCommentThreadRepository) CountActiveByPlanDocumentID(ctx context.Context, planDocumentID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, t := range r.threads {
		if t.PlanDocumentID == planDocumentID && t.Status == domain.PlanCommentThreadStatusActive {
			count++
		}
	}
	return count, nil
}

func (r *PlanCommentThreadRepository) Update(ctx context.Context, thread *domain.PlanCommentThread) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.threads[thread.ID]; !ok {
		return nil
	}

	thread.UpdatedAt = time.Now()
	r.threads[thread.ID] = thread
	return nil
}

func (r *PlanCommentThreadRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.threads, id)
	return nil
}

func (r *PlanCommentThreadRepository) MarkOutdatedByPlanDocumentID(ctx context.Context, planDocumentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for _, t := range r.threads {
		if t.PlanDocumentID == planDocumentID && t.Status == domain.PlanCommentThreadStatusActive {
			t.Status = domain.PlanCommentThreadStatusOutdated
			t.UpdatedAt = now
		}
	}
	return nil
}
