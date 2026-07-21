package service

import (
	"bytes"
	"sort"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
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
	chartHeight = 460
	topBuckets  = 6
	labelMaxLen = 13
	// go-chart's BarChart reserves roughly 2×Padding.Bottom for the x-axis
	// labels below the bars (a quirk of how it constrains the canvas box), so
	// this needs to be generous enough that category names — including ones
	// that wrap onto a second line — are never clipped.
	barLabelPadding = 52
)

// Minimal, Vercel-style palette: solid zinc bars on a white canvas with hairline
// axes, and refined green/red series for the history chart.
var (
	colorCanvas = drawing.ColorFromHex("ffffff")
	colorBar    = drawing.ColorFromHex("3f3f46") // zinc-700
	colorAxis   = drawing.ColorFromHex("71717a") // zinc-500 (labels)
	colorGrid   = drawing.ColorFromHex("e4e4e7") // zinc-200 (hairlines)
	colorGreen  = drawing.ColorFromHex("16a34a")
	colorRed    = drawing.ColorFromHex("dc2626")
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

	barStyle := chart.Style{FillColor: colorBar, StrokeColor: colorBar, StrokeWidth: 0}
	bars := make([]chart.Value, 0, len(buckets))
	maxVal := 0.0
	for _, b := range buckets {
		bars = append(bars, chart.Value{
			Value: b.Total,
			Label: truncateLabel(b.Name, labelMaxLen),
			Style: barStyle,
		})
		if b.Total > maxVal {
			maxVal = b.Total
		}
	}

	axisStyle := chart.Style{FontColor: colorAxis, FontSize: 11, StrokeColor: colorGrid}

	graph := chart.BarChart{
		Background: chart.Style{
			FillColor: colorCanvas,
			Padding:   chart.Box{Top: 24, Left: 16, Right: 16, Bottom: barLabelPadding},
		},
		Canvas:     chart.Style{FillColor: colorCanvas},
		Width:      chartWidth,
		Height:     chartHeight,
		BarWidth:   int(float64(chartWidth-120) / float64(len(bars)+1)),
		BarSpacing: 22,
		Bars:       bars,
		XAxis:      axisStyle,
		// An explicit range anchored at 0 avoids go-chart's "invalid data
		// range; cannot be zero" error when there is a single bar.
		YAxis: chart.YAxis{
			Style: axisStyle,
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

	axisStyle := chart.Style{FontColor: colorAxis, FontSize: 11, StrokeColor: colorGrid}

	graph := chart.Chart{
		Background: chart.Style{
			FillColor: colorCanvas,
			Padding:   chart.Box{Top: 24, Left: 16, Right: 16, Bottom: 12},
		},
		Canvas: chart.Style{FillColor: colorCanvas},
		Width:  chartWidth,
		Height: chartHeight,
		XAxis:  chart.XAxis{Style: axisStyle, Ticks: ticks},
		YAxis:  chart.YAxis{Style: axisStyle},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Name:    "Income",
				XValues: xs,
				YValues: incomeYs,
				Style:   chart.Style{StrokeColor: colorGreen, StrokeWidth: 2.5},
			},
			chart.ContinuousSeries{
				Name:    "Expense",
				XValues: xs,
				YValues: expenseYs,
				Style:   chart.Style{StrokeColor: colorRed, StrokeWidth: 2.5},
			},
		},
	}
	graph.Elements = []chart.Renderable{
		chart.Legend(&graph, chart.Style{
			FontColor:   colorAxis,
			FontSize:    11,
			FillColor:   colorCanvas,
			StrokeColor: colorGrid,
		}),
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
