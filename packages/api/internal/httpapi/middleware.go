package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/agentqa/agentqa/packages/api/internal/auth"
	"github.com/agentqa/agentqa/packages/api/internal/store"
)

type unauthorizedError struct {
	cause error
}

func (e unauthorizedError) Error() string {
	if e.cause == nil {
		return "unauthorized"
	}
	return e.cause.Error()
}

func (e unauthorizedError) Unwrap() error {
	return e.cause
}

func isUnauthorized(err error) bool {
	var u unauthorizedError
	return errors.As(err, &u)
}

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

		user, err := s.authenticateOIDCUser(r.Context(), token)
		if err != nil {
			if isUnauthorized(err) {
				s.errorJSON(w, r, http.StatusUnauthorized, "invalid access token", err)
				return
			}
			s.errorJSON(w, r, http.StatusInternalServerError, "user auth failed", err)
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, user.ID)
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

		if !strings.HasPrefix(raw, auth.APIKeyPrefix) {
			user, err := s.authenticateOIDCUser(r.Context(), raw)
			if err != nil {
				if isUnauthorized(err) {
					s.errorJSON(w, r, http.StatusUnauthorized, "invalid access token", err)
					return
				}
				s.errorJSON(w, r, http.StatusInternalServerError, "user auth failed", err)
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserID, user.ID)
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

func (s *Server) authenticateOIDCUser(ctx context.Context, accessToken string) (store.User, error) {
	verified, err := s.oidcVerifier.VerifyAccessToken(ctx, accessToken)
	if err != nil {
		return store.User{}, unauthorizedError{cause: err}
	}

	user, err := s.store.GetUserByOIDC(ctx, verified.Issuer, verified.Subject)
	if err == nil {
		return user, nil
	}
	if err != store.ErrNotFound {
		return store.User{}, err
	}

	userInfo, err := s.oidcVerifier.UserInfo(ctx, accessToken)
	if err != nil {
		return store.User{}, unauthorizedError{cause: err}
	}

	email := strings.TrimSpace(userInfo.Email)
	if email == "" {
		return store.User{}, unauthorizedError{cause: errors.New("missing email")}
	}

	name := strings.TrimSpace(userInfo.Name)
	issuer := verified.Issuer
	subject := verified.Subject

	user, err = s.store.UpsertUser(ctx, store.User{
		Email:        email,
		Name:         name,
		AuthProvider: "oidc",
		AuthIssuer:   &issuer,
		AuthSubject:  &subject,
	})
	if err != nil {
		return store.User{}, err
	}

	if err := s.store.AttachOIDCToUser(ctx, user.ID, verified.Issuer, verified.Subject); err != nil {
		return store.User{}, err
	}

	return user, nil
}

func userIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(ctxUserID).(string); ok {
		return value
	}
	return ""
}
