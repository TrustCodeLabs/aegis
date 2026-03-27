package httpapi

import (
	"context"
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

	mux.Handle("GET /", newEndpointHandler(s, nil, http.StatusOK, restadapter.NoInput[struct{}](), nil, s.rootResponse))
	mux.Handle("GET /health", newEndpointHandler(s, nil, http.StatusOK, restadapter.NoInput[struct{}](), nil, s.healthResponse))
	mux.Handle("GET /api/notes", newOperationHandler(s, notes.OperationList, http.StatusOK, restadapter.StaticInput(notes.ListInput{})))
	mux.Handle("POST /api/notes", newOperationHandler(s, notes.OperationCreate, http.StatusCreated, restadapter.DecodeJSONBody[notes.CreateInput]))
	mux.Handle("GET /api/notes/{id}", newOperationHandler(s, notes.OperationGet, http.StatusOK, lookupInputFromPath))
	mux.Handle("PUT /api/notes/{id}", newOperationHandler(s, notes.OperationUpdate, http.StatusOK, updateInputFromPath))
	mux.Handle("DELETE /api/notes/{id}", newOperationHandler(s, notes.OperationDelete, http.StatusOK, lookupInputFromPath))
	mux.Handle("GET /api/admin/introspection/operations", newAdminEndpointHandler(s, http.StatusOK, restadapter.NoInput[struct{}](), nil, s.operationsResponse))
	mux.Handle("GET /api/admin/introspection/topology", newAdminEndpointHandler(s, http.StatusOK, restadapter.NoInput[struct{}](), nil, s.topologyResponse))
	mux.Handle("GET /api/admin/mcp-tools", newAdminEndpointHandler(s, http.StatusOK, restadapter.NoInput[struct{}](), nil, s.mcpToolsResponse))
	mux.Handle("GET /api/admin/effects", newAdminEndpointHandler(s, http.StatusOK, effectQueryFromRequest, nil, s.effectsResponse))
	mux.Handle("GET /api/admin/skills", newAdminEndpointHandler(s, http.StatusOK, restadapter.NoInput[struct{}](), s.writeMarkdown, s.skillsResponse))
	mux.Handle("POST /api/admin/storage/mode", newAdminEndpointHandler(s, http.StatusOK, storageModeFromRequest, nil, s.storageModeResponse))

	return mux
}

func (s *Server) rootResponse(ctx context.Context, input struct{}) (any, error) {
	return map[string]any{
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
	}, nil
}

func (s *Server) healthResponse(ctx context.Context, input struct{}) (any, error) {
	ops, err := s.app.Kernel().Operations(ctx, aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
		AIOnly:         true,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"ok":            true,
		"storage_mode":  s.app.StorageMode(),
		"skills_file":   sampleapp.SkillsOutputPath,
		"ai_operations": len(ops),
		"time":          nowUTC(),
	}, nil
}

func (s *Server) operationsResponse(ctx context.Context, input struct{}) (any, error) {
	ops, err := s.app.Kernel().Operations(ctx, aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"operations": ops}, nil
}

func (s *Server) topologyResponse(ctx context.Context, input struct{}) (any, error) {
	return s.app.Kernel().Topology(ctx, aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
}

func (s *Server) mcpToolsResponse(ctx context.Context, input struct{}) (any, error) {
	tools, err := s.app.Kernel().MCPTools(ctx, aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"tools": tools}, nil
}

func (s *Server) effectsResponse(ctx context.Context, input aegis.EffectQuery) (any, error) {
	effects, err := s.app.Kernel().Effects(ctx, input)
	if err != nil {
		return nil, err
	}
	return map[string]any{"effects": effects}, nil
}

func (s *Server) skillsResponse(ctx context.Context, input struct{}) (any, error) {
	content, err := s.app.Kernel().GenerateSkillsMarkdown(ctx, aegis.IntrospectionFilter{
		VisibilityTier: sampleapp.VisibilityTierInternal,
	})
	if err != nil {
		return nil, err
	}
	if err := s.app.GenerateSkillsMarkdown(); err != nil {
		s.app.Logger().Printf("failed to refresh generated skills file: %v", err)
	}
	return content, nil
}

func (s *Server) storageModeResponse(ctx context.Context, input storageModeRequest) (any, error) {
	ops, err := s.app.SwapStorageMode(ctx, input.Mode)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"ok":           true,
		"storage_mode": input.Mode,
		"operations":   ops,
	}, nil
}

func newEndpointHandler[I any](s *Server, builder restadapter.ContextBuilder, successStatus int, binder restadapter.InputBinder[I], responseEncoder restadapter.ResponseEncoder, handler restadapter.HandlerFunc[I]) http.Handler {
	cfg := restadapter.Endpoint[I]{
		ContextBuilder: builder,
		InputBinder:    binder,
		SuccessStatus:  successStatus,
		ErrorEncoder:   s.writeError,
		Handler:        handler,
	}
	if responseEncoder != nil {
		cfg.ResponseEncoder = responseEncoder
	}
	return restadapter.NewJSONEndpoint(cfg)
}

func newAdminEndpointHandler[I any](s *Server, successStatus int, binder restadapter.InputBinder[I], responseEncoder restadapter.ResponseEncoder, handler restadapter.HandlerFunc[I]) http.Handler {
	return newEndpointHandler(s, s.adminContextFromRequest, successStatus, binder, responseEncoder, handler)
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

func effectQueryFromRequest(r *http.Request) (aegis.EffectQuery, error) {
	query := r.URL.Query()
	return aegis.EffectQuery{
		Operation: query.Get("operation"),
		TraceID:   query.Get("trace_id"),
		Status:    query.Get("status"),
		TenantID:  query.Get("tenant_id"),
	}, nil
}

func storageModeFromRequest(r *http.Request) (storageModeRequest, error) {
	input, err := restadapter.DecodeJSONBody[storageModeRequest](r)
	if err != nil {
		return storageModeRequest{}, err
	}

	input.Mode = sampleapp.NormalizeStorageMode(input.Mode)
	if !sampleapp.IsValidStorageMode(input.Mode) {
		return storageModeRequest{}, aegis.NewKernelError(
			aegis.CodeInvalidInput,
			fmt.Sprintf("mode must be %q or %q", sampleapp.StorageModeLayered, sampleapp.StorageModeDirect),
			nil,
		)
	}
	return input, nil
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
