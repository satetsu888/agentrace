package repository

import (
	"context"
	"errors"
	"time"

	"github.com/satetsu888/agentrace/server/internal/domain"
)

// ErrDuplicateEvent is returned when trying to create an event with a UUID that already exists
var ErrDuplicateEvent = errors.New("duplicate event: UUID already exists for this session")

// ProjectRepository はプロジェクトの永続化を担当する
type ProjectRepository interface {
	Create(ctx context.Context, project *domain.Project) error
	FindByID(ctx context.Context, id string) (*domain.Project, error)
	FindByIDs(ctx context.Context, ids []string) (map[string]*domain.Project, error)
	FindByCanonicalGitRepository(ctx context.Context, canonicalGitRepo string) (*domain.Project, error)
	FindOrCreateByCanonicalGitRepository(ctx context.Context, canonicalGitRepo string) (*domain.Project, error)
	FindAll(ctx context.Context, limit int, cursor string) ([]*domain.Project, string, error) // Returns (projects, nextCursor, error)
	GetDefaultProject(ctx context.Context) (*domain.Project, error)                           // CanonicalGitRepository が空のプロジェクト
	Delete(ctx context.Context, id string) error
	HasRelatedData(ctx context.Context, id string) (bool, error) // Returns true if project has sessions or plan documents
}

// SessionRepository はセッションの永続化を担当する
type SessionRepository interface {
	Create(ctx context.Context, session *domain.Session) error
	FindByID(ctx context.Context, id string) (*domain.Session, error)
	FindByClaudeSessionID(ctx context.Context, claudeSessionID string) (*domain.Session, error)
	Find(ctx context.Context, query domain.SessionQuery) ([]*domain.Session, string, error)  // Returns (sessions, nextCursor, error), excludes subagents
	FindSubagentsByParentID(ctx context.Context, parentID string) ([]*domain.Session, error) // Returns subagent sessions for a parent session
	FindOrCreateByClaudeSessionID(ctx context.Context, claudeSessionID string, userID *string) (*domain.Session, error)
	UpdateUserID(ctx context.Context, id string, userID string) error
	UpdateProjectPath(ctx context.Context, id string, projectPath string) error
	UpdateProjectID(ctx context.Context, id string, projectID string) error
	UpdateGitBranch(ctx context.Context, id string, gitBranch string) error
	UpdateTitle(ctx context.Context, id string, title string) error
	UpdateUpdatedAt(ctx context.Context, id string, updatedAt time.Time) error
	Delete(ctx context.Context, id string) error
}

// EventRepository はイベントの永続化を担当する
type EventRepository interface {
	Create(ctx context.Context, event *domain.Event) error
	// CreateBatch inserts multiple events efficiently. Duplicate events (same UUID
	// within a session) are silently skipped. Returns the number of newly inserted
	// events. Implementations should use a single transaction or batch operation
	// to avoid serial round-trips.
	CreateBatch(ctx context.Context, events []*domain.Event) (int, error)
	FindBySessionID(ctx context.Context, sessionID string) ([]*domain.Event, error)
	CountBySessionID(ctx context.Context, sessionID string) (int, error)
	DeleteBySessionID(ctx context.Context, sessionID string) error
}

// UserRepository はユーザーの永続化を担当する
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByIDs(ctx context.Context, ids []string) (map[string]*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindAll(ctx context.Context) ([]*domain.User, error)
	UpdateDisplayName(ctx context.Context, id string, displayName string) error
}

// APIKeyRepository はAPIキーの永続化を担当する
type APIKeyRepository interface {
	Create(ctx context.Context, key *domain.APIKey) error
	FindByKeyHash(ctx context.Context, keyHash string) (*domain.APIKey, error)
	FindByUserID(ctx context.Context, userID string) ([]*domain.APIKey, error)
	FindByID(ctx context.Context, id string) (*domain.APIKey, error)
	Delete(ctx context.Context, id string) error
	UpdateLastUsedAt(ctx context.Context, id string) error
}

// WebSessionRepository はWebセッションの永続化を担当する
type WebSessionRepository interface {
	Create(ctx context.Context, session *domain.WebSession) error
	FindByToken(ctx context.Context, token string) (*domain.WebSession, error)
	Delete(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
}

// PasswordCredentialRepository はパスワード認証情報の永続化を担当する
type PasswordCredentialRepository interface {
	Create(ctx context.Context, cred *domain.PasswordCredential) error
	FindByUserID(ctx context.Context, userID string) (*domain.PasswordCredential, error)
	Update(ctx context.Context, cred *domain.PasswordCredential) error
	Delete(ctx context.Context, id string) error
}

// OAuthConnectionRepository はOAuth連携の永続化を担当する
type OAuthConnectionRepository interface {
	Create(ctx context.Context, conn *domain.OAuthConnection) error
	FindByProviderAndProviderID(ctx context.Context, provider, providerID string) (*domain.OAuthConnection, error)
	FindByUserID(ctx context.Context, userID string) ([]*domain.OAuthConnection, error)
	Delete(ctx context.Context, id string) error
}

// PlanDocumentRepository はPlanDocumentの永続化を担当する
type PlanDocumentRepository interface {
	Create(ctx context.Context, doc *domain.PlanDocument) error
	FindByID(ctx context.Context, id string) (*domain.PlanDocument, error)
	Find(ctx context.Context, query domain.PlanDocumentQuery) ([]*domain.PlanDocument, string, error) // Returns (docs, nextCursor, error)
	Update(ctx context.Context, doc *domain.PlanDocument) error
	Delete(ctx context.Context, id string) error
	SetStatus(ctx context.Context, id string, status domain.PlanDocumentStatus) error
}

// PlanDocumentEventRepository はPlanDocumentEventの永続化を担当する
type PlanDocumentEventRepository interface {
	Create(ctx context.Context, event *domain.PlanDocumentEvent) error
	FindByPlanDocumentID(ctx context.Context, planDocumentID string) ([]*domain.PlanDocumentEvent, error)
	FindByClaudeSessionID(ctx context.Context, claudeSessionID string) ([]*domain.PlanDocumentEvent, error)
	GetCollaboratorUserIDs(ctx context.Context, planDocumentID string) ([]string, error)
	GetPlanDocumentIDsByUserIDs(ctx context.Context, userIDs []string) ([]string, error)
}

// UserFavoriteRepository はUserFavoriteの永続化を担当する
type UserFavoriteRepository interface {
	Create(ctx context.Context, favorite *domain.UserFavorite) error
	Delete(ctx context.Context, id string) error
	DeleteByUserAndTarget(ctx context.Context, userID string, targetType domain.UserFavoriteTargetType, targetID string) error
	FindByUserID(ctx context.Context, userID string) ([]*domain.UserFavorite, error)
	FindByUserAndTargetType(ctx context.Context, userID string, targetType domain.UserFavoriteTargetType) ([]*domain.UserFavorite, error)
	FindByUserAndTarget(ctx context.Context, userID string, targetType domain.UserFavoriteTargetType, targetID string) (*domain.UserFavorite, error)
	GetTargetIDs(ctx context.Context, userID string, targetType domain.UserFavoriteTargetType) ([]string, error)
}

// PlanCommentThreadRepository はPlanCommentThreadの永続化を担当する
type PlanCommentThreadRepository interface {
	Create(ctx context.Context, thread *domain.PlanCommentThread) error
	FindByID(ctx context.Context, id string) (*domain.PlanCommentThread, error)
	FindByPlanDocumentID(ctx context.Context, planDocumentID string, status *domain.PlanCommentThreadStatus) ([]*domain.PlanCommentThread, error)
	// CountActiveByPlanDocumentID returns the number of active threads for a plan
	// document without loading thread data.
	CountActiveByPlanDocumentID(ctx context.Context, planDocumentID string) (int, error)
	Update(ctx context.Context, thread *domain.PlanCommentThread) error
	Delete(ctx context.Context, id string) error
	// MarkOutdatedByPlanDocumentID marks all active threads as outdated for the given plan document
	MarkOutdatedByPlanDocumentID(ctx context.Context, planDocumentID string) error
}

// PlanCommentMessageRepository はPlanCommentMessageの永続化を担当する
type PlanCommentMessageRepository interface {
	Create(ctx context.Context, message *domain.PlanCommentMessage) error
	FindByID(ctx context.Context, id string) (*domain.PlanCommentMessage, error)
	FindByThreadID(ctx context.Context, threadID string) ([]*domain.PlanCommentMessage, error)
	Update(ctx context.Context, message *domain.PlanCommentMessage) error
	Delete(ctx context.Context, id string) error
	DeleteByThreadID(ctx context.Context, threadID string) error
}

// Repositories は全リポジトリをまとめる
type Repositories struct {
	Project              ProjectRepository
	Session              SessionRepository
	Event                EventRepository
	User                 UserRepository
	APIKey               APIKeyRepository
	WebSession           WebSessionRepository
	PasswordCredential   PasswordCredentialRepository
	OAuthConnection      OAuthConnectionRepository
	PlanDocument         PlanDocumentRepository
	PlanDocumentEvent    PlanDocumentEventRepository
	UserFavorite         UserFavoriteRepository
	PlanCommentThread    PlanCommentThreadRepository
	PlanCommentMessage   PlanCommentMessageRepository
}
