package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// AI counts model calls and what they cost. The ai_tasks table holds the same
// facts per company; these are the platform-wide totals, cheap to alert on.
type AI struct {
	calls  *prometheus.CounterVec
	tokens *prometheus.CounterVec
	cost   prometheus.Counter
}

// NewAI registers the model metrics.
func NewAI(reg prometheus.Registerer) *AI {
	a := &AI{
		calls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "ai",
			Name:      "calls_total",
			Help: "Model calls, by outcome: ok, cache_hit, failed, or rejected_<reason> " +
				"(rejected_quota, rejected_budget). Only ok and failed reached the model.",
		}, []string{"outcome"}),
		tokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "ai",
			Name:      "tokens_total",
			Help:      "Tokens billed by the model, by direction (in or out).",
		}, []string{"direction"}),
		cost: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "ai",
			Name:      "cost_usd_total",
			Help:      "What model calls cost, at the configured per-token prices, in US dollars.",
		}),
	}
	reg.MustRegister(a.calls, a.tokens, a.cost)
	return a
}

// ModelCall records one call as the meter wrote it down.
func (a *AI) ModelCall(outcome string, tokensIn, tokensOut int, costMicros int64) {
	a.calls.WithLabelValues(outcome).Inc()
	if tokensIn > 0 {
		a.tokens.WithLabelValues("in").Add(float64(tokensIn))
	}
	if tokensOut > 0 {
		a.tokens.WithLabelValues("out").Add(float64(tokensOut))
	}
	if costMicros > 0 {
		a.cost.Add(float64(costMicros) / 1e6)
	}
}
