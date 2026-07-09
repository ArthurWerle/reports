package repository

import (
	"errors"

	"github.com/ArthurWerle/reports/internal/model"
	"gorm.io/gorm"
)

type ExecutionRepository interface {
	Create(e *model.ReportExecution) error
	Get(id int64) (*model.ReportExecution, error)
	Update(e *model.ReportExecution) error
	// List returns executions newest-first without the html blob. A nil
	// reportID lists across all reports.
	List(reportID *int64, limit int) ([]model.ReportExecution, error)
	// HasActiveScheduled reports whether a scheduled execution already exists
	// for the period in a non-failed state (dedupe guard).
	HasActiveScheduled(reportID int64, year, month int) (bool, error)
	SaveChart(c *model.ReportChart) error
	GetChart(executionID int64, name string) (*model.ReportChart, error)
}

type executionRepository struct {
	db *gorm.DB
}

func NewExecutionRepository(db *gorm.DB) ExecutionRepository {
	return &executionRepository{db: db}
}

func (r *executionRepository) Create(e *model.ReportExecution) error {
	return r.db.Create(e).Error
}

func (r *executionRepository) Get(id int64) (*model.ReportExecution, error) {
	var e model.ReportExecution
	if err := r.db.First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *executionRepository) Update(e *model.ReportExecution) error {
	return r.db.Save(e).Error
}

func (r *executionRepository) List(reportID *int64, limit int) ([]model.ReportExecution, error) {
	var execs []model.ReportExecution
	// Omit the html blob from list responses; it can be large.
	q := r.db.
		Select("id", "report_id", "period_year", "period_month", "trigger", "status",
			"event_id", "started_at", "finished_at", "duration_ms", "error_message",
			"email_sent_at", "created_at").
		Order("id DESC")
	if reportID != nil {
		q = q.Where("report_id = ?", *reportID)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&execs).Error
	return execs, err
}

func (r *executionRepository) HasActiveScheduled(reportID int64, year, month int) (bool, error) {
	var count int64
	err := r.db.Model(&model.ReportExecution{}).
		Where("report_id = ? AND period_year = ? AND period_month = ? AND trigger = ? AND status <> ?",
			reportID, year, month, model.TriggerScheduled, model.StatusFailed).
		Count(&count).Error
	return count > 0, err
}

func (r *executionRepository) SaveChart(c *model.ReportChart) error {
	// Upsert on (execution_id, name).
	var existing model.ReportChart
	err := r.db.Where("execution_id = ? AND name = ?", c.ExecutionID, c.Name).First(&existing).Error
	if err == nil {
		existing.Image = c.Image
		return r.db.Save(&existing).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.Create(c).Error
}

func (r *executionRepository) GetChart(executionID int64, name string) (*model.ReportChart, error) {
	var c model.ReportChart
	if err := r.db.Where("execution_id = ? AND name = ?", executionID, name).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}
