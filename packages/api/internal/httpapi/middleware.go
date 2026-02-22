package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/agentqa/agentqa/packages/api/internal/auth"
	"github.com/agentqa/agentqa/packages/api/internal/store"
)

type contextKey string

const (
	ctxUserID   contextKey = "user_id"
	ctxAPIKeyID contextKey = "api_key_id"
)

func (s *Server) userAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := parseBearer(r.Header.Get("Authorization"))
		if token == "" {
			s.errorJSON(w, r, http.StatusUnauthorized, "missing access token", nil)
			return
		}

		claims, err := s.tokens.Parse(token)
		if err != nil {
			s.errorJSON(w, r, http.StatusUnauthorized, "invalid access token", err)
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := parseBearer(r.Header.Get("Authorization"))
		if raw == "" {
			s.errorJSON(w, r, http.StatusUnauthorized, "missing api key", nil)
			return
		}

		keyHash := auth.HashAPIKey(raw)
		apiKey, err := s.store.LookupAPIKey(r.Context(), keyHash)
		if err != nil {
			if err == store.ErrNotFound {
				s.errorJSON(w, r, http.StatusUnauthorized, "invalid api key", err)
				return
			}
			s.errorJSON(w, r, http.StatusInternalServerError, "api key lookup failed", err)
			return
		}

		_ = s.store.UpdateAPIKeyLastUsed(r.Context(), apiKey.ID)

		ctx := context.WithValue(r.Context(), ctxUserID, apiKey.UserID)
		ctx = context.WithValue(ctx, ctxAPIKeyID, apiKey.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) userOrAPIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := parseBearer(r.Header.Get("Authorization"))
		if raw == "" {
			s.errorJSON(w, r, http.StatusUnauthorized, "missing access token", nil)
			return
		}

		if strings.Count(raw, ".") == 2 {
			claims, err := s.tokens.Parse(raw)
			if err != nil {
				s.errorJSON(w, r, http.StatusUnauthorized, "invalid access token", err)
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		keyHash := auth.HashAPIKey(raw)
		apiKey, err := s.store.LookupAPIKey(r.Context(), keyHash)
		if err != nil {
			if err == store.ErrNotFound {
				s.errorJSON(w, r, http.StatusUnauthorized, "invalid api key", err)
				return
			}
			s.errorJSON(w, r, http.StatusInternalServerError, "api key lookup failed", err)
			return
		}

		_ = s.store.UpdateAPIKeyLastUsed(r.Context(), apiKey.ID)

		ctx := context.WithValue(r.Context(), ctxUserID, apiKey.UserID)
		ctx = context.WithValue(ctx, ctxAPIKeyID, apiKey.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(ctxUserID).(string); ok {
		return value
	}
	return ""
}
