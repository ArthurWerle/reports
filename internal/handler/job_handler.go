package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ArthurWerle/reports/internal/model"
	"github.com/ArthurWerle/reports/internal/repository"
	"github.com/ArthurWerle/reports/internal/service"
	"github.com/gin-gonic/gin"
)

type JobHandler struct {
	execRepo  repository.ExecutionRepository
	generator *service.Generator
	logger    *slog.Logger
}

func NewJobHandler(execRepo repository.ExecutionRepository, generator *service.Generator, logger *slog.Logger) *JobHandler {
	return &JobHandler{execRepo: execRepo, generator: generator, logger: logger}
}

// Generate is the events delivery callback:
// GET /api/v1/jobs/generate?job_type=generate-report&payload={"execution_id":N}
//
// It always returns 200 so the events retry mechanism (which only covers
// delivery) does not loop forever; generation failures are recorded on the
// execution row instead. Redelivery while running/success is a no-op.
func (h *JobHandler) Generate(c *gin.Context) {
	payload := c.Query("payload")
	var p struct {
		ExecutionID int64 `json:"execution_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		h.logger.Warn("job: invalid payload", "payload", payload, "error", err)
		c.Status(http.StatusOK)
		return
	}

	exec, err := h.execRepo.Get(p.ExecutionID)
	if err != nil {
		h.logger.Warn("job: unknown execution", "execution_id", p.ExecutionID, "error", err)
		c.Status(http.StatusOK)
		return
	}

	switch exec.Status {
	case model.StatusRunning, model.StatusSuccess:
		// Idempotent against redelivery — already handled or in flight.
		c.Status(http.StatusOK)
		return
	}

	// pending or failed → claim it and generate in the background.
	now := time.Now()
	exec.Status = model.StatusRunning
	exec.StartedAt = &now
	exec.ErrorMessage = nil
	exec.FinishedAt = nil
	exec.DurationMs = nil
	if err := h.execRepo.Update(exec); err != nil {
		h.logger.Error("job: mark running", "execution_id", exec.ID, "error", err)
		c.Status(http.StatusOK)
		return
	}

	go h.generator.Generate(exec.ID)
	c.Status(http.StatusOK)
}
