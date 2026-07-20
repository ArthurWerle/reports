package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ArthurWerle/reports/internal/config"
	"github.com/ArthurWerle/reports/internal/model"
	"github.com/ArthurWerle/reports/internal/repository"
)

// EmailSender delivers the report email. *Mailer is the real implementation;
// a fake is injected in tests.
type EmailSender interface {
	Send(subject, htmlBody string, recipients []string, charts map[string][]byte) error
}

// Generator runs the report pipeline for a single execution.
type Generator struct {
	execRepo   repository.ExecutionRepository
	reportRepo repository.ReportRepository
	tx         *TransactionsClient
	insights   *InsightsClient
	mailer     EmailSender
	cfg        config.Config
	loc        *time.Location
	logger     *slog.Logger
	now        func() time.Time
}

func NewGenerator(
	execRepo repository.ExecutionRepository,
	reportRepo repository.ReportRepository,
	tx *TransactionsClient,
	insights *InsightsClient,
	mailer EmailSender,
	cfg config.Config,
	loc *time.Location,
	logger *slog.Logger,
) *Generator {
	return &Generator{
		execRepo:   execRepo,
		reportRepo: reportRepo,
		tx:         tx,
		insights:   insights,
		mailer:     mailer,
		cfg:        cfg,
		loc:        loc,
		logger:     logger,
		now:        time.Now,
	}
}

// Generate runs the full pipeline for the execution id. All outcomes (success
// and failure) are persisted on the execution row. The returned error is nil
// only when the report was generated AND the email was actually sent, so the
// caller can propagate the real outcome to the events service. Panics are
// recovered into a failed status.
func (g *Generator) Generate(execID int64) (retErr error) {
	exec, err := g.execRepo.Get(execID)
	if err != nil {
		g.logger.Error("generator: load execution", "execution_id", execID, "error", err)
		return fmt.Errorf("load execution %d: %w", execID, err)
	}

	started := g.now()
	if exec.StartedAt != nil {
		started = *exec.StartedAt
	}

	defer func() {
		if r := recover(); r != nil {
			g.logger.Error("generator: panic", "execution_id", execID, "panic", r)
			msg := fmt.Sprintf("panic: %v", r)
			g.finalize(exec, started, model.StatusFailed, msg)
			retErr = fmt.Errorf("%s", msg)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	year, month := exec.PeriodYear, exec.PeriodMonth

	// 1. Fetch aggregated numbers.
	data, err := g.fetch(ctx, year, month)
	if err != nil {
		return g.fail(exec, started, "fetch transactions data: "+err.Error())
	}

	// 2. Insights (degrade gracefully on failure).
	var degradedMsg string
	ins, err := g.insights.Generate(ctx, g.insightsRequest(data))
	if err != nil {
		g.logger.Warn("generator: insights degraded", "execution_id", execID, "error", err)
		ins = nil
		degradedMsg = "insights unavailable: " + err.Error()
	}

	// 3. Charts (non-fatal per chart).
	data.Charts = g.renderCharts(data)

	// 4. HTML — web mode (stored) uses HTTP chart URLs.
	webHTML, err := BuildReportHTML(data, ins, exec.ID, g.cfg.Report.Currency, false, g.now())
	if err != nil {
		return g.fail(exec, started, "render web html: "+err.Error())
	}
	exec.HTML = &webHTML
	if ins != nil {
		if b, mErr := json.Marshal(ins); mErr == nil {
			s := string(b)
			exec.Insights = &s
		}
	}

	// Persist charts before email so the report stays viewable even if email fails.
	for name, png := range data.Charts {
		if len(png) == 0 {
			continue
		}
		if err := g.execRepo.SaveChart(&model.ReportChart{ExecutionID: exec.ID, Name: name, Image: png}); err != nil {
			return g.fail(exec, started, "persist chart "+name+": "+err.Error())
		}
	}

	// 5. Email. Sending is mandatory for success: an execution (and the events
	// job behind it) is only "done" when the email actually went out.
	recipients := g.recipientsFor(exec.ReportID)
	if !g.cfg.SMTP.Configured() {
		return g.fail(exec, started, "email not sent: SMTP not configured")
	}
	if len(recipients) == 0 {
		return g.fail(exec, started, "email not sent: report has no recipients")
	}
	emailHTML, err := BuildReportHTML(data, ins, exec.ID, g.cfg.Report.Currency, true, g.now())
	if err != nil {
		return g.fail(exec, started, "render email html: "+err.Error())
	}
	subject := "Monthly report — " + PeriodLabel(year, month)
	if err := g.mailer.Send(subject, emailHTML, recipients, data.Charts); err != nil {
		return g.fail(exec, started, "send email: "+err.Error())
	}
	sentAt := g.now()
	exec.EmailSentAt = &sentAt

	// 6. Finalize success (degradation, if any, recorded but not fatal).
	g.finalize(exec, started, model.StatusSuccess, degradedMsg)
	return nil
}

// fail finalizes the execution as failed and returns the message as an error.
func (g *Generator) fail(exec *model.ReportExecution, started time.Time, message string) error {
	g.finalize(exec, started, model.StatusFailed, message)
	return fmt.Errorf("%s", message)
}

func (g *Generator) fetch(ctx context.Context, year, month int) (ReportData, error) {
	data := ReportData{Year: year, Month: month, Charts: map[string][]byte{}}

	overview, err := g.tx.MonthOverview(ctx, month, year)
	if err != nil {
		return data, fmt.Errorf("month overview: %w", err)
	}
	data.Overview = overview

	if data.ByCategory, err = g.tx.ExpensesByCategory(ctx, month, year); err != nil {
		return data, fmt.Errorf("expenses by category: %w", err)
	}
	if data.BySubcategory, err = g.tx.ExpensesBySubcategory(ctx, month, year); err != nil {
		return data, fmt.Errorf("expenses by subcategory: %w", err)
	}
	if data.ByLocation, err = g.tx.ExpensesByLocation(ctx, month, year); err != nil {
		return data, fmt.Errorf("expenses by location: %w", err)
	}

	startDate, endDate := HistoryWindow(year, month, g.loc)
	if data.History, err = g.tx.MonthlyHistory(ctx, startDate, endDate); err != nil {
		return data, fmt.Errorf("monthly history: %w", err)
	}

	// Best-effort: biggest transactions only enrich insights.
	if biggest, bErr := g.tx.Biggest(ctx, month, year); bErr != nil {
		g.logger.Warn("generator: biggest transactions fetch failed", "error", bErr)
	} else {
		data.Biggest = biggest
	}

	return data, nil
}

func (g *Generator) insightsRequest(data ReportData) InsightsRequest {
	return InsightsRequest{
		Month:                 data.Month,
		Year:                  data.Year,
		Language:              g.cfg.Report.Language,
		Overview:              data.Overview,
		ExpensesByCategory:    data.ByCategory,
		ExpensesBySubcategory: data.BySubcategory,
		ExpensesByLocation:    data.ByLocation,
		MonthlyHistory:        data.History,
		BiggestTransactions:   data.Biggest,
	}
}

func (g *Generator) renderCharts(data ReportData) map[string][]byte {
	charts := map[string][]byte{}
	add := func(name, title string, items []NamedTotal) {
		png, err := RenderBreakdownChart(title, items)
		if err != nil {
			g.logger.Warn("generator: chart render failed", "chart", name, "error", err)
			return
		}
		if png != nil {
			charts[name] = png
		}
	}
	add(ChartCategory, "Expenses by category", data.ByCategory)
	add(ChartSubcategory, "Expenses by subcategory", data.BySubcategory)
	add(ChartLocation, "Expenses by location", data.ByLocation)

	if png, err := RenderHistoryChart(data.History); err != nil {
		g.logger.Warn("generator: chart render failed", "chart", ChartHistory, "error", err)
	} else if png != nil {
		charts[ChartHistory] = png
	}
	return charts
}

func (g *Generator) recipientsFor(reportID int64) []string {
	report, err := g.reportRepo.Get(reportID)
	if err != nil {
		g.logger.Warn("generator: load report for recipients", "report_id", reportID, "error", err)
		return nil
	}
	return ParseRecipients(report.Recipients)
}

// finalize records the terminal state, duration, and any message on the row.
func (g *Generator) finalize(exec *model.ReportExecution, started time.Time, status, message string) {
	finished := g.now()
	duration := finished.Sub(started).Milliseconds()
	exec.Status = status
	exec.FinishedAt = &finished
	exec.DurationMs = &duration
	if message != "" {
		exec.ErrorMessage = &message
	}
	if err := g.execRepo.Update(exec); err != nil {
		g.logger.Error("generator: persist final state", "execution_id", exec.ID, "error", err)
	}
}
