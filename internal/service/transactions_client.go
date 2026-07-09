package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// OverviewSide is one side (income or expense) of the month overview.
type OverviewSide struct {
	CurrentMonth        float64  `json:"currentMonth"`
	LastMonth           float64  `json:"lastMonth"`
	PercentageVariation *float64 `json:"percentageVariation"`
}

// MonthOverview mirrors transactions' reports/month-overview response.
type MonthOverview struct {
	Income  OverviewSide `json:"income"`
	Expense OverviewSide `json:"expense"`
}

// NamedTotal is a single breakdown bucket (category / subcategory / location).
type NamedTotal struct {
	Name  string  `json:"name"`
	Total float64 `json:"total"`
}

// HistoryPoint is one month of income/expense in the 6-month history.
type HistoryPoint struct {
	Month   string  `json:"month"`
	Income  float64 `json:"income"`
	Expense float64 `json:"expense"`
}

// BiggestTx is a single large expense used only to enrich insights.
type BiggestTx struct {
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
}

// TransactionsClient fetches aggregated numbers from the transactions service.
type TransactionsClient struct {
	baseURL string
	client  *http.Client
}

func NewTransactionsClient(baseURL string) *TransactionsClient {
	return &TransactionsClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *TransactionsClient) get(ctx context.Context, path string, query url.Values, out interface{}) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("call transactions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("transactions returned status %d for %s", resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode transactions response: %w", err)
	}
	return nil
}

func monthYearQuery(month, year int) url.Values {
	q := url.Values{}
	q.Set("month", fmt.Sprintf("%d", month))
	q.Set("year", fmt.Sprintf("%d", year))
	return q
}

func (c *TransactionsClient) MonthOverview(ctx context.Context, month, year int) (MonthOverview, error) {
	var out MonthOverview
	err := c.get(ctx, "/transactions/reports/month-overview", monthYearQuery(month, year), &out)
	return out, err
}

func (c *TransactionsClient) ExpensesByCategory(ctx context.Context, month, year int) ([]NamedTotal, error) {
	var out struct {
		Categories []struct {
			CategoryName string  `json:"category_name"`
			Total        float64 `json:"total"`
		} `json:"categories"`
	}
	if err := c.get(ctx, "/transactions/reports/monthly-expenses-by-category", monthYearQuery(month, year), &out); err != nil {
		return nil, err
	}
	result := make([]NamedTotal, 0, len(out.Categories))
	for _, r := range out.Categories {
		result = append(result, NamedTotal{Name: r.CategoryName, Total: r.Total})
	}
	return result, nil
}

func (c *TransactionsClient) ExpensesBySubcategory(ctx context.Context, month, year int) ([]NamedTotal, error) {
	var out struct {
		Subcategories []struct {
			SubcategoryName string  `json:"subcategory_name"`
			Total           float64 `json:"total"`
		} `json:"subcategories"`
	}
	if err := c.get(ctx, "/transactions/reports/monthly-expenses-by-subcategory", monthYearQuery(month, year), &out); err != nil {
		return nil, err
	}
	result := make([]NamedTotal, 0, len(out.Subcategories))
	for _, r := range out.Subcategories {
		result = append(result, NamedTotal{Name: r.SubcategoryName, Total: r.Total})
	}
	return result, nil
}

func (c *TransactionsClient) ExpensesByLocation(ctx context.Context, month, year int) ([]NamedTotal, error) {
	var out struct {
		Locations []struct {
			LocationName string  `json:"location_name"`
			Total        float64 `json:"total"`
		} `json:"locations"`
	}
	if err := c.get(ctx, "/transactions/reports/monthly-expenses-by-location", monthYearQuery(month, year), &out); err != nil {
		return nil, err
	}
	result := make([]NamedTotal, 0, len(out.Locations))
	for _, r := range out.Locations {
		result = append(result, NamedTotal{Name: r.LocationName, Total: r.Total})
	}
	return result, nil
}

// MonthlyHistory returns the monthly income/expense points in [startDate, endDate]
// (dates formatted YYYY-MM-DD). The response is a bare array.
func (c *TransactionsClient) MonthlyHistory(ctx context.Context, startDate, endDate string) ([]HistoryPoint, error) {
	q := url.Values{}
	q.Set("start_date", startDate)
	q.Set("end_date", endDate)
	var out []HistoryPoint
	if err := c.get(ctx, "/transactions/reports/monthly-history", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Biggest returns the largest expenses for the month. Best-effort enrichment.
func (c *TransactionsClient) Biggest(ctx context.Context, month, year int) ([]BiggestTx, error) {
	var out struct {
		Transactions []struct {
			Description *string `json:"description"`
			Amount      float64 `json:"amount"`
		} `json:"transactions"`
	}
	if err := c.get(ctx, "/transactions/biggest", monthYearQuery(month, year), &out); err != nil {
		return nil, err
	}
	result := make([]BiggestTx, 0, len(out.Transactions))
	for _, r := range out.Transactions {
		desc := ""
		if r.Description != nil {
			desc = *r.Description
		}
		result = append(result, BiggestTx{Description: desc, Amount: r.Amount})
	}
	return result, nil
}
