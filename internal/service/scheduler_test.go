package service

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ArthurWerle/reports/internal/model"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSchedulerEnqueuesWhenDueAndDedupes(t *testing.T) {
	rr := newFakeReportRepo()
	er := newFakeExecRepo()
	enq := &fakeEnqueuer{}
	rr.add(&model.Report{Name: "Monthly report", DayOfMonth: 1, Hour: 8, Enabled: true})

	s := NewScheduler(rr, er, enq, "http://reports:8080", time.UTC, testLogger())
	s.now = func() time.Time { return time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC) }

	s.Tick()
	if enq.count() != 1 {
		t.Fatalf("expected 1 enqueue, got %d", enq.count())
	}
	execs, _ := er.List(nil, 0)
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
	e := execs[0]
	if e.PeriodYear != 2026 || e.PeriodMonth != 6 {
		t.Errorf("period = %d-%d, want 2026-6 (previous month)", e.PeriodYear, e.PeriodMonth)
	}
	if e.Trigger != model.TriggerScheduled {
		t.Errorf("trigger = %q, want scheduled", e.Trigger)
	}
	if e.EventID == nil {
		t.Errorf("expected event id stored on execution")
	}

	// Repeated ticks in the same hour must not enqueue again (dedupe).
	s.Tick()
	s.Tick()
	if enq.count() != 1 {
		t.Errorf("dedupe failed: %d enqueues after repeated ticks", enq.count())
	}
}

func TestSchedulerNotDue(t *testing.T) {
	rr := newFakeReportRepo()
	er := newFakeExecRepo()
	enq := &fakeEnqueuer{}
	rr.add(&model.Report{Name: "m", DayOfMonth: 1, Hour: 8, Enabled: true})
	s := NewScheduler(rr, er, enq, "http://reports:8080", time.UTC, testLogger())

	// Wrong hour.
	s.now = func() time.Time { return time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC) }
	s.Tick()
	// Wrong day.
	s.now = func() time.Time { return time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC) }
	s.Tick()

	if enq.count() != 0 {
		t.Errorf("expected no enqueues when not due, got %d", enq.count())
	}
}

func TestSchedulerSkipsDisabledReports(t *testing.T) {
	rr := newFakeReportRepo()
	er := newFakeExecRepo()
	enq := &fakeEnqueuer{}
	rr.add(&model.Report{Name: "disabled", DayOfMonth: 1, Hour: 8, Enabled: false})
	s := NewScheduler(rr, er, enq, "http://reports:8080", time.UTC, testLogger())
	s.now = func() time.Time { return time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC) }
	s.Tick()
	if enq.count() != 0 {
		t.Errorf("disabled report should not enqueue, got %d", enq.count())
	}
}

func TestSchedulerReenqueuesAfterFailedExecution(t *testing.T) {
	rr := newFakeReportRepo()
	er := newFakeExecRepo()
	rr.add(&model.Report{Name: "m", DayOfMonth: 1, Hour: 8, Enabled: true})

	// A prior FAILED scheduled execution for the same period must not block a retry.
	_ = er.Create(&model.ReportExecution{ReportID: 1, PeriodYear: 2026, PeriodMonth: 6, Trigger: model.TriggerScheduled, Status: model.StatusFailed})

	enq := &fakeEnqueuer{}
	s := NewScheduler(rr, er, enq, "http://reports:8080", time.UTC, testLogger())
	s.now = func() time.Time { return time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC) }
	s.Tick()
	if enq.count() != 1 {
		t.Errorf("failed prior execution should allow re-enqueue, got %d", enq.count())
	}
}
