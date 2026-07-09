package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ArthurWerle/reports/internal/model"
	"github.com/ArthurWerle/reports/internal/repository"
	"github.com/ArthurWerle/reports/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct {
	reportRepo      repository.ReportRepository
	execRepo        repository.ExecutionRepository
	enq             service.Enqueuer
	callbackBaseURL string
	loc             *time.Location
	logger          *slog.Logger
}

func NewReportHandler(
	reportRepo repository.ReportRepository,
	execRepo repository.ExecutionRepository,
	enq service.Enqueuer,
	callbackBaseURL string,
	loc *time.Location,
	logger *slog.Logger,
) *ReportHandler {
	return &ReportHandler{
		reportRepo:      reportRepo,
		execRepo:        execRepo,
		enq:             enq,
		callbackBaseURL: callbackBaseURL,
		loc:             loc,
		logger:          logger,
	}
}

type reportInput struct {
	Name       string `json:"name"`
	DayOfMonth int    `json:"day_of_month"`
	Hour       int    `json:"hour"`
	Recipients string `json:"recipients"`
	Enabled    *bool  `json:"enabled"`
}

func (in reportInput) validate() (string, bool) {
	if in.Name == "" {
		return "name is required", false
	}
	if in.DayOfMonth < 1 || in.DayOfMonth > 28 {
		return "day_of_month must be between 1 and 28", false
	}
	if in.Hour < 0 || in.Hour > 23 {
		return "hour must be between 0 and 23", false
	}
	return "", true
}

func (h *ReportHandler) List(c *gin.Context) {
	reports, err := h.reportRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list reports", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, reports)
}

func (h *ReportHandler) Create(c *gin.Context) {
	var in reportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "details": err.Error()})
		return
	}
	if msg, ok := in.validate(); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	report := &model.Report{
		Name:       in.Name,
		DayOfMonth: in.DayOfMonth,
		Hour:       in.Hour,
		Recipients: in.Recipients,
		Enabled:    enabled,
	}
	if err := h.reportRepo.Create(report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create report", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, report)
}

func (h *ReportHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	report, err := h.reportRepo.Get(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load report", "details": err.Error()})
		return
	}

	var in reportInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "details": err.Error()})
		return
	}
	if msg, ok := in.validate(); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	report.Name = in.Name
	report.DayOfMonth = in.DayOfMonth
	report.Hour = in.Hour
	report.Recipients = in.Recipients
	if in.Enabled != nil {
		report.Enabled = *in.Enabled
	}
	if err := h.reportRepo.Update(report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update report", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *ReportHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.reportRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete report", "details": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

type runInput struct {
	Month *int `json:"month"`
	Year  *int `json:"year"`
}

// Run performs a manual run. Body {month, year} is optional and defaults to the
// previous month. Manual runs are never deduped.
func (h *ReportHandler) Run(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if _, err := h.reportRepo.Get(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "report not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load report", "details": err.Error()})
		return
	}

	var in runInput
	// Body is optional; ignore decode errors on empty/invalid body and default.
	_ = c.ShouldBindJSON(&in)

	year, month := service.PreviousMonth(time.Now().In(h.loc))
	if in.Month != nil && in.Year != nil {
		if *in.Month < 1 || *in.Month > 12 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "month must be between 1 and 12"})
			return
		}
		if *in.Year < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year"})
			return
		}
		year, month = *in.Year, *in.Month
	}

	exec, err := service.EnqueueExecution(h.execRepo, h.enq, h.callbackBaseURL, id, year, month, model.TriggerManual, h.logger)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to enqueue generation", "details": err.Error(), "execution": exec})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"execution": exec})
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return id, true
}
