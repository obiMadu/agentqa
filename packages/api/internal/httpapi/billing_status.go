package httpapi

import (
	"net/http"
	"time"

	"github.com/agentqa/agentqa/packages/api/internal/store"
)

func (s *Server) handleBillingStatus(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	if userID == "" {
		s.errorJSON(w, r, http.StatusUnauthorized, "missing user context", nil)
		return
	}

	now := time.Now().UTC()
	planValue, entitlements, override, err := s.resolvePlan(r.Context(), userID, now)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "plan resolve failed", err)
		return
	}

	periodStart := utcMonthStart(now)
	usage, err := s.store.GetBillingUsageMonthly(r.Context(), userID, periodStart)
	if err != nil {
		if err != store.ErrNotFound {
			s.errorJSON(w, r, http.StatusInternalServerError, "usage lookup failed", err)
			return
		}
		usage = store.BillingUsageMonthly{UserID: userID, PeriodStart: periodStart, QuestionRequests: 0}
	}

	activeAgents, err := s.store.CountActivePluginInstalls(r.Context(), userID)
	if err != nil {
		s.errorJSON(w, r, http.StatusInternalServerError, "active agent count failed", err)
		return
	}

	monthLimit := 0
	agentLimit := 0
	if planValue == planFree {
		monthLimit = 10
		agentLimit = 1
	}

	var proExpiresAt *string
	if entitlements.ProExpiresAt != nil {
		value := entitlements.ProExpiresAt.UTC().Format(time.RFC3339)
		proExpiresAt = &value
	}

	writeJSON(w, http.StatusOK, BillingStatusResponse{
		Plan:             string(planValue),
		ProActive:        planValue == planPro,
		ProExpiresAt:     proExpiresAt,
		TrialUsed:        entitlements.TrialUsedAt != nil,
		MonthPeriodStart: periodStart.Format(time.RFC3339),
		MonthLimit:       monthLimit,
		MonthUsed:        usage.QuestionRequests,
		AgentLimit:       agentLimit,
		ActiveAgents:     activeAgents,
		Override:         override,
	})
}

func utcMonthStart(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}
