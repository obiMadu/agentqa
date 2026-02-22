package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentqa/agentqa/packages/api/internal/auth"
	"github.com/agentqa/agentqa/packages/api/internal/push"
	"github.com/agentqa/agentqa/packages/api/internal/store"
	"github.com/agentqa/agentqa/packages/api/internal/waiter"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	if userID == "" {
		s.errorJSON(w, r, http.StatusUnauthorized, "missing user context", nil)
		return
	}

	user, err := s.store.GetUser(r.Context(), userID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "user not found", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "user lookup failed", err)
		return
	}

	writeJSON(w, http.StatusOK, UserPayload{ID: user.ID, Email: user.Email, Name: user.Name})
}

func (s *Server) handleGetAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	key, err := s.store.GetActiveAPIKey(r.Context(), userID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "api key not found", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "api key lookup failed", err)
		return
	}

	var lastUsedAt *string
	if key.LastUsedAt != nil {
		value := key.LastUsedAt.Format(time.RFC3339)
		lastUsedAt = &value
	}

	writeJSON(w, http.StatusOK, APIKeyInfoResponse{
		KeyPrefix:  key.KeyPrefix,
		CreatedAt:  key.CreatedAt.Format(time.RFC3339),
		LastUsedAt: lastUsedAt,
	})
}

func (s *Server) handleGetRawAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	key, err := s.store.GetActiveAPIKey(r.Context(), userID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "api key not found", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "api key lookup failed", err)
		return
	}

	if key.KeyCiphertext == nil || *key.KeyCiphertext == "" || key.KeyNonce == nil || *key.KeyNonce == "" {
		s.errorJSON(w, r, http.StatusNotFound, "raw api key not available", nil)
		return
	}

	plain, err := auth.DecryptAPIKey(*key.KeyCiphertext, *key.KeyNonce, s.apiKeyEncryptionKey)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "api key decrypt failed", err)
		return
	}

	writeJSON(w, http.StatusOK, APIKeyRawResponse{APIKey: plain})
}

func (s *Server) handleResetAPIKey(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	if err := s.store.RevokeActiveAPIKeys(r.Context(), userID); err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "api key revoke failed", err)
		return
	}

	plain, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "api key generation failed", err)
		return
	}

	ciphertext, nonce, err := auth.EncryptAPIKey(plain, s.apiKeyEncryptionKey)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "api key encryption failed", err)
		return
	}

	key, err := s.store.CreateAPIKey(
		r.Context(),
		userID,
		"Primary",
		hash,
		prefix,
		ciphertext,
		nonce,
		[]string{"questions:write", "questions:wait"},
	)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "api key store failed", err)
		return
	}

	writeJSON(w, http.StatusCreated, ResetAPIKeyResponse{
		APIKey:    plain,
		KeyPrefix: key.KeyPrefix,
		CreatedAt: key.CreatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleGetUIPreferences(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	prefs, err := s.store.GetUIPreferences(r.Context(), userID)
	if err != nil {
		if err == store.ErrNotFound {
			prefs, err = s.store.UpsertUIPreferences(r.Context(), userID, defaultUIPreferences())
			if err != nil {
				s.errorJSON(w, r, http.StatusInternalServerError, "ui preferences create failed", err)
				return
			}
		} else {
			s.errorJSON(w, r, http.StatusInternalServerError, "ui preferences lookup failed", err)
			return
		}
	}

	updatedData, changed := applyUIPreferencesDefaults(prefs.Data)
	if changed {
		prefs, err = s.store.UpsertUIPreferences(r.Context(), userID, updatedData)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "ui preferences update failed", err)
			return
		}
	}

	writeJSON(w, http.StatusOK, UIPreferencesResponse{
		Preferences: prefs.Data,
		UpdatedAt:   prefs.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handleUpdateUIPreferences(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req UIPreferencesRequest
	if err := decodeJSON(r, &req); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if err := validateUIPreferences(req.Preferences); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, err.Error(), err)
		return
	}

	prefs, err := s.store.UpsertUIPreferences(r.Context(), userID, req.Preferences)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "ui preferences update failed", err)
		return
	}

	writeJSON(w, http.StatusOK, UIPreferencesResponse{
		Preferences: prefs.Data,
		UpdatedAt:   prefs.UpdatedAt.Format(time.RFC3339),
	})
}

func (s *Server) handlePluginRegister(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	var req PluginRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.InstallID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "install_id is required", nil)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "OpenCode"
	}

	keyIDValue, ok := r.Context().Value(ctxAPIKeyID).(string)
	if !ok || keyIDValue == "" {
		s.errorJSON(w, r, http.StatusInternalServerError, "api key context missing", nil)
		return
	}
	keyID := &keyIDValue

	now := time.Now().UTC()
	planValue, _, _, err := s.resolvePlan(r.Context(), userID, now)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "plan resolve failed", err)
		return
	}

	active := true
	if planValue == planFree {
		activeCount, err := s.store.CountActivePluginInstalls(r.Context(), userID)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "active agent count failed", err)
			return
		}
		if activeCount >= 1 {
			active = false
		}
	}

	if err := s.store.UpsertPluginInstall(r.Context(), store.PluginInstall{
		InstallID: req.InstallID,
		UserID:    userID,
		Name:      name,
		KeyID:     keyID,
		Active:    active,
	}); err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "install link failed", err)
		return
	}

	if planValue == planFree {
		_, _ = s.store.EnforceSingleActivePluginInstall(r.Context(), userID)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"linked": true})
}

func (s *Server) handleListPluginInstalls(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	installs, err := s.store.ListPluginInstalls(r.Context(), userID)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "plugin list failed", err)
		return
	}

	payload := make([]PluginInstallPayload, 0, len(installs))
	for _, install := range installs {
		var lastSeenAt *string
		if install.LastSeenAt != nil {
			value := install.LastSeenAt.Format(time.RFC3339)
			lastSeenAt = &value
		}
		var pairingRequestedAt *string
		if install.PairingRequestedAt != nil {
			value := install.PairingRequestedAt.Format(time.RFC3339)
			pairingRequestedAt = &value
		}
		var pairedAt *string
		if install.PairedAt != nil {
			value := install.PairedAt.Format(time.RFC3339)
			pairedAt = &value
		}
		payload = append(payload, PluginInstallPayload{
			InstallID:          install.InstallID,
			Name:               install.Name,
			Active:             install.Active,
			Paired:             install.Paired,
			PairingRequestedAt: pairingRequestedAt,
			PairedAt:           pairedAt,
			CreatedAt:          install.CreatedAt.Format(time.RFC3339),
			LastSeenAt:         lastSeenAt,
		})
	}

	writeJSON(w, http.StatusOK, ListPluginInstallsResponse{Installs: payload})
}

func (s *Server) handleUpdatePluginInstall(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	installID := chi.URLParam(r, "id")
	if installID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing install id", nil)
		return
	}

	var req UpdatePluginInstallRequest
	if err := decodeJSON(r, &req); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if req.Active == nil {
		s.errorJSON(w, r, http.StatusBadRequest, "active is required", nil)
		return
	}

	if *req.Active {
		now := time.Now().UTC()
		planValue, _, _, err := s.resolvePlan(r.Context(), userID, now)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "plan resolve failed", err)
			return
		}

		if planValue == planFree {
			if err := s.store.ActivatePluginInstallExclusive(r.Context(), userID, installID); err != nil {
				if err == store.ErrNotFound {
					s.errorJSON(w, r, http.StatusNotFound, "install not found", err)
					return
				}
				s.errorJSON(w, r, http.StatusInternalServerError, "install update failed", err)
				return
			}
		} else {
			if err := s.store.UpdatePluginInstallActive(r.Context(), userID, installID, true); err != nil {
				if err == store.ErrNotFound {
					s.errorJSON(w, r, http.StatusNotFound, "install not found", err)
					return
				}
				s.errorJSON(w, r, http.StatusInternalServerError, "install update failed", err)
				return
			}
		}
	} else {
		if err := s.store.UpdatePluginInstallActive(r.Context(), userID, installID, false); err != nil {
			if err == store.ErrNotFound {
				s.errorJSON(w, r, http.StatusNotFound, "install not found", err)
				return
			}
			s.errorJSON(w, r, http.StatusInternalServerError, "install update failed", err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"updated":    true,
		"install_id": installID,
		"active":     *req.Active,
	})
}

func (s *Server) handlePairPluginInstall(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	installID := chi.URLParam(r, "id")
	if installID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing install id", nil)
		return
	}

	if err := s.store.PairPluginInstall(r.Context(), userID, installID); err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "install not found", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "install pair failed", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"paired":     true,
		"install_id": installID,
	})
}

func (s *Server) handleCreateQuestion(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	var req CreateQuestionRequest
	if err := decodeJSON(r, &req); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.RequestID == "" || len(req.Questions) == 0 {
		s.errorJSON(w, r, http.StatusBadRequest, "request_id and questions are required", nil)
		return
	}

	if req.InstallID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "install_id is required", nil)
		return
	}

	install, err := s.store.GetPluginInstall(r.Context(), req.InstallID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusForbidden, "install_id not registered", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "install lookup failed", err)
		return
	}
	if install.UserID != userID {
		s.errorJSON(w, r, http.StatusForbidden, "install_id not registered", nil)
		return
	}
	if !install.Active {
		s.errorJSON(w, r, http.StatusForbidden, "install disabled", nil)
		return
	}
	if !install.Paired {
		if err := s.store.RequestPluginInstallPairing(r.Context(), userID, req.InstallID); err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "pairing request failed", err)
			return
		}
		s.errorJSON(w, r, http.StatusPreconditionRequired, "pairing required", nil)
		return
	}

	_ = s.store.UpsertPluginInstall(r.Context(), store.PluginInstall{
		InstallID: req.InstallID,
		UserID:    userID,
		Name:      install.Name,
		KeyID:     install.KeyID,
		Active:    install.Active,
	})

	payload, err := json.Marshal(struct {
		Questions []QuestionPrompt `json:"questions"`
		Tool      *QuestionTool    `json:"tool,omitempty"`
	}{Questions: req.Questions, Tool: req.Tool})
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "payload encode failed", err)
		return
	}

	now := time.Now().UTC()
	planValue, _, _, err := s.resolvePlan(r.Context(), userID, now)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "plan resolve failed", err)
		return
	}

	monthlyLimit := 0
	if planValue == planFree {
		monthlyLimit = 10
	}

	created, err := s.store.CreateQuestion(r.Context(), store.Question{
		ID:        req.RequestID,
		UserID:    userID,
		InstallID: req.InstallID,
		SessionID: req.SessionID,
		Payload:   payload,
		Status:    "pending",
	}, utcMonthStart(now), monthlyLimit)
	if err != nil {
		if err == store.ErrQuotaExceeded {
			s.errorJSON(w, r, http.StatusPaymentRequired, "quota exceeded", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "question create failed", err)
		return
	}
	if !created {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"created":     false,
			"question_id": req.RequestID,
		})
		return
	}

	s.logger.Printf("[Server] Received question: %s", req.RequestID)

	preview := ""
	if len(req.Questions) > 0 {
		preview = req.Questions[0].Question
		if preview == "" {
			preview = req.Questions[0].Header
		}
	}
	_ = s.push.SendQuestion(r.Context(), push.Message{
		UserID:     userID,
		QuestionID: req.RequestID,
		Preview:    preview,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"created":     true,
		"question_id": req.RequestID,
	})
}

func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	questions, err := s.store.ListQuestions(r.Context(), userID)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "question list failed", err)
		return
	}

	items := make([]QuestionListItem, 0, len(questions))
	installNames := make(map[string]string)
	for _, question := range questions {
		var payload struct {
			Questions []QuestionPrompt `json:"questions"`
		}
		if err := json.Unmarshal(question.Payload, &payload); err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "payload decode failed", err)
			return
		}
		if question.InstallID == "" {
			s.errorJSON(w, r, http.StatusInternalServerError, "install_id missing for question", nil)
			return
		}
		installName, ok := installNames[question.InstallID]
		if !ok {
			install, err := s.store.GetPluginInstall(r.Context(), question.InstallID)
			if err != nil {
				if err == store.ErrNotFound {
					s.errorJSON(w, r, http.StatusForbidden, "install_id not registered", err)
					return
				}
				s.errorJSON(w, r, http.StatusInternalServerError, "install lookup failed", err)
				return
			}
			installName = strings.TrimSpace(install.Name)
			if installName == "" {
				installName = "OpenCode"
			}
			installNames[question.InstallID] = installName
		}
		preview := ""
		if len(payload.Questions) > 0 {
			preview = payload.Questions[0].Question
			if preview == "" {
				preview = payload.Questions[0].Header
			}
		}
		questionCount := len(payload.Questions)

		items = append(items, QuestionListItem{
			ID:            question.ID,
			Status:        question.Status,
			SessionID:     question.SessionID,
			InstallID:     question.InstallID,
			Preview:       preview,
			QuestionCount: questionCount,
			Source:        installName,
			CreatedAt:     question.CreatedAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, ListQuestionsResponse{Questions: items})
}

func (s *Server) handleGetQuestion(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	questionID := chi.URLParam(r, "id")
	if questionID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing question id", nil)
		return
	}

	question, err := s.store.GetQuestion(r.Context(), userID, questionID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "question not found", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "question fetch failed", err)
		return
	}

	var payload struct {
		Questions []QuestionPrompt `json:"questions"`
	}
	if err := json.Unmarshal(question.Payload, &payload); err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "payload decode failed", err)
		return
	}

	response := QuestionResponse{
		ID:        question.ID,
		Status:    question.Status,
		SessionID: question.SessionID,
		InstallID: question.InstallID,
		Questions: payload.Questions,
		CreatedAt: question.CreatedAt.Format(time.RFC3339),
	}

	if question.Status == "answered" {
		answer, err := s.store.GetAnswer(r.Context(), questionID)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "answer lookup failed", err)
			return
		}
		answers, err := decodeAnswers(answer.Body)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "answer decode failed", err)
			return
		}
		response.Answers = answers
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAnswerQuestion(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	questionID := chi.URLParam(r, "id")
	if questionID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing question id", nil)
		return
	}

	var req AnswerRequest
	if err := decodeJSON(r, &req); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if len(req.Answers) == 0 {
		s.errorJSON(w, r, http.StatusBadRequest, "answers are required", nil)
		return
	}

	if _, err := s.store.AnswerQuestion(r.Context(), questionID, userID, req.Answers); err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "question not found", err)
			return
		}
		if err == store.ErrAlreadyResolved {
			s.errorJSON(w, r, http.StatusConflict, "question already resolved", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "answer failed", err)
		return
	}

	s.wait.Notify(questionID, waiter.Result{Status: "answered", Answers: req.Answers})

	s.logger.Printf("[Server] Answer received for %s", questionID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"answered":    true,
		"question_id": questionID,
	})
}

func (s *Server) handleRejectQuestion(w http.ResponseWriter, r *http.Request) {
	questionID := chi.URLParam(r, "id")
	if questionID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing question id", nil)
		return
	}

	userID := userIDFromContext(r.Context())
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	if err := s.store.RejectQuestion(r.Context(), questionID, userIDPtr); err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "question not found", err)
			return
		}
		if err == store.ErrAlreadyResolved {
			s.errorJSON(w, r, http.StatusConflict, "question already resolved", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "reject failed", err)
		return
	}

	s.wait.Notify(questionID, waiter.Result{Status: "rejected"})

	s.logger.Printf("[Server] Question rejected: %s", questionID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rejected":    true,
		"question_id": questionID,
	})
}

func (s *Server) handleWaitQuestion(w http.ResponseWriter, r *http.Request) {
	questionID := chi.URLParam(r, "id")
	if questionID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing question id", nil)
		return
	}

	userID := userIDFromContext(r.Context())
	installID := r.URL.Query().Get("install_id")
	if installID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "install_id is required", nil)
		return
	}

	install, err := s.store.GetPluginInstall(r.Context(), installID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusForbidden, "install_id not registered", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "install lookup failed", err)
		return
	}
	if install.UserID != userID {
		s.errorJSON(w, r, http.StatusForbidden, "install_id not registered", nil)
		return
	}
	question, err := s.store.GetQuestion(r.Context(), userID, questionID)
	if err != nil {
		if err == store.ErrNotFound {
			s.errorJSON(w, r, http.StatusNotFound, "question not found", err)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "question fetch failed", err)
		return
	}

	if question.Status == "answered" {
		answer, err := s.store.GetAnswer(r.Context(), questionID)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "answer lookup failed", err)
			return
		}
		answers, err := decodeAnswers(answer.Body)
		if err != nil {
			s.errorJSON(w, r, http.StatusInternalServerError, "answer decode failed", err)
			return
		}
		writeJSON(w, http.StatusOK, WaitResponse{Status: "answered", Answers: answers, QuestionID: questionID})
		return
	}

	if question.Status == "rejected" {
		writeJSON(w, http.StatusOK, WaitResponse{Status: "rejected", QuestionID: questionID})
		return
	}

	if s.cfg.QuestionTTL > 0 && time.Since(question.CreatedAt) > s.cfg.QuestionTTL {
		w.WriteHeader(http.StatusGone)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()

	result, err := s.wait.Wait(ctx, questionID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.errorJSON(w, r, http.StatusInternalServerError, "wait failed", err)
		return
	}

	writeJSON(w, http.StatusOK, WaitResponse{
		Status:     result.Status,
		Answers:    result.Answers,
		QuestionID: questionID,
	})
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	var req RegisterDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if req.Platform == "" || req.PushToken == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "platform and push_token are required", nil)
		return
	}

	if err := s.store.UpsertDevice(r.Context(), store.Device{
		UserID:    userID,
		Platform:  req.Platform,
		PushToken: req.PushToken,
	}); err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "device register failed", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"registered": true})
}

func decodeAnswers(body string) ([][]string, error) {
	var answers [][]string
	if err := json.Unmarshal([]byte(body), &answers); err != nil {
		return nil, err
	}
	return answers, nil
}

func defaultUIPreferences() store.UIPreferencesData {
	return store.UIPreferencesData{
		Background: store.BackgroundPreferences{
			Mode: store.BackgroundModeShuffle,
		},
		Theme: &store.ThemePreferences{
			Accent: store.ThemeAccentGreen,
		},
		Questions: &store.QuestionPreferences{
			DisplayMode: store.QuestionDisplayStacked,
		},
	}
}

func applyUIPreferencesDefaults(data store.UIPreferencesData) (store.UIPreferencesData, bool) {
	changed := false
	if strings.TrimSpace(data.Background.Mode) == "" {
		data.Background.Mode = store.BackgroundModeShuffle
		changed = true
	}
	if data.Theme == nil {
		data.Theme = &store.ThemePreferences{Accent: store.ThemeAccentGreen}
		changed = true
	} else if strings.TrimSpace(data.Theme.Accent) == "" {
		data.Theme.Accent = store.ThemeAccentGreen
		changed = true
	}
	if data.Questions == nil {
		data.Questions = &store.QuestionPreferences{DisplayMode: store.QuestionDisplayStacked}
		changed = true
	} else {
		displayMode := strings.TrimSpace(data.Questions.DisplayMode)
		if displayMode == "" {
			data.Questions.DisplayMode = store.QuestionDisplayStacked
			changed = true
		} else if _, ok := store.ValidQuestionDisplayModes[displayMode]; !ok {
			data.Questions.DisplayMode = store.QuestionDisplayStacked
			changed = true
		}
	}
	return data, changed
}

func validateUIPreferences(prefs store.UIPreferencesData) error {
	mode := strings.TrimSpace(prefs.Background.Mode)
	if mode == "" {
		return errors.New("background.mode is required")
	}
	if mode != store.BackgroundModeShuffle && mode != store.BackgroundModeFixed {
		return errors.New("background.mode must be shuffle or fixed")
	}
	if mode == store.BackgroundModeFixed && strings.TrimSpace(prefs.Background.PatternID) == "" {
		return errors.New("background.pattern_id is required when mode is fixed")
	}
	if prefs.Theme == nil {
		return errors.New("theme is required")
	}
	accent := strings.TrimSpace(prefs.Theme.Accent)
	if accent == "" {
		return errors.New("theme.accent is required")
	}
	if _, ok := store.ValidThemeAccents[accent]; !ok {
		return errors.New("theme.accent is invalid")
	}
	if prefs.Questions != nil {
		displayMode := strings.TrimSpace(prefs.Questions.DisplayMode)
		if displayMode != "" {
			if _, ok := store.ValidQuestionDisplayModes[displayMode]; !ok {
				return errors.New("questions.display_mode is invalid")
			}
		}
	}
	return nil
}
