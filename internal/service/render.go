package service

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"time"

	"github.com/ArthurWerle/reports/internal/templates"
)

// ReportData is everything the generator gathers for one report period.
type ReportData struct {
	Year          int
	Month         int
	Overview      MonthOverview
	ByCategory    []NamedTotal
	BySubcategory []NamedTotal
	ByLocation    []NamedTotal
	History       []HistoryPoint
	Biggest       []BiggestTx
	// Charts maps a chart name to its PNG bytes (absent/empty = skipped).
	Charts map[string][]byte
}

type chartView struct {
	Title string
	// Src is a template.URL so html/template preserves cid: references (its
	// URL sanitizer would otherwise strip non-http schemes).
	Src     template.URL
	Present bool
}

type overviewCard struct {
	Label     string
	Amount    string
	HasDelta  bool
	DeltaText string
	Up        bool
}

// ReportView is the template model for report.html.
type ReportView struct {
	Period      string
	Headline    string
	Income      overviewCard
	Expense     overviewCard
	Charts      []chartView
	Highlights  []string
	Concerns    []string
	Closing     string
	HasInsights bool
	GeneratedAt string
}

func overviewToCard(label, currency string, side OverviewSide) overviewCard {
	card := overviewCard{
		Label:  label,
		Amount: FormatCurrency(currency, side.CurrentMonth),
	}
	if side.PercentageVariation != nil {
		pct := *side.PercentageVariation
		card.HasDelta = true
		card.Up = pct >= 0
		arrow := "▲"
		if pct < 0 {
			arrow = "▼"
		}
		card.DeltaText = fmt.Sprintf("%s %.1f%% vs last month", arrow, math.Abs(pct))
	}
	return card
}

func chartSrc(name string, execID int64, email bool) template.URL {
	if email {
		return template.URL("cid:" + name)
	}
	return template.URL(fmt.Sprintf("/api/v1/executions/%d/charts/%s", execID, name))
}

// BuildReportHTML renders report.html. When email is true, chart <img> src uses
// cid: references (inline attachments); otherwise it uses HTTP chart URLs.
func BuildReportHTML(data ReportData, insights *Insights, execID int64, currency string, email bool, generatedAt time.Time) (string, error) {
	view := ReportView{
		Period:      PeriodLabel(data.Year, data.Month),
		GeneratedAt: generatedAt.Format("2006-01-02 15:04 MST"),
	}
	if insights != nil {
		view.Headline = insights.Headline
		view.Highlights = insights.Highlights
		view.Concerns = insights.Concerns
		view.Closing = insights.Closing
		view.HasInsights = true
	} else {
		view.Headline = "Your report for " + view.Period
	}

	view.Income = overviewToCard("Income", currency, data.Overview.Income)
	view.Expense = overviewToCard("Expenses", currency, data.Overview.Expense)

	chartMeta := []struct{ name, title string }{
		{ChartCategory, "Expenses by category"},
		{ChartSubcategory, "Expenses by subcategory"},
		{ChartLocation, "Expenses by location"},
		{ChartHistory, "Income vs expense (last months)"},
	}
	for _, cm := range chartMeta {
		if len(data.Charts[cm.name]) == 0 {
			continue
		}
		view.Charts = append(view.Charts, chartView{
			Title:   cm.title,
			Src:     chartSrc(cm.name, execID, email),
			Present: true,
		})
	}

	var buf bytes.Buffer
	if err := templates.Report.Execute(&buf, view); err != nil {
		return "", err
	}
	return buf.String(), nil
}
