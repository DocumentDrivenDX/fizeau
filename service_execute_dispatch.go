package fizeau

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DocumentDrivenDX/fizeau/internal/harnesses"
	claudeharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/claude"
	codexharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/codex"
	geminiharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/gemini"
	opencodeharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/opencode"
	piharness "github.com/DocumentDrivenDX/fizeau/internal/harnesses/pi"
)

type executeRouteResolver interface {
	resolveExecuteRoute(ServiceExecuteRequest) (*RouteDecision, error)
}

type executeSessionLogOpener interface {
	openSessionLog(ServiceExecuteRequest, RouteDecision, string) *serviceSessionLog
}

type executeEventFanout interface {
	openSession(string)
	broadcastEvent(string, ServiceEvent)
	closeSession(string, ServiceEvent)
	wrapExecuteWithHub(string, chan ServiceEvent, *overrideContext, map[string]string) chan ServiceEvent
}

type executeRunnerInvoker interface {
	dispatchExecuteRun(context.Context, executeRunContext)
}

type executeRunContext struct {
	req      ServiceExecuteRequest
	decision RouteDecision
	meta     map[string]string
	out      chan<- ServiceEvent
	seq      *atomic.Int64
	start    time.Time
	sl       *serviceSessionLog
	session  string
}

func (s *service) executeRouteResolver() executeRouteResolver {
	return s
}

func (s *service) executeSessionLogOpener() executeSessionLogOpener {
	return s
}

func (s *service) executeEventFanout() executeEventFanout {
	return s.hub
}

func (s *service) executeRunnerInvoker() executeRunnerInvoker {
	return s
}

func (s *service) dispatchExecuteRun(ctx context.Context, run executeRunContext) {
	switch run.decision.Harness {
	case "agent", "":
		s.runNative(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session)
	case "claude":
		s.runSubprocess(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session, &claudeharness.Runner{})
	case "codex":
		s.runSubprocess(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session, &codexharness.Runner{})
	case "gemini":
		s.runSubprocess(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session, &geminiharness.Runner{})
	case "opencode":
		s.runSubprocess(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session, &opencodeharness.Runner{})
	case "pi":
		s.runSubprocess(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session, &piharness.Runner{})
	case "virtual":
		s.runVirtual(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session)
	case "script":
		s.runScript(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session)
	default:
		if cfg, ok := s.registry.Get(run.decision.Harness); ok && cfg.IsHTTPProvider {
			s.runNative(ctx, run.req, run.decision, run.meta, run.out, run.seq, run.start, run.sl, run.session)
			return
		}
		finalizeAndEmit(run.out, run.seq, run.meta, run.req, run.sl, harnesses.FinalData{
			Status:     "failed",
			Error:      fmt.Sprintf("harness %q dispatch not yet wired in service.Execute", run.decision.Harness),
			DurationMS: time.Since(run.start).Milliseconds(),
			RoutingActual: &harnesses.RoutingActual{
				Harness:  run.decision.Harness,
				Provider: run.decision.Provider,
				Model:    run.decision.Model,
			},
		})
	}
}
