package httpapi

import (
	"fmt"
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

func (s *Server) writeMarkdown(w http.ResponseWriter, status int, payload any) error {
	var body []byte
	switch value := payload.(type) {
	case string:
		body = []byte(value)
	case []byte:
		body = value
	default:
		return fmt.Errorf("markdown response requires string or []byte payload, got %T", payload)
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(status)
	_, err := w.Write(body)
	return err
}
