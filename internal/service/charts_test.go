package service

import (
	"bytes"
	"testing"
)

var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47}

func TestRenderBreakdownChartProducesPNG(t *testing.T) {
	items := []NamedTotal{
		{Name: "Groceries", Total: 300},
		{Name: "Restaurants", Total: 200},
		{Name: "(none)", Total: 50},
	}
	png, err := RenderBreakdownChart("Expenses by category", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(png) == 0 {
		t.Fatal("expected non-empty PNG bytes")
	}
	if !bytes.HasPrefix(png, pngMagic) {
		t.Errorf("output is not a PNG (magic = % x)", png[:4])
	}
}

func TestRenderBreakdownChartEmpty(t *testing.T) {
	png, err := RenderBreakdownChart("Empty", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if png != nil {
		t.Errorf("expected nil for empty data, got %d bytes", len(png))
	}

	// All-zero totals also produce no chart.
	png, _ = RenderBreakdownChart("Zeros", []NamedTotal{{Name: "a", Total: 0}})
	if png != nil {
		t.Errorf("expected nil for all-zero data")
	}
}

func TestTopNWithOtherRollup(t *testing.T) {
	items := make([]NamedTotal, 0, 10)
	for i := 0; i < 10; i++ {
		items = append(items, NamedTotal{Name: "c", Total: float64(10 - i)}) // 10,9,...,1
	}
	got := topNWithOther(items, topBuckets)
	if len(got) != topBuckets+1 {
		t.Fatalf("expected %d buckets (top + Other), got %d", topBuckets+1, len(got))
	}
	last := got[len(got)-1]
	if last.Name != "Other" {
		t.Errorf("expected last bucket 'Other', got %q", last.Name)
	}
	// The values (10-topBuckets)..1 roll into Other; derive the expected sum
	// from topBuckets so this stays correct if the constant changes.
	k := 10 - topBuckets
	want := float64(k * (k + 1) / 2)
	if last.Total != want {
		t.Errorf("Other total = %v, want %v", last.Total, want)
	}
}

func TestRenderHistoryChart(t *testing.T) {
	points := []HistoryPoint{
		{Month: "Jan 26", Income: 100, Expense: 80},
		{Month: "Feb 26", Income: 120, Expense: 90},
	}
	png, err := RenderHistoryChart(points)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.HasPrefix(png, pngMagic) {
		t.Errorf("history chart is not a PNG")
	}

	if png, _ := RenderHistoryChart(nil); png != nil {
		t.Errorf("expected nil history chart for no points")
	}
}
