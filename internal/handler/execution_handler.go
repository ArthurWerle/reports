package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ArthurWerle/reports/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ExecutionHandler struct {
	execRepo repository.ExecutionRepository
	logger   *slog.Logger
}

func NewExecutionHandler(execRepo repository.ExecutionRepository, logger *slog.Logger) *ExecutionHandler {
	return &ExecutionHandler{execRepo: execRepo, logger: logger}
}

// List returns executions newest-first (without the html blob). Optional
// report_id and limit query params.
func (h *ExecutionHandler) List(c *gin.Context) {
	var reportID *int64
	if v := c.Query("report_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report_id"})
			return
		}
		reportID = &id
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	execs, err := h.execRepo.List(reportID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list executions", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, execs)
}

// HTML serves the stored web-mode report HTML.
func (h *ExecutionHandler) HTML(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	exec, err := h.execRepo.Get(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(http.StatusNotFound, "execution not found")
			return
		}
		c.String(http.StatusInternalServerError, "failed to load execution")
		return
	}
	if exec.HTML == nil || *exec.HTML == "" {
		c.String(http.StatusNotFound, "no report html for this execution")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(*exec.HTML))
}

// Chart serves a rendered PNG chart.
func (h *ExecutionHandler) Chart(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	name := c.Param("name")
	chart, err := h.execRepo.GetChart(id, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(http.StatusNotFound, "chart not found")
			return
		}
		c.String(http.StatusInternalServerError, "failed to load chart")
		return
	}
	c.Data(http.StatusOK, "image/png", chart.Image)
}
