package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// InsightsRequest is the body posted to ai-internal's /report-insights.
type InsightsRequest struct {
	Month                 int            `json:"month"`
	Year                  int            `json:"year"`
	Language              string         `json:"language"`
	Overview              MonthOverview  `json:"overview"`
	ExpensesByCategory    []NamedTotal   `json:"expensesByCategory"`
	ExpensesBySubcategory []NamedTotal   `json:"expensesBySubcategory"`
	ExpensesByLocation    []NamedTotal   `json:"expensesByLocation"`
	MonthlyHistory        []HistoryPoint `json:"monthlyHistory"`
	BiggestTransactions   []BiggestTx    `json:"biggestTransactions,omitempty"`
}

// Insights is the structured prose returned by /report-insights.
type Insights struct {
	Headline   string   `json:"headline"`
	Highlights []string `json:"highlights"`
	Concerns   []string `json:"concerns"`
	Closing    string   `json:"closing"`
}

// InsightsClient talks to ai-internal's /report-insights endpoint.
type InsightsClient struct {
	baseURL string
	client  *http.Client
}

func NewInsightsClient(baseURL string) *InsightsClient {
	return &InsightsClient{
		baseURL: baseURL,
		// The LLM call can take a while; allow more than the events delivery timeout.
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *InsightsClient) Generate(ctx context.Context, req InsightsRequest) (*Insights, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal insights request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/report-insights", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build insights request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call ai-internal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai-internal returned status %d", resp.StatusCode)
	}

	var out struct {
		Success bool      `json:"success"`
		Data    *Insights `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode insights response: %w", err)
	}
	if !out.Success || out.Data == nil {
		return nil, fmt.Errorf("ai-internal reported failure (success=%v)", out.Success)
	}
	return out.Data, nil
}
