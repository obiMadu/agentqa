package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	svixHeaderID        = "svix-id"
	svixHeaderTimestamp = "svix-timestamp"
	svixHeaderSignature = "svix-signature"
)

type superwallWebhookEvent struct {
	Data struct {
		ID                string `json:"id"`
		OriginalAppUserID string `json:"originalAppUserId"`
		UserAttributes    struct {
			AgentQAUserID string `json:"agentqa_user_id"`
		} `json:"userAttributes"`
		ExpirationAt *string `json:"expirationAt"`
		PeriodType   string  `json:"periodType"`
		Name         string  `json:"name"`
	} `json:"data"`
}

func (s *Server) handleSuperwallWebhook(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(s.cfg.SuperwallWebhookSecret)
	if secret == "" {
		s.errorJSON(w, r, http.StatusInternalServerError, "webhook not configured", nil)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if err := verifySvixSignature(secret, r.Header, body, time.Now()); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid webhook signature", err)
		return
	}

	var event superwallWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		s.errorJSON(w, r, http.StatusBadRequest, "invalid webhook payload", err)
		return
	}

	eventID := strings.TrimSpace(event.Data.ID)
	if eventID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing event id", nil)
		return
	}

	userID := extractSuperwallUserID(event)
	if userID == "" {
		s.errorJSON(w, r, http.StatusBadRequest, "missing user id", nil)
		return
	}

	now := time.Now().UTC()
	var proExpiresAt *time.Time
	proActive := false
	if event.Data.ExpirationAt != nil && strings.TrimSpace(*event.Data.ExpirationAt) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*event.Data.ExpirationAt))
		if err != nil {
			s.errorJSON(w, r, http.StatusBadRequest, "invalid expirationAt", err)
			return
		}
		parsed = parsed.UTC()
		proExpiresAt = &parsed
		proActive = parsed.After(now)
	}

	markTrialUsed := strings.EqualFold(strings.TrimSpace(event.Data.PeriodType), "TRIAL") && strings.TrimSpace(event.Data.Name) == "initial_purchase"

	_, err = s.store.UpsertBillingEntitlements(r.Context(), userID, proActive, proExpiresAt, markTrialUsed)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "entitlements update failed", err)
		return
	}

	inserted, err := s.store.StoreSuperwallWebhookEvent(r.Context(), eventID, body)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "webhook event store failed", err)
		return
	}
	if !inserted {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func extractSuperwallUserID(event superwallWebhookEvent) string {
	userID := strings.TrimSpace(event.Data.OriginalAppUserID)
	userID = strings.TrimPrefix(userID, "$SuperwallAlias:")
	userID = strings.TrimSpace(userID)
	if userID != "" {
		return userID
	}
	return strings.TrimSpace(event.Data.UserAttributes.AgentQAUserID)
}

func verifySvixSignature(secret string, headers http.Header, body []byte, now time.Time) error {
	svixID := strings.TrimSpace(headers.Get(svixHeaderID))
	svixTimestamp := strings.TrimSpace(headers.Get(svixHeaderTimestamp))
	svixSignature := strings.TrimSpace(headers.Get(svixHeaderSignature))
	if svixID == "" || svixTimestamp == "" || svixSignature == "" {
		return errors.New("missing svix headers")
	}

	ts, err := strconv.ParseInt(svixTimestamp, 10, 64)
	if err != nil {
		return errors.New("invalid svix timestamp")
	}
	const maxAgeSeconds = int64(5 * 60)
	age := now.Unix() - ts
	if age < -maxAgeSeconds || age > maxAgeSeconds {
		return errors.New("svix timestamp outside allowed window")
	}

	secretBytes, err := decodeSvixSecret(secret)
	if err != nil {
		return err
	}

	signed := svixID + "." + svixTimestamp + "." + string(body)
	h := hmac.New(sha256.New, secretBytes)
	_, _ = h.Write([]byte(signed))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))

	for _, part := range strings.Fields(svixSignature) {
		if !strings.HasPrefix(part, "v1,") {
			continue
		}
		candidate := strings.TrimPrefix(part, "v1,")
		if hmac.Equal([]byte(candidate), []byte(expected)) {
			return nil
		}
	}

	return errors.New("signature mismatch")
}

func decodeSvixSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, "whsec_") {
		return nil, errors.New("invalid webhook secret")
	}
	encoded := strings.TrimPrefix(secret, "whsec_")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid webhook secret")
	}
	return decoded, nil
}
