package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hangrix/hangrix/apps/hangrix/internal/httpx"
	actordomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/actor/domain"
	authdomain "github.com/hangrix/hangrix/apps/hangrix/internal/modules/auth/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/runner/service"
)

type UserHandler struct {
	repo          domain.Repo
	middleware    authdomain.Middleware
	actorResolver actordomain.Resolver
}

type UserHandlerDeps struct {
	Repo          domain.Repo
	Middleware    authdomain.Middleware
	ActorResolver actordomain.Resolver
}

func NewUserHandler(deps *UserHandlerDeps) *UserHandler {
	return &UserHandler{
		repo:          deps.Repo,
		middleware:    deps.Middleware,
		actorResolver: deps.ActorResolver,
	}
}

func (h *UserHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/runners", func(r chi.Router) {
		r.Use(h.middleware.RequireAuth)
		r.Post("/", h.createRunner)
		r.Get("/", h.listRunners)
		r.Delete("/{id}", h.disableRunner)
		r.Delete("/{id}/permanent", h.removeRunner)
	})
}

func (h *UserHandler) toPublicRunner(r *domain.Runner) publicRunner {
	var caps any = map[string]any{}
	if len(r.Capabilities) > 0 {
		_ = json.Unmarshal(r.Capabilities, &caps)
	}
	createdBy := int64(0)
	if h.actorResolver != nil {
		uid, ok := h.actorResolver.UserID(context.Background(), r.ActorID)
		if ok {
			createdBy = uid
		}
	}
	return publicRunner{
		ID:                r.ID,
		Name:              r.Name,
		OwnerUserID:       r.OwnerUserID,
		Visibility:        string(r.Visibility),
		Status:            string(r.Status),
		Online:            r.Online(time.Now()),
		Capabilities:      caps,
		LastHeartbeatAt:   r.LastHeartbeatAt,
		EnrollTokenPrefix: r.EnrollTokenPrefix,
		EnrollTokenUsed:   r.EnrollTokenUsedAt != nil,
		AgentTokenPrefix:  r.AgentTokenPrefix,
		AgentTokenRevoked: r.AgentTokenRevokedAt != nil,
		CreatedBy:         createdBy,
		ActorID:           r.ActorID,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
}

func (h *UserHandler) listRunners(w http.ResponseWriter, r *http.Request) {
	caller, _ := authdomain.UserFromRequest(r)
	if caller == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	visibility := domain.VisibilityUser
	rows, err := h.repo.ListRunners(r.Context(), &caller.ID, &visibility)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]publicRunner, 0, len(rows))
	for _, rr := range rows {
		items = append(items, h.toPublicRunner(rr))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *UserHandler) createRunner(w http.ResponseWriter, r *http.Request) {
	caller, _ := authdomain.UserFromRequest(r)
	if caller == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createRunnerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !runnerNameRe.MatchString(req.Name) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid name")
		return
	}
	actorID, err := currentUserActorID(r.Context(), h.actorResolver, caller)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	in := domain.CreateRunnerInput{
		Name:        req.Name,
		OwnerUserID: &caller.ID,
		Visibility:  domain.VisibilityUser,
		ActorID:     actorID,
	}
	if err := in.Validate(); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	plaintext, prefix, hashed, err := service.MintEnrollToken()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	runner, err := h.repo.CreateRunner(r.Context(), in, domain.NewEnrollToken{
		Prefix: prefix,
		Hash:   string(hashed),
	})
	if err != nil {
		if errors.Is(err, domain.ErrRunnerConflict) {
			httpx.WriteError(w, http.StatusConflict, "name already taken")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, createRunnerResp{
		Runner:               h.toPublicRunner(runner),
		EnrollTokenPlaintext: plaintext,
	})
}

func (h *UserHandler) disableRunner(w http.ResponseWriter, r *http.Request) {
	runner, ok := h.resolveOwnedRunner(w, r)
	if !ok {
		return
	}
	if err := h.repo.DisableRunner(r.Context(), runner.ID); err != nil {
		if errors.Is(err, domain.ErrRunnerNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "runner not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) removeRunner(w http.ResponseWriter, r *http.Request) {
	runner, ok := h.resolveOwnedRunner(w, r)
	if !ok {
		return
	}
	if err := h.repo.DeleteRunner(r.Context(), runner.ID); err != nil {
		if errors.Is(err, domain.ErrRunnerNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "runner not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) resolveOwnedRunner(w http.ResponseWriter, r *http.Request) (*domain.Runner, bool) {
	caller, _ := authdomain.UserFromRequest(r)
	if caller == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return nil, false
	}
	id, ok := httpx.ParseID(w, chi.URLParam(r, "id"))
	if !ok {
		return nil, false
	}
	runner, err := h.repo.GetRunnerByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrRunnerNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "runner not found")
			return nil, false
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if runner.OwnerUserID == nil || *runner.OwnerUserID != caller.ID || runner.Visibility != domain.VisibilityUser {
		httpx.WriteError(w, http.StatusNotFound, "runner not found")
		return nil, false
	}
	return runner, true
}
