package httpapi

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/agentqa/agentqa/packages/api/internal/auth"
	"github.com/agentqa/agentqa/packages/api/internal/config"
	"github.com/agentqa/agentqa/packages/api/internal/push"
	"github.com/agentqa/agentqa/packages/api/internal/store"
	"github.com/agentqa/agentqa/packages/api/internal/waiter"
)

type Server struct {
	cfg                 config.Config
	store               store.Store
	wait                *waiter.Hub
	push                push.Sender
	oidcVerifier        *auth.OIDCVerifier
	apiKeyEncryptionKey []byte
	logger              *log.Logger
}

func NewServer(cfg config.Config, store store.Store, wait *waiter.Hub, push push.Sender, oidcVerifier *auth.OIDCVerifier, logger *log.Logger, apiKeyEncryptionKey []byte) *Server {
	return &Server{
		cfg:                 cfg,
		store:               store,
		wait:                wait,
		push:                push,
		oidcVerifier:        oidcVerifier,
		apiKeyEncryptionKey: apiKeyEncryptionKey,
		logger:              logger,
	}
}

func (s *Server) Routes() http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(s.cors)

	router.Get("/healthz", s.handleHealth)
	router.Post("/billing/superwall/webhook", s.handleSuperwallWebhook)

	router.Group(func(r chi.Router) {
		r.Use(s.userAuth)
		r.Get("/me", s.handleMe)
		r.Get("/api-key", s.handleGetAPIKey)
		r.Get("/api-key/raw", s.handleGetRawAPIKey)
		r.Post("/api-key/reset", s.handleResetAPIKey)
		r.Get("/ui-preferences", s.handleGetUIPreferences)
		r.Put("/ui-preferences", s.handleUpdateUIPreferences)
		r.Post("/devices", s.handleRegisterDevice)
		r.Get("/plugins", s.handleListPluginInstalls)
		r.Patch("/plugins/{id}", s.handleUpdatePluginInstall)
		r.Post("/plugins/{id}/pair", s.handlePairPluginInstall)
		r.Get("/questions", s.handleListQuestions)
		r.Get("/questions/{id}", s.handleGetQuestion)
	})

	router.Group(func(r chi.Router) {
		r.Use(s.apiKeyAuth)
		r.Post("/plugin/register", s.handlePluginRegister)
		r.Post("/questions", s.handleCreateQuestion)
		r.Get("/questions/{id}/wait", s.handleWaitQuestion)
	})

	router.Group(func(r chi.Router) {
		r.Use(s.userOrAPIKeyAuth)
		r.Post("/questions/{id}/answers", s.handleAnswerQuestion)
		r.Post("/questions/{id}/reject", s.handleRejectQuestion)
	})

	return router
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
