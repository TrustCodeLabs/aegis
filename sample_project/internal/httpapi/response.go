package httpapi

import (
	"net/http"

	"aegis/restadapter"
)

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := restadapter.ClassifyError(err)

	s.writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    err.Error(),
			"request_id": firstNonEmpty(r.Header.Get("X-Request-ID"), ""),
			"trace_id":   firstNonEmpty(r.Header.Get("X-Trace-ID"), ""),
		},
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	_ = restadapter.WriteJSON(w, status, payload)
}
