package model

import "time"

// Report is a report configuration + schedule.
type Report struct {
	ID         int64     `gorm:"primaryKey" json:"id"`
	Name       string    `json:"name"`
	DayOfMonth int       `gorm:"column:day_of_month" json:"day_of_month"`
	Hour       int       `json:"hour"`
	Recipients string    `json:"recipients"` // comma-separated email addresses
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Report) TableName() string { return "reports" }

// Execution status values.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
)

// Execution trigger values.
const (
	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
)

// ReportExecution is a single run of a report for one period.
type ReportExecution struct {
	ID           int64      `gorm:"primaryKey" json:"id"`
	ReportID     int64      `gorm:"column:report_id" json:"report_id"`
	PeriodYear   int        `gorm:"column:period_year" json:"period_year"`
	PeriodMonth  int        `gorm:"column:period_month" json:"period_month"`
	Trigger      string     `json:"trigger"`
	Status       string     `json:"status"`
	EventID      *int64     `gorm:"column:event_id" json:"event_id,omitempty"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"started_at,omitempty"`
	FinishedAt   *time.Time `gorm:"column:finished_at" json:"finished_at,omitempty"`
	DurationMs   *int64     `gorm:"column:duration_ms" json:"duration_ms,omitempty"`
	ErrorMessage *string    `gorm:"column:error_message" json:"error_message,omitempty"`
	// Insights holds the raw /report-insights JSON. Stored in a jsonb column;
	// kept as a string so we don't need an extra datatypes dependency.
	Insights    *string    `gorm:"column:insights;type:jsonb" json:"-"`
	HTML        *string    `gorm:"column:html" json:"-"`
	EmailSentAt *time.Time `gorm:"column:email_sent_at" json:"email_sent_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (ReportExecution) TableName() string { return "report_executions" }

// ReportChart is a rendered PNG chart belonging to an execution.
type ReportChart struct {
	ID          int64  `gorm:"primaryKey" json:"id"`
	ExecutionID int64  `gorm:"column:execution_id" json:"execution_id"`
	Name        string `json:"name"`
	Image       []byte `gorm:"column:image" json:"-"`
}

func (ReportChart) TableName() string { return "report_charts" }
