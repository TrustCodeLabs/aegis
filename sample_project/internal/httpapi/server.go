package httpapi

import (
	"fmt"
	"net/http"

	"aegis"
	"aegis/restadapter"
	sampleapp "aegis/sample_project/internal/app"
	"aegis/sample_project/internal/notes"
)

type Server struct {
	app *sampleapp.App
}

type storageModeRequest struct {
	Mode string `json:"mode"`
}

func New(app *sampleapp.App) *Server {
	return &Server{app: app}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.Handle("GET /api/notes", newOperationHandler(s, notes.OperationList, http.StatusOK, restadapter.StaticInput(notes.ListInput{})))
	mux.Handle("POST /api/notes", newOperationHandler(s, notes.OperationCreate, http.StatusCreated, restadapter.DecodeJSONBody[notes.CreateInput]))
	mux.Handle("GET /api/notes/{id}", newOperationHandler(s, notes.OperationGet, http.StatusOK, lookupInputFromPath))
	mux.Handle("PUT /api/notes/{id}", newOperationHandler(s, notes.OperationUpdate, http.StatusOK, updateInputFromPath))
	mux.Handle("DELETE /api/notes/{id}", newOperationHandler(s, notes.OperationDelete, http.StatusOK, lookupInputFromPath))
	mux.HandleFunc("GET /api/admin/introspection/operations", s.handleOperations)
	mux.HandleFunc("GET /api/admin/introspection/topology", s.handleTopology)
	mux.HandleFunc("GET /api/admin/mcp-tools", s.handleMCPTools)
	mux.HandleFunc("GET /api/admin/effects", s.handleEffects)
	mux.HandleFunc("GET /api/admin/skills", s.handleSkills)
	mux.HandleFunc("POST /api/admin/storage/mode", s.handleStorageMode)

	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"service": sampleapp.ServiceName,
		"runtime": sampleapp.RuntimeName,
		"port":    sampleapp.EnvOrDefault("PORT", sampleapp.DefaultPort),
		"defaults": map[string]string{
			"tenant": sampleapp.DefaultTenant,
			"role":   sampleapp.DefaultRole,
		},
		"routes": []map[string]string{
			{"method": "GET", "path": "/health"},
			{"method": "GET", "path": "/api/notes"},
			{"method": "POST", "path": "/api/notes"},
			{"method": "GET", "path": "/api/notes/{id}"},
			{"method": "PUT", "path": "/api/notes/{id}"},
			{"method": "DELETE", "path": "/api/notes/{id}"},
			{"method": "GET", "path": "/api/admin/introspection/operations"},
			{"method": "GET", "path": "/api/admin/introspection/topology"},
			{"method": "GET", "path": "/api/admin/mcp-tools"},
			{"method": "GET", "path": "/api/admin/effects"},
			{"method": "GET", "path": "/api/admin/skills"},
			{"method": "POST", "path": "/api/admin/storage/mode"},
		},
		"headers": map[string]string{
			"X-Tenant-ID":      "team-a | team-b (default team-a)",
			"X-Role":           "viewer | editor | admin (default editor)",
			"X-Subject-ID":     "arbitrary subject id (default demo-user)",
			"X-Confirm-Delete": "true for DELETE /api/notes/{id}",
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ops, err := s.app.Kernel().Operations(r.Context(), aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
		AIOnly:         true,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"storage_mode":  s.app.StorageMode(),
		"skills_file":   sampleapp.SkillsOutputPath,
		"ai_operations": len(ops),
		"time":          nowUTC(),
	})
}

func (s *Server) handleOperations(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	ops, err := s.app.Kernel().Operations(r.Context(), aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"operations": ops})
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	graph, err := s.app.Kernel().Topology(r.Context(), aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, graph)
}

func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	tools, err := s.app.Kernel().MCPTools(r.Context(), aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (s *Server) handleEffects(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	query := r.URL.Query()
	effects, err := s.app.Kernel().Effects(r.Context(), aegis.EffectQuery{
		Operation: query.Get("operation"),
		TraceID:   query.Get("trace_id"),
		Status:    query.Get("status"),
		TenantID:  query.Get("tenant_id"),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"effects": effects})
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	content, err := s.app.Kernel().GenerateSkillsMarkdown(r.Context(), aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.app.GenerateSkillsMarkdown(); err != nil {
		s.app.Logger().Printf("failed to refresh generated skills file: %v", err)
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

func (s *Server) handleStorageMode(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	input, err := restadapter.DecodeJSONBody[storageModeRequest](r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	input.Mode = sampleapp.NormalizeStorageMode(input.Mode)
	if !sampleapp.IsValidStorageMode(input.Mode) {
		s.writeError(w, r, fmt.Errorf("mode must be %q or %q", sampleapp.StorageModeLayered, sampleapp.StorageModeDirect))
		return
	}

	ops, err := s.app.SwapStorageMode(r.Context(), input.Mode)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"storage_mode": input.Mode,
		"operations":   ops,
	})
}

func newOperationHandler[I any](s *Server, operation string, successStatus int, binder restadapter.InputBinder[I]) http.Handler {
	return restadapter.NewJSONHandler(restadapter.Operation[I]{
		Kernel:         s.app.Kernel(),
		Operation:      operation,
		ContextBuilder: s.executionContextFromRequest,
		InputBinder:    binder,
		SuccessStatus:  successStatus,
		ErrorEncoder:   s.writeError,
	})
}

func lookupInputFromPath(r *http.Request) (notes.LookupInput, error) {
	return notes.LookupInput{ID: r.PathValue("id")}, nil
}

func updateInputFromPath(r *http.Request) (notes.UpdateInput, error) {
	input, err := restadapter.DecodeJSONBody[notes.UpdateInput](r)
	if err != nil {
		return notes.UpdateInput{}, err
	}
	input.ID = r.PathValue("id")
	return input, nil
}
