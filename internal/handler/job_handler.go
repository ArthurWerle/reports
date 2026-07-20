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
// It runs the full pipeline synchronously and returns 200 only when the report
// was generated and the email was sent, so the events service marks the job
// "done" only on real completion. Any failure returns a non-2xx, which events
// retries and ultimately records as "failed". Redelivery while running/success
// is a 200 no-op; a failed execution is re-run (that is the retry path).
func (h *JobHandler) Generate(c *gin.Context) {
	payload := c.Query("payload")
	var p struct {
		ExecutionID int64 `json:"execution_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		h.logger.Warn("job: invalid payload", "payload", payload, "error", err)
		c.String(http.StatusBadRequest, "invalid payload")
		return
	}

	exec, err := h.execRepo.Get(p.ExecutionID)
	if err != nil {
		h.logger.Warn("job: unknown execution", "execution_id", p.ExecutionID, "error", err)
		c.String(http.StatusNotFound, "unknown execution")
		return
	}

	h.logger.Info("job: callback received", "execution_id", exec.ID, "status", exec.Status)

	switch exec.Status {
	case model.StatusRunning, model.StatusSuccess:
		// Idempotent against redelivery — already handled or in flight.
		h.logger.Info("job: idempotent no-op", "execution_id", exec.ID, "status", exec.Status)
		c.Status(http.StatusOK)
		return
	}

	// pending or failed → claim it and generate.
	now := time.Now()
	exec.Status = model.StatusRunning
	exec.StartedAt = &now
	exec.ErrorMessage = nil
	exec.FinishedAt = nil
	exec.DurationMs = nil
	if err := h.execRepo.Update(exec); err != nil {
		h.logger.Error("job: mark running", "execution_id", exec.ID, "error", err)
		c.String(http.StatusInternalServerError, "mark running failed")
		return
	}

	if err := h.generator.Generate(exec.ID); err != nil {
		h.logger.Error("job: generation failed",
			"execution_id", exec.ID, "duration", time.Since(now).String(), "error", err)
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	h.logger.Info("job: generation succeeded, email sent",
		"execution_id", exec.ID, "duration", time.Since(now).String())
	c.Status(http.StatusOK)
}
