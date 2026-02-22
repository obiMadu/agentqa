package httpapi

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentqa/agentqa/packages/api/internal/store"
)

type plan string

const (
	planFree plan = "free"
	planPro  plan = "pro"
)

func (s *Server) resolvePlan(ctx context.Context, userID string, now time.Time) (plan, store.BillingEntitlements, *BillingOverridePayload, error) {
	entitlements, err := s.store.GetBillingEntitlements(ctx, userID)
	if err != nil {
		if err == store.ErrNotFound {
			entitlements = store.BillingEntitlements{UserID: userID, ProActive: false}
		} else {
			return planFree, store.BillingEntitlements{}, nil, err
		}
	}

	overridePlan, overrideApplies, err := s.subscriptionOverride(userID)
	if err != nil {
		return planFree, store.BillingEntitlements{}, nil, err
	}
	if overrideApplies {
		override := &BillingOverridePayload{Plan: string(overridePlan)}
		return overridePlan, entitlements, override, nil
	}

	return effectivePlanFromEntitlements(entitlements, now), entitlements, nil, nil
}

func effectivePlanFromEntitlements(entitlements store.BillingEntitlements, now time.Time) plan {
	if entitlements.ProExpiresAt != nil {
		if entitlements.ProExpiresAt.After(now) {
			return planPro
		}
		return planFree
	}
	if entitlements.ProActive {
		return planPro
	}
	return planFree
}

func (s *Server) subscriptionOverride(userID string) (plan, bool, error) {
	overridePlan := strings.ToLower(strings.TrimSpace(s.cfg.SubscriptionOverridePlan))
	if overridePlan == "" {
		return planFree, false, nil
	}
	if overridePlan != string(planFree) && overridePlan != string(planPro) {
		return planFree, false, fmt.Errorf("invalid SUBSCRIPTION_OVERRIDE_PLAN")
	}

	if len(s.cfg.SubscriptionOverrideUserIDs) == 0 {
		return plan(overridePlan), true, nil
	}
	for _, candidate := range s.cfg.SubscriptionOverrideUserIDs {
		if strings.TrimSpace(candidate) == userID {
			return plan(overridePlan), true, nil
		}
	}

	return planFree, false, nil
}
