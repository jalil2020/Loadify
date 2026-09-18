package metrics

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

func TestPostingCountsByClosedLabels(t *testing.T) {
	p := NewPosting(prometheus.NewRegistry())

	p.JobFinished(domain.PostingSent)
	p.JobFinished(domain.PostingSent)
	p.JobFinished(domain.PostingRateLimited)
	p.GuardRefused(domain.SkipCooldown)
	p.TelegramSend(200*time.Millisecond, "")
	p.TelegramSend(time.Second, domain.TelegramFloodWait)
	p.Alerted(domain.AlertAccountRestricted)

	checks := map[string]float64{
		"sent":         testutil.ToFloat64(p.finished.WithLabelValues("sent")),
		"rate_limited": testutil.ToFloat64(p.finished.WithLabelValues("rate_limited")),
		"cooldown":     testutil.ToFloat64(p.guardRefusals.WithLabelValues("cooldown")),
		"flood_wait":   testutil.ToFloat64(p.sendFailures.WithLabelValues("flood_wait")),
		"restricted":   testutil.ToFloat64(p.alerts.WithLabelValues(string(domain.AlertAccountRestricted))),
	}
	want := map[string]float64{"sent": 2, "rate_limited": 1, "cooldown": 1, "flood_wait": 1, "restricted": 1}
	for name, got := range checks {
		if got != want[name] {
			t.Errorf("%s = %v, want %v", name, got, want[name])
		}
	}
	// A successful send is timed but is not a failure of any kind.
	if n := testutil.CollectAndCount(p.sendFailures); n != 1 {
		t.Errorf("%d failure series, want only flood_wait", n)
	}
	if n := testutil.CollectAndCount(p.sendDuration); n != 2 {
		t.Errorf("%d duration series, want ok and error", n)
	}
}

func TestAIAddsUpTokensAndCost(t *testing.T) {
	a := NewAI(prometheus.NewRegistry())

	a.ModelCall("ok", 1200, 300, 4500)
	a.ModelCall("cache_hit", 0, 0, 0)
	a.ModelCall("rejected_quota", 0, 0, 0)

	if got := testutil.ToFloat64(a.cost); got != 0.0045 {
		t.Errorf("cost = %v USD, want 0.0045", got)
	}
	if got := testutil.ToFloat64(a.tokens.WithLabelValues("in")); got != 1200 {
		t.Errorf("tokens in = %v, want 1200", got)
	}
	if got := testutil.ToFloat64(a.calls.WithLabelValues("cache_hit")); got != 1 {
		t.Errorf("cache hits = %v, want 1", got)
	}
}

type backlogReader struct {
	backlog domain.PostingBacklog
	err     error
}

func (r backlogReader) Backlog(context.Context) (domain.PostingBacklog, error) {
	return r.backlog, r.err
}

func TestBacklogReportsHowLateTheQueueIs(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	due := now.Add(-90 * time.Second)
	later := now.Add(time.Hour)

	cases := []struct {
		name   string
		reader backlogReader
		want   string
	}{
		{
			name: "a job that should have gone 90 seconds ago",
			reader: backlogReader{backlog: domain.PostingBacklog{
				Jobs:              map[domain.PostingJobStatus]int64{domain.PostingPending: 3},
				OldestNextAttempt: &due,
			}},
			want: `
# HELP loadify_posting_backlog_up 1 when the backlog could be read at this scrape, 0 when the database did not answer.
# TYPE loadify_posting_backlog_up gauge
loadify_posting_backlog_up 1
# HELP loadify_posting_jobs Jobs waiting or in flight, by status.
# TYPE loadify_posting_jobs gauge
loadify_posting_jobs{status="pending"} 3
loadify_posting_jobs{status="processing"} 0
loadify_posting_jobs{status="rate_limited"} 0
# HELP loadify_posting_oldest_overdue_seconds How long the oldest job whose time has come has been waiting to be picked up; 0 when none.
# TYPE loadify_posting_oldest_overdue_seconds gauge
loadify_posting_oldest_overdue_seconds 90
`,
		},
		{
			// Waiting out a cooldown is the guard working, not the queue being stuck.
			name: "everything waiting on purpose",
			reader: backlogReader{backlog: domain.PostingBacklog{
				Jobs:              map[domain.PostingJobStatus]int64{domain.PostingRateLimited: 5},
				OldestNextAttempt: &later,
			}},
			want: `
# HELP loadify_posting_backlog_up 1 when the backlog could be read at this scrape, 0 when the database did not answer.
# TYPE loadify_posting_backlog_up gauge
loadify_posting_backlog_up 1
# HELP loadify_posting_jobs Jobs waiting or in flight, by status.
# TYPE loadify_posting_jobs gauge
loadify_posting_jobs{status="pending"} 0
loadify_posting_jobs{status="processing"} 0
loadify_posting_jobs{status="rate_limited"} 5
# HELP loadify_posting_oldest_overdue_seconds How long the oldest job whose time has come has been waiting to be picked up; 0 when none.
# TYPE loadify_posting_oldest_overdue_seconds gauge
loadify_posting_oldest_overdue_seconds 0
`,
		},
		{
			name:   "the database does not answer",
			reader: backlogReader{err: errors.New("connection refused")},
			want: `
# HELP loadify_posting_backlog_up 1 when the backlog could be read at this scrape, 0 when the database did not answer.
# TYPE loadify_posting_backlog_up gauge
loadify_posting_backlog_up 0
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBacklog(tc.reader, time.Second, logger.New("test", "error"))
			b.now = func() time.Time { return now }
			if err := testutil.CollectAndCompare(b, strings.NewReader(tc.want)); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestServeAnswersUntilCancelled(t *testing.T) {
	addr := freeAddr(t)
	reg := NewRegistry("test", "v1.2.3")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := Serve(ctx, addr, reg, logger.New("test", "error")); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	res, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	for _, want := range []string{`loadify_build_info{service="test",version="v1.2.3"} 1`, "go_goroutines"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("scrape does not contain %q", want)
		}
	}

	// A second process on the same address is told at startup.
	if err := Serve(ctx, addr, reg, logger.New("test", "error")); err == nil {
		t.Error("serving on an address already in use was not reported")
	}

	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := http.Get("http://" + addr + "/metrics"); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("still serving after the context ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestServeCanBeTurnedOff(t *testing.T) {
	if err := Serve(context.Background(), "", NewRegistry("test", "v"), logger.New("test", "error")); err != nil {
		t.Fatalf("Serve with no address: %v", err)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no free port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}
