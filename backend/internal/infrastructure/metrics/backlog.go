package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// backlogStatuses are the ones reported even when nothing is in them, so a
// graph shows a zero rather than a gap.
var backlogStatuses = []domain.PostingJobStatus{
	domain.PostingPending, domain.PostingRateLimited, domain.PostingProcessing,
}

// Backlog reads the posting queue from the database at scrape time.
//
// It answers the question the counters cannot: is anything stuck? Jobs that
// wait on purpose (a cooldown, a FLOOD_WAIT) are not late; a job whose time
// came and went while nobody picked it up is, and the age of the oldest such
// job is what an alert should watch.
type Backlog struct {
	reader  domain.PostingBacklogReader
	timeout time.Duration
	now     func() time.Time
	log     logger.Logger

	jobs    *prometheus.Desc
	overdue *prometheus.Desc
	up      *prometheus.Desc
}

// NewBacklog builds the collector; register it on the worker's registry.
func NewBacklog(reader domain.PostingBacklogReader, timeout time.Duration, log logger.Logger) *Backlog {
	return &Backlog{
		reader: reader, timeout: timeout, now: time.Now, log: log,
		jobs: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "posting", "jobs"),
			"Jobs waiting or in flight, by status.",
			[]string{"status"}, nil),
		overdue: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "posting", "oldest_overdue_seconds"),
			"How long the oldest job whose time has come has been waiting to be picked up; 0 when none.",
			nil, nil),
		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "posting", "backlog_up"),
			"1 when the backlog could be read at this scrape, 0 when the database did not answer.",
			nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (b *Backlog) Describe(ch chan<- *prometheus.Desc) {
	ch <- b.jobs
	ch <- b.overdue
	ch <- b.up
}

// Collect implements prometheus.Collector. A database that does not answer is
// reported as backlog_up 0, not as a failed scrape: the process metrics are
// still worth having, and "the worker cannot see its queue" is itself the news.
func (b *Backlog) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	backlog, err := b.reader.Backlog(ctx)
	if err != nil {
		b.log.Warn("posting backlog not read for metrics", logger.Err(err))
		ch <- prometheus.MustNewConstMetric(b.up, prometheus.GaugeValue, 0)
		return
	}
	ch <- prometheus.MustNewConstMetric(b.up, prometheus.GaugeValue, 1)

	for _, status := range backlogStatuses {
		ch <- prometheus.MustNewConstMetric(b.jobs, prometheus.GaugeValue,
			float64(backlog.Jobs[status]), string(status))
	}

	overdue := 0.0
	if backlog.OldestNextAttempt != nil {
		if late := b.now().Sub(*backlog.OldestNextAttempt); late > 0 {
			overdue = late.Seconds()
		}
	}
	ch <- prometheus.MustNewConstMetric(b.overdue, prometheus.GaugeValue, overdue)
}
