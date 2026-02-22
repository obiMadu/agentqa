package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

func decodeJSON(r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) errorJSON(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	if err != nil && s != nil && s.logger != nil {
		if r != nil {
			s.logger.Printf("request error: %s %s -> %d %s: %v", r.Method, r.URL.Path, status, message, err)
		} else {
			s.logger.Printf("request error -> %d %s: %v", status, message, err)
		}
	}

	writeJSON(w, status, map[string]string{"error": message})
}

func parseBearer(header string) string {
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return ""
	}

	if !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}
