package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aegis"
	sampleapp "aegis/sample_project/internal/app"
	"aegis/sample_project/internal/notes"
)

func (s *Server) executionContextFromRequest(ctx context.Context, r *http.Request) (context.Context, error) {
	role := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Role"), sampleapp.DefaultRole)))
	tenantID := strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Tenant-ID"), sampleapp.DefaultTenant))
	subjectID := strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Subject-ID"), sampleapp.DefaultSubjectID))
	requestID := strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Request-ID"), generateID("req")))
	traceID := strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Trace-ID"), generateID("trace")))

	caps := notes.CapabilitiesForRole(role)
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = aegis.WithSubject(ctx, aegis.Subject{
		ID:           subjectID,
		Type:         "user",
		Roles:        []string{role},
		Capabilities: caps,
		Attributes: map[string]any{
			"tenant_id": tenantID,
			"role":      role,
		},
	})
	ctx = aegis.WithCapabilities(ctx, aegis.NewGrantedCapabilities(caps...))
	ctx = aegis.WithTenantID(ctx, tenantID)
	ctx = aegis.WithTransport(ctx, "http")
	ctx = aegis.WithEnvironment(ctx, sampleapp.EnvironmentName)
	ctx = aegis.WithRequestID(ctx, requestID)
	ctx = aegis.WithTraceID(ctx, traceID)
	ctx = aegis.WithMetadata(ctx, map[string]any{
		"http_path":   r.URL.Path,
		"http_method": r.Method,
		"user_agent":  r.UserAgent(),
	})
	if isTruthy(r.Header.Get("X-Confirm")) || isTruthy(r.Header.Get("X-Confirm-Delete")) {
		ctx = aegis.WithConfirmed(ctx, true)
	}
	return ctx, nil
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if roleFromRequest(r) == "admin" {
		return true
	}
	s.writeJSON(w, http.StatusForbidden, map[string]any{
		"error": map[string]any{
			"code":    "admin_required",
			"message": "admin role is required for this endpoint",
		},
	})
	return false
}

func roleFromRequest(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmpty(r.Header.Get("X-Role"), sampleapp.DefaultRole)))
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, nowUTC().UnixNano())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
