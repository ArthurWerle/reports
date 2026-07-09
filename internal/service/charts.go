package service

import (
	"bytes"
	"sort"

	chart "github.com/wcharczuk/go-chart/v2"
)

// Chart names (also used as report_charts.name and cid: references).
const (
	ChartCategory    = "expenses-by-category"
	ChartSubcategory = "expenses-by-subcategory"
	ChartLocation    = "expenses-by-location"
	ChartHistory     = "monthly-history"
)

const (
	chartWidth  = 900
	chartHeight = 420
	topBuckets  = 8
	labelMaxLen = 14
)

func truncateLabel(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

// topNWithOther keeps the largest n buckets and rolls the rest into "Other".
// Input is copied and sorted descending so the caller's slice is untouched.
func topNWithOther(items []NamedTotal, n int) []NamedTotal {
	cp := make([]NamedTotal, 0, len(items))
	for _, it := range items {
		if it.Total > 0 {
			cp = append(cp, it)
		}
	}
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].Total > cp[j].Total })
	if len(cp) <= n {
		return cp
	}
	var other float64
	for _, it := range cp[n:] {
		other += it.Total
	}
	result := make([]NamedTotal, 0, n+1)
	result = append(result, cp[:n]...)
	if other > 0 {
		result = append(result, NamedTotal{Name: "Other", Total: other})
	}
	return result
}

// RenderBreakdownChart renders a bar chart of expense buckets (top 8 + Other).
// Returns (nil, nil) when there is no positive data to plot.
func RenderBreakdownChart(title string, items []NamedTotal) ([]byte, error) {
	buckets := topNWithOther(items, topBuckets)
	if len(buckets) == 0 {
		return nil, nil
	}

	bars := make([]chart.Value, 0, len(buckets))
	maxVal := 0.0
	for _, b := range buckets {
		bars = append(bars, chart.Value{
			Value: b.Total,
			Label: truncateLabel(b.Name, labelMaxLen),
		})
		if b.Total > maxVal {
			maxVal = b.Total
		}
	}

	graph := chart.BarChart{
		Title:      title,
		TitleStyle: chart.Style{FontSize: 14},
		Background: chart.Style{Padding: chart.Box{Top: 45, Left: 20, Right: 20, Bottom: 20}},
		Width:      chartWidth,
		Height:     chartHeight,
		BarWidth:   int(float64(chartWidth-120) / float64(len(bars)+1)),
		Bars:       bars,
		// An explicit range anchored at 0 avoids go-chart's "invalid data
		// range; cannot be zero" error when there is a single bar.
		YAxis: chart.YAxis{
			Range: &chart.ContinuousRange{Min: 0, Max: maxVal * 1.15},
		},
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderHistoryChart renders a two-series line chart (income vs expense) with
// one point per month. Returns (nil, nil) when there is nothing to plot.
func RenderHistoryChart(points []HistoryPoint) ([]byte, error) {
	if len(points) == 0 {
		return nil, nil
	}

	xs := make([]float64, len(points))
	incomeYs := make([]float64, len(points))
	expenseYs := make([]float64, len(points))
	ticks := make([]chart.Tick, len(points))
	for i, p := range points {
		xs[i] = float64(i)
		incomeYs[i] = p.Income
		expenseYs[i] = p.Expense
		ticks[i] = chart.Tick{Value: float64(i), Label: p.Month}
	}

	graph := chart.Chart{
		Title:      "Income vs Expense",
		TitleStyle: chart.Style{FontSize: 14},
		Background: chart.Style{Padding: chart.Box{Top: 45, Left: 20, Right: 20, Bottom: 20}},
		Width:      chartWidth,
		Height:     chartHeight,
		XAxis:      chart.XAxis{Ticks: ticks},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Name:    "Income",
				XValues: xs,
				YValues: incomeYs,
				Style:   chart.Style{StrokeColor: chart.ColorGreen, StrokeWidth: 2.5},
			},
			chart.ContinuousSeries{
				Name:    "Expense",
				XValues: xs,
				YValues: expenseYs,
				Style:   chart.Style{StrokeColor: chart.ColorRed, StrokeWidth: 2.5},
			},
		},
	}
	graph.Elements = []chart.Renderable{chart.Legend(&graph)}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
