package service

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// PreviousMonth returns the (year, month) of the calendar month before now,
// computed from the first of now's month to avoid end-of-month normalization
// bugs (e.g. March 31 - 1 month).
func PreviousMonth(now time.Time) (int, int) {
	firstOfThis := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	prev := firstOfThis.AddDate(0, -1, 0)
	return prev.Year(), int(prev.Month())
}

// PeriodLabel formats a period like "June 2026".
func PeriodLabel(year, month int) string {
	return fmt.Sprintf("%s %d", time.Month(month).String(), year)
}

// HistoryWindow returns the start_date and end_date (YYYY-MM-DD) for the 6-month
// window ending at (year, month), in the given location.
func HistoryWindow(year, month int, loc *time.Location) (string, string) {
	periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	start := periodStart.AddDate(0, -5, 0)
	end := periodStart.AddDate(0, 1, -1) // last day of the period month
	return start.Format("2006-01-02"), end.Format("2006-01-02")
}

// FormatCurrency renders an amount for display. BRL uses pt-BR grouping
// (R$ 1.234,56); other currencies fall back to a plain "CODE 1,234.56" style.
func FormatCurrency(currency string, amount float64) string {
	switch strings.ToUpper(currency) {
	case "BRL":
		return "R$ " + groupNumber(amount, ".", ",")
	case "USD":
		return "$" + groupNumber(amount, ",", ".")
	case "EUR":
		return "€" + groupNumber(amount, ".", ",")
	default:
		return currency + " " + groupNumber(amount, ",", ".")
	}
}

// groupNumber formats amount with two decimals using the given thousands and
// decimal separators.
func groupNumber(amount float64, thousandsSep, decimalSep string) string {
	neg := amount < 0
	amount = math.Abs(amount)
	// Round to cents.
	cents := int64(math.Round(amount * 100))
	intPart := cents / 100
	frac := cents % 100

	intStr := fmt.Sprintf("%d", intPart)
	var grouped strings.Builder
	n := len(intStr)
	for i, digit := range intStr {
		if i > 0 && (n-i)%3 == 0 {
			grouped.WriteString(thousandsSep)
		}
		grouped.WriteRune(digit)
	}

	result := fmt.Sprintf("%s%s%02d", grouped.String(), decimalSep, frac)
	if neg {
		result = "-" + result
	}
	return result
}
