package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/ArthurWerle/reports/internal/model"
	"github.com/ArthurWerle/reports/internal/repository"
)

// EnqueueExecution creates a pending execution row and enqueues a generate-report
// job in the events service. Used by both the scheduler and manual runs. On
// enqueue failure the row is marked failed (so it is visible in the UI and does
// not block a retry) and the error is returned.
func EnqueueExecution(
	execRepo repository.ExecutionRepository,
	enq Enqueuer,
	callbackBaseURL string,
	reportID int64,
	year, month int,
	trigger string,
	logger *slog.Logger,
) (*model.ReportExecution, error) {
	exec := &model.ReportExecution{
		ReportID:    reportID,
		PeriodYear:  year,
		PeriodMonth: month,
		Trigger:     trigger,
		Status:      model.StatusPending,
	}
	if err := execRepo.Create(exec); err != nil {
		return nil, err
	}

	eventID, err := EnqueueGeneration(enq, exec.ID, callbackBaseURL)
	if err != nil {
		msg := "enqueue: " + err.Error()
		exec.Status = model.StatusFailed
		exec.ErrorMessage = &msg
		if uErr := execRepo.Update(exec); uErr != nil {
			logger.Error("scheduler: mark enqueue failure", "execution_id", exec.ID, "error", uErr)
		}
		return exec, err
	}

	exec.EventID = &eventID
	if err := execRepo.Update(exec); err != nil {
		logger.Error("scheduler: store event id", "execution_id", exec.ID, "error", err)
	}
	return exec, nil
}

// Scheduler enqueues scheduled report runs on their configured day/hour.
type Scheduler struct {
	reportRepo      repository.ReportRepository
	execRepo        repository.ExecutionRepository
	enq             Enqueuer
	callbackBaseURL string
	loc             *time.Location
	logger          *slog.Logger
	interval        time.Duration
	now             func() time.Time
}

func NewScheduler(
	reportRepo repository.ReportRepository,
	execRepo repository.ExecutionRepository,
	enq Enqueuer,
	callbackBaseURL string,
	loc *time.Location,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		reportRepo:      reportRepo,
		execRepo:        execRepo,
		enq:             enq,
		callbackBaseURL: callbackBaseURL,
		loc:             loc,
		logger:          logger,
		interval:        60 * time.Second,
		now:             time.Now,
	}
}

// Run ticks every interval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.logger.Info("scheduler started", "interval", s.interval.String())
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.Tick()
		}
	}
}

// Tick enqueues any reports due at the current minute. Exported for tests.
func (s *Scheduler) Tick() {
	now := s.now().In(s.loc)
	reports, err := s.reportRepo.ListEnabled()
	if err != nil {
		s.logger.Error("scheduler: list enabled reports", "error", err)
		return
	}

	for _, r := range reports {
		if now.Day() != r.DayOfMonth || now.Hour() != r.Hour {
			continue
		}
		year, month := PreviousMonth(now)

		exists, err := s.execRepo.HasActiveScheduled(r.ID, year, month)
		if err != nil {
			s.logger.Error("scheduler: dedupe check", "report_id", r.ID, "error", err)
			continue
		}
		if exists {
			continue
		}

		exec, err := EnqueueExecution(s.execRepo, s.enq, s.callbackBaseURL, r.ID, year, month, model.TriggerScheduled, s.logger)
		if err != nil {
			s.logger.Error("scheduler: enqueue", "report_id", r.ID, "error", err)
			continue
		}
		s.logger.Info("scheduler: enqueued report",
			"report_id", r.ID, "execution_id", exec.ID, "period", PeriodLabel(year, month))
	}
}
