package repository

import (
	"github.com/ArthurWerle/reports/internal/model"
	"gorm.io/gorm"
)

type ReportRepository interface {
	List() ([]model.Report, error)
	ListEnabled() ([]model.Report, error)
	Get(id int64) (*model.Report, error)
	Create(r *model.Report) error
	Update(r *model.Report) error
	Delete(id int64) error
}

type reportRepository struct {
	db *gorm.DB
}

func NewReportRepository(db *gorm.DB) ReportRepository {
	return &reportRepository{db: db}
}

func (r *reportRepository) List() ([]model.Report, error) {
	var reports []model.Report
	err := r.db.Order("id ASC").Find(&reports).Error
	return reports, err
}

func (r *reportRepository) ListEnabled() ([]model.Report, error) {
	var reports []model.Report
	err := r.db.Where("enabled = ?", true).Order("id ASC").Find(&reports).Error
	return reports, err
}

func (r *reportRepository) Get(id int64) (*model.Report, error) {
	var report model.Report
	if err := r.db.First(&report, id).Error; err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *reportRepository) Create(rep *model.Report) error {
	return r.db.Create(rep).Error
}

func (r *reportRepository) Update(rep *model.Report) error {
	return r.db.Save(rep).Error
}

func (r *reportRepository) Delete(id int64) error {
	return r.db.Delete(&model.Report{}, id).Error
}
