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

func newTestGenerator(t *testing.T, txURL, aiURL string) (*Generator, *fakeExecRepo) {
	t.Helper()
	rr := newFakeReportRepo()
	rr.add(&model.Report{Name: "m", DayOfMonth: 1, Hour: 8, Recipients: "", Enabled: true})
	er := newFakeExecRepo()

	cfg := config.Config{}
	cfg.Report.Currency = "BRL"
	cfg.Report.Language = "en"
	// SMTP intentionally unconfigured → email is skipped.

	g := NewGenerator(er, rr, NewTransactionsClient(txURL), NewInsightsClient(aiURL), NewMailer(cfg.SMTP), cfg, time.UTC, testLogger())
	return g, er
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

	g, er := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	g.Generate(id)

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
	if got.EmailSentAt != nil {
		t.Errorf("email should be skipped (SMTP unconfigured)")
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

	g, er := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	g.Generate(id)

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

	g, er := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	g.Generate(id)

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

	g, er := newTestGenerator(t, tx.URL, ai.URL)
	id := seedRunningExec(er)
	g.Generate(id)

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
