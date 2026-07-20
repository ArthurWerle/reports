package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArthurWerle/reports/internal/config"
	"github.com/ArthurWerle/reports/internal/model"
)

func newTxServer(empty, failOverview bool) *httptest.Server {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
	mux.HandleFunc("/transactions/reports/month-overview", func(w http.ResponseWriter, r *http.Request) {
		if failOverview {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if empty {
			write(w, `{"income":{"currentMonth":0,"lastMonth":0,"percentageVariation":null},"expense":{"currentMonth":0,"lastMonth":0,"percentageVariation":null}}`)
			return
		}
		write(w, `{"income":{"currentMonth":1000,"lastMonth":900,"percentageVariation":11.1},"expense":{"currentMonth":700,"lastMonth":800,"percentageVariation":-12.5}}`)
	})
	mux.HandleFunc("/transactions/reports/monthly-expenses-by-category", func(w http.ResponseWriter, r *http.Request) {
		if empty {
			write(w, `{"categories":[]}`)
			return
		}
		write(w, `{"categories":[{"category_name":"Rent","total":400},{"category_name":"Food","total":300}]}`)
	})
	mux.HandleFunc("/transactions/reports/monthly-expenses-by-subcategory", func(w http.ResponseWriter, r *http.Request) {
		if empty {
			write(w, `{"subcategories":[]}`)
			return
		}
		write(w, `{"subcategories":[{"subcategory_name":"Groceries","total":200},{"subcategory_name":"(none)","total":100}]}`)
	})
	mux.HandleFunc("/transactions/reports/monthly-expenses-by-location", func(w http.ResponseWriter, r *http.Request) {
		if empty {
			write(w, `{"locations":[]}`)
			return
		}
		write(w, `{"locations":[{"location_name":"Market","total":250}]}`)
	})
	mux.HandleFunc("/transactions/reports/monthly-history", func(w http.ResponseWriter, r *http.Request) {
		if empty {
			write(w, `[]`)
			return
		}
		write(w, `[{"month":"May 26","income":900,"expense":800},{"month":"Jun 26","income":1000,"expense":700}]`)
	})
	mux.HandleFunc("/transactions/biggest", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"transactions":[{"description":"TV","amount":500}]}`)
	})
	return httptest.NewServer(mux)
}

func newAIServer(fail bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "llm down", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"headline":"In June, expenses dropped 12.5%.","highlights":["Spending fell"],"concerns":["Rent is high"],"closing":"Keep it up."}}`))
	}))
}

// fakeMailer records sends and always succeeds.
type fakeMailer struct{ sent int }

func (f *fakeMailer) Send(subject, htmlBody string, recipients []string, charts map[string][]byte) error {
	f.sent++
	return nil
}

func newTestGenerator(t *testing.T, txURL, aiURL string) (*Generator, *fakeExecRepo, *fakeMailer) {
	t.Helper()
	rr := newFakeReportRepo()
	rr.add(&model.Report{Name: "m", DayOfMonth: 1, Hour: 8, Recipients: "a@b.com", Enabled: true})
	er := newFakeExecRepo()

	cfg := config.Config{}
	cfg.Report.Currency = "BRL"
	cfg.Report.Language = "en"
	cfg.SMTP.Host = "smtp.test"
	cfg.SMTP.Port = "587"
	cfg.SMTP.From = "reports@test"

	mailer := &fakeMailer{}
	g := NewGenerator(er, rr, NewTransactionsClient(txURL), NewInsightsClient(aiURL), mailer, cfg, time.UTC, testLogger())
	return g, er, mailer
}

func seedRunningExec(er *fakeExecRepo) int64 {
	started := time.Now()
	exec := &model.ReportExecution{
		ReportID: 1, PeriodYear: 2026, PeriodMonth: 6,
		Trigger: model.TriggerManual, Status: model.StatusRunning, StartedAt: &started,
	}
	_ = er.Create(exec)
	return exec.ID
}

func TestGeneratorSuccess(t *testing.T) {
	tx := newTxServer(false, false)
	defer tx.Close()
	ai := newAIServer(false)
	defer ai.Close()

	g, er, mailer := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	if err := g.Generate(id); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	got, _ := er.Get(id)
	if got.Status != model.StatusSuccess {
		t.Fatalf("status = %q, want success (err=%v)", got.Status, got.ErrorMessage)
	}
	if got.HTML == nil || *got.HTML == "" {
		t.Errorf("expected web HTML stored")
	}
	if got.Insights == nil {
		t.Errorf("expected insights stored")
	}
	if got.ErrorMessage != nil {
		t.Errorf("expected no error message, got %q", *got.ErrorMessage)
	}
	if mailer.sent != 1 {
		t.Errorf("expected 1 email sent, got %d", mailer.sent)
	}
	if got.EmailSentAt == nil {
		t.Errorf("expected email_sent_at set")
	}
	if got.DurationMs == nil {
		t.Errorf("expected duration recorded")
	}
	if n := er.chartCount(id); n != 4 {
		t.Errorf("expected 4 charts, got %d", n)
	}
}

func TestGeneratorTransactionsDown(t *testing.T) {
	tx := newTxServer(false, true) // month-overview 500
	defer tx.Close()
	ai := newAIServer(false)
	defer ai.Close()

	g, er, _ := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	if err := g.Generate(id); err == nil {
		t.Fatalf("Generate should return error when transactions are down")
	}

	got, _ := er.Get(id)
	if got.Status != model.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorMessage == nil || !strings.HasPrefix(*got.ErrorMessage, "fetch transactions data:") {
		t.Errorf("error message = %v, want step-prefixed fetch error", got.ErrorMessage)
	}
}

func TestGeneratorInsightsDownDegrades(t *testing.T) {
	tx := newTxServer(false, false)
	defer tx.Close()
	ai := newAIServer(true) // llm 500
	defer ai.Close()

	g, er, _ := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	if err := g.Generate(id); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	got, _ := er.Get(id)
	if got.Status != model.StatusSuccess {
		t.Fatalf("status = %q, want success (degraded)", got.Status)
	}
	if got.Insights != nil {
		t.Errorf("expected no insights when LLM is down")
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, "insights unavailable") {
		t.Errorf("expected degradation note, got %v", got.ErrorMessage)
	}
	if n := er.chartCount(id); n != 4 {
		t.Errorf("charts should still render, got %d", n)
	}
}

func TestGeneratorEmptyMonthSkipsCharts(t *testing.T) {
	tx := newTxServer(true, false) // all empty
	defer tx.Close()
	ai := newAIServer(false)
	defer ai.Close()

	g, er, _ := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	if err := g.Generate(id); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	got, _ := er.Get(id)
	if got.Status != model.StatusSuccess {
		t.Fatalf("status = %q, want success", got.Status)
	}
	if n := er.chartCount(id); n != 0 {
		t.Errorf("expected 0 charts for empty month, got %d", n)
	}
	if got.HTML == nil {
		t.Errorf("expected HTML even with no charts")
	}
}

func TestGeneratorEmailSkippedIsFailure(t *testing.T) {
	tx := newTxServer(false, false)
	defer tx.Close()
	ai := newAIServer(false)
	defer ai.Close()

	g, er, mailer := newTestGenerator(t, tx.URL, ai.URL)
	g.cfg.SMTP = config.SMTPConfig{} // unconfigured → email cannot be sent
	id := seedRunningExec(er)
	if err := g.Generate(id); err == nil {
		t.Fatalf("Generate should return error when email cannot be sent")
	}

	got, _ := er.Get(id)
	if got.Status != model.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, "email not sent") {
		t.Errorf("error message = %v, want email-not-sent", got.ErrorMessage)
	}
	if mailer.sent != 0 {
		t.Errorf("no email should be sent, got %d", mailer.sent)
	}
	if got.EmailSentAt != nil {
		t.Errorf("email_sent_at must stay nil")
	}
	if got.HTML == nil {
		t.Errorf("report HTML should still be stored for the web UI")
	}
}

func TestGeneratorNoRecipientsIsFailure(t *testing.T) {
	tx := newTxServer(false, false)
	defer tx.Close()
	ai := newAIServer(false)
	defer ai.Close()

	g, er, _ := newTestGenerator(t, tx.URL, ai.URL)
	rr := newFakeReportRepo()
	rr.add(&model.Report{Name: "m", DayOfMonth: 1, Hour: 8, Recipients: "", Enabled: true})
	g.reportRepo = rr
	id := seedRunningExec(er)
	if err := g.Generate(id); err == nil {
		t.Fatalf("Generate should return error when report has no recipients")
	}

	got, _ := er.Get(id)
	if got.Status != model.StatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, "no recipients") {
		t.Errorf("error message = %v, want no-recipients", got.ErrorMessage)
	}
}
