package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/loadify/backend/internal/domain"
)

// Posting is what the posting worker reports about each job it takes.
type Posting struct {
	finished      *prometheus.CounterVec
	guardRefusals *prometheus.CounterVec
	sendFailures  *prometheus.CounterVec
	sendDuration  *prometheus.HistogramVec
	alerts        *prometheus.CounterVec
}

// NewPosting registers the worker's metrics.
func NewPosting(reg prometheus.Registerer) *Posting {
	p := &Posting{
		finished: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "posting",
			Name:      "jobs_finished_total",
			Help: "Jobs a worker took to an outcome, by the status it left them in. " +
				"pending and rate_limited mean the job will be tried again.",
		}, []string{"status"}),
		guardRefusals: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "posting",
			Name:      "guard_refusals_total",
			Help:      "Sends the anti-spam guard held back or refused, by reason.",
		}, []string{"reason"}),
		sendFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "telegram",
			Name:      "send_failures_total",
			Help:      "Sends Telegram did not accept, by kind of refusal (flood_wait, peer_flood, transient, ...).",
		}, []string{"kind"}),
		sendDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "telegram",
			Name:      "send_duration_seconds",
			Help:      "Time for one send, connecting included, by result (ok or error).",
			Buckets:   durationBuckets,
		}, []string{"result"}),
		alerts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "posting",
			Name:      "alerts_total",
			Help:      "Problems reported to people (restricted account, closed group, ...), by kind.",
		}, []string{"kind"}),
	}
	reg.MustRegister(p.finished, p.guardRefusals, p.sendFailures, p.sendDuration, p.alerts)
	return p
}

// JobFinished counts a job the worker took to an outcome.
func (p *Posting) JobFinished(status domain.PostingJobStatus) {
	p.finished.WithLabelValues(string(status)).Inc()
}

// GuardRefused counts a send the guard did not allow.
func (p *Posting) GuardRefused(reason domain.SkipReason) {
	p.guardRefusals.WithLabelValues(string(reason)).Inc()
}

// TelegramSend times a send; kind is empty when Telegram accepted it.
func (p *Posting) TelegramSend(took time.Duration, kind domain.TelegramErrorKind) {
	result := "ok"
	if kind != "" {
		result = "error"
		p.sendFailures.WithLabelValues(string(kind)).Inc()
	}
	p.sendDuration.WithLabelValues(result).Observe(took.Seconds())
}

// Alerted counts a problem reported to people.
func (p *Posting) Alerted(kind domain.DistributionAlertKind) {
	p.alerts.WithLabelValues(string(kind)).Inc()
}
