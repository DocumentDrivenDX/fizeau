package fizeau

import (
	"context"
	"time"

	"github.com/easel/fizeau/internal/routehealth"
)

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
	cooldown := routehealth.CooldownFromRecord(record, ttl)
	return &CooldownState{
		Reason:      cooldown.Reason,
		Until:       cooldown.Until,
		FailCount:   cooldown.FailCount,
		LastError:   cooldown.LastError,
		LastAttempt: cooldown.LastAttempt,
	}
}

func (s *service) routeMetricSignals(now time.Time, ttl time.Duration) (map[string]float64, map[string]float64) {
	if s == nil || s.routeHealth == nil {
		return nil, nil
	}
	return s.routeHealth.MetricSignals(now, ttl)
}
