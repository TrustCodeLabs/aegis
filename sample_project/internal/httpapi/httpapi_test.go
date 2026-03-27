package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"aegis"
	sampleapp "aegis/sample_project/internal/app"
)

func TestExecutionContextAndErrorHandlingHelpers(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/notes", nil)
	req.Header.Set("X-Role", "admin")
	req.Header.Set("X-Tenant-ID", "team-b")
	req.Header.Set("X-Subject-ID", "tester")
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("X-Trace-ID", "trace-1")
	req.Header.Set("X-Confirm-Delete", "true")

	ctx, err := server.executionContextFromRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("build execution context: %v", err)
	}

	subject := aegis.SubjectFromContext(ctx)
	if subject.ID != "tester" {
		t.Fatalf("unexpected subject id: %q", subject.ID)
	}
	if aegis.TenantIDFromContext(ctx) != "team-b" {
		t.Fatalf("unexpected tenant id")
	}
	if !aegis.ConfirmedFromContext(ctx) {
		t.Fatalf("expected confirmation flag from header")
	}

	rec := httptest.NewRecorder()
	server.writeError(rec, req, aegis.NewKernelError(aegis.CodeResourceNotImplemented, "cache resource is not implemented yet", nil))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code": "not_implemented"`) {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}

	adminRec := httptest.NewRecorder()
	if !server.requireAdmin(adminRec, req) {
		t.Fatalf("expected admin request to pass")
	}

	viewerReq := httptest.NewRequest(http.MethodGet, "/api/admin/mcp-tools", nil)
	viewerRec := httptest.NewRecorder()
	if server.requireAdmin(viewerRec, viewerReq) {
		t.Fatalf("expected viewer request to fail admin check")
	}
}

func TestRoutesServeCRUDAndAdminEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	application, err := sampleapp.New(log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("create sample app: %v", err)
	}

	handler := New(application).Routes()

	assertStatus := func(method, path string, body io.Reader, headers map[string]string, want int) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, body)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("%s %s: expected %d, got %d, body=%s", method, path, want, rec.Code, rec.Body.String())
		}
		return rec
	}

	assertStatus(http.MethodGet, "/", nil, nil, http.StatusOK)
	assertStatus(http.MethodGet, "/health", nil, nil, http.StatusOK)

	createBody := strings.NewReader(`{"id":"httpapi-note-1","title":"Created","content":"body","internal":"secret"}`)
	createRec := assertStatus(http.MethodPost, "/api/notes", createBody, map[string]string{
		"Content-Type": "application/json",
	}, http.StatusCreated)
	if !strings.Contains(createRec.Body.String(), `"internal": "[redacted]"`) {
		t.Fatalf("expected redacted note response, got %s", createRec.Body.String())
	}

	assertStatus(http.MethodGet, "/api/notes/httpapi-note-1", nil, nil, http.StatusOK)

	updateBody := strings.NewReader(`{"title":"Updated","content":"new body","internal":"changed"}`)
	assertStatus(http.MethodPut, "/api/notes/httpapi-note-1", updateBody, map[string]string{
		"Content-Type": "application/json",
	}, http.StatusOK)

	assertStatus(http.MethodGet, "/api/admin/introspection/operations", nil, map[string]string{
		"X-Role": "admin",
	}, http.StatusOK)
	assertStatus(http.MethodGet, "/api/admin/introspection/topology", nil, map[string]string{
		"X-Role": "admin",
	}, http.StatusOK)
	assertStatus(http.MethodGet, "/api/admin/mcp-tools", nil, map[string]string{
		"X-Role": "admin",
	}, http.StatusOK)
	assertStatus(http.MethodGet, "/api/admin/effects", nil, map[string]string{
		"X-Role": "admin",
	}, http.StatusOK)
	skillsRec := assertStatus(http.MethodGet, "/api/admin/skills", nil, map[string]string{
		"X-Role": "admin",
	}, http.StatusOK)
	if contentType := skillsRec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/markdown") {
		t.Fatalf("expected markdown response, got %q", contentType)
	}
	if !strings.Contains(skillsRec.Body.String(), "# Skills") {
		t.Fatalf("expected skills markdown response, got %s", skillsRec.Body.String())
	}

	assertStatus(http.MethodDelete, "/api/notes/httpapi-note-1", nil, nil, http.StatusConflict)

	deleteRec := assertStatus(http.MethodDelete, "/api/notes/httpapi-note-1", nil, map[string]string{
		"X-Confirm-Delete": "true",
	}, http.StatusOK)

	var payload map[string]any
	if err := json.Unmarshal(deleteRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if payload["deleted"] != true {
		t.Fatalf("expected deleted flag, got %#v", payload)
	}

	modeBody := strings.NewReader(`{"mode":"direct"}`)
	assertStatus(http.MethodPost, "/api/admin/storage/mode", modeBody, map[string]string{
		"X-Role":       "admin",
		"Content-Type": "application/json",
	}, http.StatusOK)

	for _, route := range []struct {
		method string
		path   string
		body   io.Reader
	}{
		{method: http.MethodGet, path: "/api/admin/introspection/operations"},
		{method: http.MethodGet, path: "/api/admin/introspection/topology"},
		{method: http.MethodGet, path: "/api/admin/mcp-tools"},
		{method: http.MethodGet, path: "/api/admin/effects"},
		{method: http.MethodGet, path: "/api/admin/skills"},
		{method: http.MethodPost, path: "/api/admin/storage/mode", body: strings.NewReader(`{"mode":"layered"}`)},
	} {
		assertStatus(route.method, route.path, route.body, nil, http.StatusForbidden)
	}

	assertStatus(http.MethodPost, "/api/notes", strings.NewReader(`{"title":`), map[string]string{
		"Content-Type": "application/json",
	}, http.StatusBadRequest)
	assertStatus(http.MethodPut, "/api/notes/httpapi-note-1", strings.NewReader(`{"title":"broken"`), map[string]string{
		"Content-Type": "application/json",
	}, http.StatusBadRequest)
	assertStatus(http.MethodPost, "/api/admin/storage/mode", strings.NewReader(`{"mode":"unknown"}`), map[string]string{
		"X-Role":       "admin",
		"Content-Type": "application/json",
	}, http.StatusBadRequest)
	assertStatus(http.MethodPost, "/api/admin/storage/mode", strings.NewReader(`{"mode":`), map[string]string{
		"X-Role":       "admin",
		"Content-Type": "application/json",
	}, http.StatusBadRequest)
}
