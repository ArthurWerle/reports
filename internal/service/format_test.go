package service

import (
	"testing"
	"time"
)

func TestPreviousMonth(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		wantYear  int
		wantMonth int
	}{
		{"mid-year", time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC), 2026, 6},
		{"january rolls to december prior year", time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC), 2025, 12},
		{"march 31 does not normalize", time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC), 2026, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y, m := PreviousMonth(tt.now)
			if y != tt.wantYear || m != tt.wantMonth {
				t.Errorf("PreviousMonth(%v) = %d-%d, want %d-%d", tt.now, y, m, tt.wantYear, tt.wantMonth)
			}
		})
	}
}

func TestHistoryWindow(t *testing.T) {
	start, end := HistoryWindow(2026, 6, time.UTC)
	if start != "2026-01-01" {
		t.Errorf("start = %s, want 2026-01-01", start)
	}
	if end != "2026-06-30" {
		t.Errorf("end = %s, want 2026-06-30", end)
	}

	// January period: window reaches back into the previous year.
	start, end = HistoryWindow(2026, 1, time.UTC)
	if start != "2025-08-01" {
		t.Errorf("start = %s, want 2025-08-01", start)
	}
	if end != "2026-01-31" {
		t.Errorf("end = %s, want 2026-01-31", end)
	}
}

func TestFormatCurrency(t *testing.T) {
	tests := []struct {
		currency string
		amount   float64
		want     string
	}{
		{"BRL", 1234.56, "R$ 1.234,56"},
		{"BRL", 0, "R$ 0,00"},
		{"BRL", 1000000, "R$ 1.000.000,00"},
		{"USD", 1234.5, "$1,234.50"},
		{"EUR", 12.3, "€12,30"},
		{"GBP", 5, "GBP 5.00"},
	}
	for _, tt := range tests {
		got := FormatCurrency(tt.currency, tt.amount)
		if got != tt.want {
			t.Errorf("FormatCurrency(%s, %v) = %q, want %q", tt.currency, tt.amount, got, tt.want)
		}
	}
}

func TestParseRecipients(t *testing.T) {
	got := ParseRecipients(" a@x.com ,, b@y.com,  ")
	if len(got) != 2 || got[0] != "a@x.com" || got[1] != "b@y.com" {
		t.Errorf("ParseRecipients returned %#v", got)
	}
	if len(ParseRecipients("")) != 0 {
		t.Errorf("empty list should yield no recipients")
	}
}

func TestPeriodLabel(t *testing.T) {
	if got := PeriodLabel(2026, 6); got != "June 2026" {
		t.Errorf("PeriodLabel = %q, want June 2026", got)
	}
}
