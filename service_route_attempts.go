package fizeau

import (
	"context"
	"time"

	"github.com/easel/fizeau/internal/routehealth"
)

const defaultRouteAttemptCooldown = routehealth.DefaultCooldown

func (s *service) RecordRouteAttempt(_ context.Context, attempt RouteAttempt) error {
	if s == nil {
		s = &service{}
	}
	return s.routeHealthStore().RecordAttempt(routehealth.Attempt{
		Harness:   attempt.Harness,
		Provider:  attempt.Provider,
		Model:     attempt.Model,
		Endpoint:  attempt.Endpoint,
		Status:    attempt.Status,
		Reason:    attempt.Reason,
		Error:     attempt.Error,
		Duration:  attempt.Duration,
		Timestamp: attempt.Timestamp,
	})
}

func (s *service) routeHealthStore() *routehealth.Store {
	if s.routeHealth == nil {
		s.routeHealth = routehealth.NewStore()
	}
	return s.routeHealth
}

func (s *service) activeRouteAttempts(now time.Time, ttl time.Duration) []routehealth.Record {
	if s == nil || s.routeHealth == nil {
		return nil
	}
	return s.routeHealth.ActiveAttempts(now, ttl)
}

func routeAttemptCooldown(record routehealth.Record, ttl time.Duration) *CooldownState {
	if ttl <= 0 {
		ttl = defaultRouteAttemptCooldown
	}
	reason := record.Reason
	if reason == "" {
		reason = "route_attempt_failure"
	}
	return &CooldownState{
		Reason:      reason,
		Until:       record.RecordedAt.Add(ttl),
		FailCount:   1,
		LastError:   record.Error,
		LastAttempt: record.RecordedAt,
	}
}

func (s *service) routeMetricSignals(now time.Time, ttl time.Duration) (map[string]float64, map[string]float64) {
	if s == nil || s.routeHealth == nil {
		return nil, nil
	}
	return s.routeHealth.MetricSignals(now, ttl)
}
