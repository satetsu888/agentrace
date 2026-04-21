package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/satetsu888/agentrace/server/internal/domain"
	"github.com/satetsu888/agentrace/server/internal/repository"
)

type PlanDocumentHandler struct {
	repos  *repository.Repositories
	webURL string
}

func NewPlanDocumentHandler(webURL string, repos *repository.Repositories) *PlanDocumentHandler {
	return &PlanDocumentHandler{repos: repos, webURL: webURL}
}

// Response types

type CollaboratorResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type PlanDocumentProjectResponse struct {
	ID                     string `json:"id"`
	CanonicalGitRepository string `json:"canonical_git_repository"`
}

type PlanDocumentResponse struct {
	ID                  string                       `json:"id"`
	Project             *PlanDocumentProjectResponse `json:"project"`
	Description         string                       `json:"description"`
	Body                string                       `json:"body"`
	Status              string                       `json:"status"`
	Collaborators       []*CollaboratorResponse      `json:"collaborators"`
	URL                 string                       `json:"url,omitempty"`
	CreatedAt           string                       `json:"created_at"`
	UpdatedAt           string                       `json:"updated_at"`
	IsFavorited         bool                         `json:"is_favorited"`
	ActiveCommentsCount int                          `json:"active_comments_count"`
}

type PlanDocumentListResponse struct {
	Plans      []*PlanDocumentResponse `json:"plans"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type PlanDocumentEventResponse struct {
	ID              string  `json:"id"`
	PlanDocumentID  string  `json:"plan_document_id"`
	SessionID       *string `json:"session_id"`
	ClaudeSessionID *string `json:"claude_session_id"`
	ToolUseID       *string `json:"tool_use_id"`
	UserID          *string `json:"user_id"`
	UserName        *string `json:"user_name"`
	EventType       string  `json:"event_type"`
	Patch           string  `json:"patch"`
	Message         string  `json:"message"`
	CreatedAt       string  `json:"created_at"`
}

type PlanDocumentEventsResponse struct {
	Events []*PlanDocumentEventResponse `json:"events"`
}

// Request types

type CreatePlanDocumentRequest struct {
	Description     string  `json:"description"`
	Body            string  `json:"body"`
	ProjectID       *string `json:"project_id"`
	Status          *string `json:"status"`
	ClaudeSessionID *string `json:"claude_session_id"`
	ToolUseID       *string `json:"tool_use_id"`
}

type UpdatePlanDocumentRequest struct {
	Description     *string `json:"description"`
	Body            *string `json:"body"`
	Patch           *string `json:"patch"`
	Message         *string `json:"message"`
	ClaudeSessionID *string `json:"claude_session_id"`
	ToolUseID       *string `json:"tool_use_id"`
	ProjectID       *string `json:"project_id"`
}

type SetPlanDocumentStatusRequest struct {
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

// Helper functions

func (h *PlanDocumentHandler) planDocumentToResponse(ctx context.Context, doc *domain.PlanDocument, isFavorited bool) (*PlanDocumentResponse, error) {
	// Get collaborator user IDs from events
	userIDs, err := h.repos.PlanDocumentEvent.GetCollaboratorUserIDs(ctx, doc.ID)
	if err != nil {
		return nil, err
	}

	users, err := h.repos.User.FindByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	// Fetch project (single lookup for single-plan paths)
	var projectCache map[string]*domain.Project
	if doc.ProjectID != "" {
		projectCache, err = h.repos.Project.FindByIDs(ctx, []string{doc.ProjectID})
		if err != nil {
			return nil, err
		}
	}

	activeCommentsCount, err := h.repos.PlanCommentThread.CountActiveByPlanDocumentID(ctx, doc.ID)
	if err != nil {
		// Count is non-critical; fall back to zero rather than failing the request.
		activeCommentsCount = 0
	}

	return h.buildPlanDocumentResponse(doc, isFavorited, userIDs, users, projectCache, activeCommentsCount), nil
}

// planDocumentsToResponseBatch builds responses for multiple plans while
// pre-batching user and project lookups to eliminate N+1 queries during list
// requests.
func (h *PlanDocumentHandler) planDocumentsToResponseBatch(ctx context.Context, docs []*domain.PlanDocument, favoritedIDs map[string]bool) ([]*PlanDocumentResponse, error) {
	if len(docs) == 0 {
		return []*PlanDocumentResponse{}, nil
	}

	// Collect collaborator user IDs per plan (one query per plan; the expensive
	// part — resolving each user — is batched below).
	collaboratorIDsByPlan := make(map[string][]string, len(docs))
	userIDSet := make(map[string]struct{})
	projectIDSet := make(map[string]struct{})

	for _, doc := range docs {
		ids, err := h.repos.PlanDocumentEvent.GetCollaboratorUserIDs(ctx, doc.ID)
		if err != nil {
			return nil, err
		}
		collaboratorIDsByPlan[doc.ID] = ids
		for _, id := range ids {
			userIDSet[id] = struct{}{}
		}
		if doc.ProjectID != "" {
			projectIDSet[doc.ProjectID] = struct{}{}
		}
	}

	userIDs := make([]string, 0, len(userIDSet))
	for id := range userIDSet {
		userIDs = append(userIDs, id)
	}
	users, err := h.repos.User.FindByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	projectIDs := make([]string, 0, len(projectIDSet))
	for id := range projectIDSet {
		projectIDs = append(projectIDs, id)
	}
	projects, err := h.repos.Project.FindByIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	responses := make([]*PlanDocumentResponse, 0, len(docs))
	for _, doc := range docs {
		count, err := h.repos.PlanCommentThread.CountActiveByPlanDocumentID(ctx, doc.ID)
		if err != nil {
			count = 0
		}
		responses = append(responses, h.buildPlanDocumentResponse(
			doc, favoritedIDs[doc.ID], collaboratorIDsByPlan[doc.ID], users, projects, count,
		))
	}
	return responses, nil
}

func (h *PlanDocumentHandler) buildPlanDocumentResponse(
	doc *domain.PlanDocument,
	isFavorited bool,
	collaboratorIDs []string,
	users map[string]*domain.User,
	projects map[string]*domain.Project,
	activeCommentsCount int,
) *PlanDocumentResponse {
	collaborators := make([]*CollaboratorResponse, 0, len(collaboratorIDs))
	for _, userID := range collaboratorIDs {
		if user, ok := users[userID]; ok && user != nil {
			collaborators = append(collaborators, &CollaboratorResponse{
				ID:          user.ID,
				DisplayName: user.GetDisplayName(),
			})
		}
	}

	var projectResp *PlanDocumentProjectResponse
	if doc.ProjectID != "" {
		if project, ok := projects[doc.ProjectID]; ok && project != nil {
			projectResp = &PlanDocumentProjectResponse{
				ID:                     project.ID,
				CanonicalGitRepository: project.CanonicalGitRepository,
			}
		}
	}

	var url string
	if h.webURL != "" {
		url = h.webURL + "/plans/" + doc.ID
	}

	return &PlanDocumentResponse{
		ID:                  doc.ID,
		Project:             projectResp,
		Description:         doc.Description,
		Body:                doc.Body,
		Status:              string(doc.Status),
		Collaborators:       collaborators,
		URL:                 url,
		CreatedAt:           doc.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           doc.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		IsFavorited:         isFavorited,
		ActiveCommentsCount: activeCommentsCount,
	}
}

func (h *PlanDocumentHandler) eventToResponse(ctx context.Context, event *domain.PlanDocumentEvent) *PlanDocumentEventResponse {
	var userName *string
	if event.UserID != nil {
		user, err := h.repos.User.FindByID(ctx, *event.UserID)
		if err == nil && user != nil {
			displayName := user.GetDisplayName()
			userName = &displayName
		}
	}

	// Resolve claude_session_id to AgenTrace session ID
	var sessionID *string
	if event.ClaudeSessionID != nil && *event.ClaudeSessionID != "" {
		session, err := h.repos.Session.FindByClaudeSessionID(ctx, *event.ClaudeSessionID)
		if err == nil && session != nil {
			sessionID = &session.ID
		}
	}

	eventType := string(event.EventType)
	if eventType == "" {
		eventType = string(domain.PlanDocumentEventTypeBodyChange)
	}

	return &PlanDocumentEventResponse{
		ID:              event.ID,
		PlanDocumentID:  event.PlanDocumentID,
		SessionID:       sessionID,
		ClaudeSessionID: event.ClaudeSessionID,
		ToolUseID:       event.ToolUseID,
		UserID:          event.UserID,
		UserName:        userName,
		EventType:       eventType,
		Patch:           event.Patch,
		Message:         event.Message,
		CreatedAt:       event.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// Handlers

// List returns all plan documents, optionally filtered by project_id, git_remote_url, status, or collaborator
func (h *PlanDocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := GetUserIDFromContext(ctx)

	// Parse query parameters
	limit := 100
	cursor := r.URL.Query().Get("cursor")
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	projectID := r.URL.Query().Get("project_id")
	gitRemoteURL := r.URL.Query().Get("git_remote_url")
	statusParam := r.URL.Query().Get("status")
	descriptionParam := r.URL.Query().Get("description")
	collaboratorParam := r.URL.Query().Get("collaborator")
	sortBy := r.URL.Query().Get("sort")
	// Validate sortBy - default to updated_at
	if sortBy != "created_at" {
		sortBy = "updated_at"
	}

	// Parse status parameter (comma-separated)
	var statuses []domain.PlanDocumentStatus
	if statusParam != "" {
		statusStrs := strings.Split(statusParam, ",")
		for _, s := range statusStrs {
			s = strings.TrimSpace(s)
			status := domain.PlanDocumentStatus(s)
			if status.IsValid() {
				statuses = append(statuses, status)
			}
		}
	}

	// Parse collaborator parameter (comma-separated user IDs)
	var collaboratorUserIDs []string
	if collaboratorParam != "" {
		for _, id := range strings.Split(collaboratorParam, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				collaboratorUserIDs = append(collaboratorUserIDs, id)
			}
		}
	}

	// Resolve project ID from git_remote_url if needed
	if projectID == "" && gitRemoteURL != "" {
		canonicalURL := domain.NormalizeGitURL(gitRemoteURL)
		project, projErr := h.repos.Project.FindByCanonicalGitRepository(ctx, canonicalURL)
		if projErr != nil {
			http.Error(w, `{"error": "failed to find project"}`, http.StatusInternalServerError)
			return
		}
		if project != nil {
			projectID = project.ID
		}
	}

	// If collaborator filter is specified, first get matching plan document IDs
	var planDocumentIDs []string
	if len(collaboratorUserIDs) > 0 {
		ids, err := h.repos.PlanDocumentEvent.GetPlanDocumentIDsByUserIDs(ctx, collaboratorUserIDs)
		if err != nil {
			http.Error(w, `{"error": "failed to filter by collaborator"}`, http.StatusInternalServerError)
			return
		}
		planDocumentIDs = ids
		// If no plan documents match the collaborator filter, return empty list
		if len(planDocumentIDs) == 0 {
			response := PlanDocumentListResponse{Plans: []*PlanDocumentResponse{}}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Use unified Find method with query object
	query := domain.PlanDocumentQuery{
		ProjectID:           projectID,
		Statuses:            statuses,
		DescriptionContains: descriptionParam,
		PlanDocumentIDs:     planDocumentIDs,
		Limit:               limit,
		Cursor:              cursor,
		SortBy:              sortBy,
	}
	docs, nextCursor, err := h.repos.PlanDocument.Find(ctx, query)

	if err != nil {
		http.Error(w, `{"error": "failed to fetch plan documents"}`, http.StatusInternalServerError)
		return
	}

	// Get favorited plan IDs for the current user
	favoritedIDs := make(map[string]bool)
	if userID != "" {
		targetIDs, err := h.repos.UserFavorite.GetTargetIDs(ctx, userID, domain.UserFavoriteTargetTypePlan)
		if err == nil {
			for _, id := range targetIDs {
				favoritedIDs[id] = true
			}
		}
	}

	plans, err := h.planDocumentsToResponseBatch(ctx, docs, favoritedIDs)
	if err != nil {
		http.Error(w, `{"error": "failed to build response"}`, http.StatusInternalServerError)
		return
	}

	// Sort: favorited plans first, then by updated_at desc (already sorted by repo)
	sortPlansByFavorite(plans)

	response := PlanDocumentListResponse{
		Plans:      plans,
		NextCursor: nextCursor,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// sortPlansByFavorite sorts plans with favorited ones first, maintaining original order within each group
func sortPlansByFavorite(plans []*PlanDocumentResponse) {
	favorited := make([]*PlanDocumentResponse, 0)
	notFavorited := make([]*PlanDocumentResponse, 0)
	for _, p := range plans {
		if p.IsFavorited {
			favorited = append(favorited, p)
		} else {
			notFavorited = append(notFavorited, p)
		}
	}
	copy(plans, append(favorited, notFavorited...))
}

// Get returns a single plan document by ID
func (h *PlanDocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := GetUserIDFromContext(ctx)
	vars := mux.Vars(r)
	id := vars["id"]

	doc, err := h.repos.PlanDocument.FindByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch plan document"}`, http.StatusInternalServerError)
		return
	}
	if doc == nil {
		http.Error(w, `{"error": "plan document not found"}`, http.StatusNotFound)
		return
	}

	// Check if favorited
	var isFavorited bool
	if userID != "" {
		fav, err := h.repos.UserFavorite.FindByUserAndTarget(ctx, userID, domain.UserFavoriteTargetTypePlan, id)
		if err == nil && fav != nil {
			isFavorited = true
		}
	}

	resp, err := h.planDocumentToResponse(ctx, doc, isFavorited)
	if err != nil {
		http.Error(w, `{"error": "failed to build response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetEvents returns the change history for a plan document
func (h *PlanDocumentHandler) GetEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	// First check if the plan document exists
	doc, err := h.repos.PlanDocument.FindByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch plan document"}`, http.StatusInternalServerError)
		return
	}
	if doc == nil {
		http.Error(w, `{"error": "plan document not found"}`, http.StatusNotFound)
		return
	}

	events, err := h.repos.PlanDocumentEvent.FindByPlanDocumentID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch events"}`, http.StatusInternalServerError)
		return
	}

	eventResponses := make([]*PlanDocumentEventResponse, len(events))
	for i, event := range events {
		eventResponses[i] = h.eventToResponse(ctx, event)
	}

	response := PlanDocumentEventsResponse{Events: eventResponses}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Create creates a new plan document
func (h *PlanDocumentHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreatePlanDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Description == "" {
		http.Error(w, `{"error": "description is required"}`, http.StatusBadRequest)
		return
	}

	// Get user ID from context (set by auth middleware)
	userID := GetUserIDFromContext(ctx)

	// Determine project ID: explicit project_id > session-based > default
	projectID := domain.DefaultProjectID
	if req.ProjectID != nil && *req.ProjectID != "" {
		// Use explicitly provided project ID
		projectID = *req.ProjectID
	} else if req.ClaudeSessionID != nil && *req.ClaudeSessionID != "" {
		// Fall back to session-based project ID
		session, err := h.repos.Session.FindByClaudeSessionID(ctx, *req.ClaudeSessionID)
		if err != nil {
			http.Error(w, `{"error": "failed to find session"}`, http.StatusInternalServerError)
			return
		}
		if session != nil && session.ProjectID != "" {
			projectID = session.ProjectID
		}
	}

	// Determine initial status (default: planning)
	status := domain.PlanDocumentStatusPlanning
	if req.Status != nil && *req.Status != "" {
		requestedStatus := domain.PlanDocumentStatus(*req.Status)
		if requestedStatus.IsValid() {
			status = requestedStatus
		}
	}

	doc := &domain.PlanDocument{
		ProjectID:   projectID,
		Description: req.Description,
		Body:        req.Body,
		Status:      status,
	}

	if err := h.repos.PlanDocument.Create(ctx, doc); err != nil {
		http.Error(w, `{"error": "failed to create plan document"}`, http.StatusInternalServerError)
		return
	}

	// Create initial event with body as "all additions" diff
	event := &domain.PlanDocumentEvent{
		PlanDocumentID:  doc.ID,
		ClaudeSessionID: req.ClaudeSessionID,
		ToolUseID:       req.ToolUseID,
		Patch:           bodyToInitialPatch(req.Body),
	}
	if userID != "" {
		event.UserID = &userID
	}

	if err := h.repos.PlanDocumentEvent.Create(ctx, event); err != nil {
		// Log error but don't fail the request
		// The document was created successfully
		log.Printf("Warning: failed to create plan document event: %v", err)
	}

	resp, err := h.planDocumentToResponse(ctx, doc, false)
	if err != nil {
		http.Error(w, `{"error": "failed to build response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// Update updates an existing plan document
func (h *PlanDocumentHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUserID := GetUserIDFromContext(ctx)
	vars := mux.Vars(r)
	id := vars["id"]

	doc, err := h.repos.PlanDocument.FindByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch plan document"}`, http.StatusInternalServerError)
		return
	}
	if doc == nil {
		http.Error(w, `{"error": "plan document not found"}`, http.StatusNotFound)
		return
	}

	var req UpdatePlanDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Get user ID from context (set by auth middleware)
	userID := GetUserIDFromContext(ctx)

	// Track if body is being updated (for comment invalidation)
	bodyUpdated := req.Body != nil && *req.Body != doc.Body

	// Update fields if provided
	if req.Description != nil {
		doc.Description = *req.Description
	}
	if req.Body != nil {
		doc.Body = *req.Body
	}
	if req.ProjectID != nil {
		doc.ProjectID = *req.ProjectID
	}

	if err := h.repos.PlanDocument.Update(ctx, doc); err != nil {
		http.Error(w, `{"error": "failed to update plan document"}`, http.StatusInternalServerError)
		return
	}

	// If body was updated, check and invalidate comments whose target text is no longer found
	if bodyUpdated {
		h.invalidateOutdatedComments(ctx, doc)
	}

	// Create event with patch if provided
	if req.Patch != nil {
		event := &domain.PlanDocumentEvent{
			PlanDocumentID:  doc.ID,
			ClaudeSessionID: req.ClaudeSessionID,
			ToolUseID:       req.ToolUseID,
			Patch:           *req.Patch,
		}
		if req.Message != nil {
			event.Message = *req.Message
		}
		if userID != "" {
			event.UserID = &userID
		}

		if err := h.repos.PlanDocumentEvent.Create(ctx, event); err != nil {
			// Log error but don't fail the request
			// The document was updated successfully
			log.Printf("Warning: failed to create plan document event: %v", err)
		}
	}

	// Check if favorited
	var isFavorited bool
	if currentUserID != "" {
		fav, err := h.repos.UserFavorite.FindByUserAndTarget(ctx, currentUserID, domain.UserFavoriteTargetTypePlan, id)
		if err == nil && fav != nil {
			isFavorited = true
		}
	}

	resp, err := h.planDocumentToResponse(ctx, doc, isFavorited)
	if err != nil {
		http.Error(w, `{"error": "failed to build response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Delete deletes a plan document
func (h *PlanDocumentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	doc, err := h.repos.PlanDocument.FindByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch plan document"}`, http.StatusInternalServerError)
		return
	}
	if doc == nil {
		http.Error(w, `{"error": "plan document not found"}`, http.StatusNotFound)
		return
	}

	if err := h.repos.PlanDocument.Delete(ctx, id); err != nil {
		http.Error(w, `{"error": "failed to delete plan document"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetStatus sets the status of a plan document
func (h *PlanDocumentHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUserID := GetUserIDFromContext(ctx)
	vars := mux.Vars(r)
	id := vars["id"]

	doc, err := h.repos.PlanDocument.FindByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch plan document"}`, http.StatusInternalServerError)
		return
	}
	if doc == nil {
		http.Error(w, `{"error": "plan document not found"}`, http.StatusNotFound)
		return
	}

	var req SetPlanDocumentStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error": "invalid request body"}`, http.StatusBadRequest)
		return
	}

	status := domain.PlanDocumentStatus(req.Status)
	if !status.IsValid() {
		http.Error(w, `{"error": "invalid status. must be one of: scratch, draft, planning, pending, ready, implementation, complete"}`, http.StatusBadRequest)
		return
	}

	// Store old status for event
	oldStatus := doc.Status

	if err := h.repos.PlanDocument.SetStatus(ctx, id, status); err != nil {
		http.Error(w, `{"error": "failed to update status"}`, http.StatusInternalServerError)
		return
	}

	// Get user ID from context (set by auth middleware)
	userID := GetUserIDFromContext(ctx)

	// Create status change event
	event := &domain.PlanDocumentEvent{
		PlanDocumentID: doc.ID,
		EventType:      domain.PlanDocumentEventTypeStatusChange,
		Patch:          string(oldStatus) + " -> " + string(status),
	}
	if req.Message != nil {
		event.Message = *req.Message
	}
	if userID != "" {
		event.UserID = &userID
	}
	// Ignore error - status was updated successfully
	h.repos.PlanDocumentEvent.Create(ctx, event)

	// Fetch updated document
	doc, err = h.repos.PlanDocument.FindByID(ctx, id)
	if err != nil {
		http.Error(w, `{"error": "failed to fetch updated plan document"}`, http.StatusInternalServerError)
		return
	}

	// Check if favorited
	var isFavorited bool
	if currentUserID != "" {
		fav, err := h.repos.UserFavorite.FindByUserAndTarget(ctx, currentUserID, domain.UserFavoriteTargetTypePlan, id)
		if err == nil && fav != nil {
			isFavorited = true
		}
	}

	resp, err := h.planDocumentToResponse(ctx, doc, isFavorited)
	if err != nil {
		http.Error(w, `{"error": "failed to build response"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// invalidateOutdatedComments checks all active comment threads and marks them as outdated
// if their target text can no longer be found in the updated document body
func (h *PlanDocumentHandler) invalidateOutdatedComments(ctx context.Context, doc *domain.PlanDocument) {
	// Get all active threads for this plan
	activeStatus := domain.PlanCommentThreadStatusActive
	threads, err := h.repos.PlanCommentThread.FindByPlanDocumentID(ctx, doc.ID, &activeStatus)
	if err != nil {
		// Log error but don't fail - the document update was successful
		return
	}

	for _, thread := range threads {
		// Check if the thread's target text can still be found
		position := FindThreadPosition(doc.Body, thread)
		if !position.Found {
			// Mark as outdated
			thread.Status = domain.PlanCommentThreadStatusOutdated
			h.repos.PlanCommentThread.Update(ctx, thread)
			// Ignore update errors - best effort
		}
	}
}

// bodyToInitialPatch converts body content to an "all additions" diff format.
// Each line is prefixed with "+" to indicate it was added.
func bodyToInitialPatch(body string) string {
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = "+" + line
	}
	return strings.Join(result, "\n")
}
