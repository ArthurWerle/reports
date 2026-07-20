package handler

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ArthurWerle/reports/internal/model"
	"github.com/ArthurWerle/reports/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// stubExecRepo implements repository.ExecutionRepository for the callback tests.
type stubExecRepo struct {
	execs   map[int64]*model.ReportExecution
	updates int
	repository.ExecutionRepository
}

func (s *stubExecRepo) Get(id int64) (*model.ReportExecution, error) {
	e, ok := s.execs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *e
	return &cp, nil
}

func (s *stubExecRepo) Update(e *model.ReportExecution) error {
	s.updates++
	cp := *e
	s.execs[e.ID] = &cp
	return nil
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func callGenerate(h *JobHandler, payload string) int {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	q := url.Values{}
	q.Set("job_type", "generate-report")
	q.Set("payload", payload)
	c.Request = httptest.NewRequest("GET", "/api/v1/jobs/generate?"+q.Encode(), nil)
	h.Generate(c)
	return w.Code
}

func TestCallbackIdempotentForRunning(t *testing.T) {
	repo := &stubExecRepo{execs: map[int64]*model.ReportExecution{
		1: {ID: 1, Status: model.StatusRunning},
	}}
	h := NewJobHandler(repo, nil, discardLogger())

	if code := callGenerate(h, `{"execution_id":1}`); code != 200 {
		t.Fatalf("status = %d, want 200", code)
	}
	if repo.updates != 0 {
		t.Errorf("running execution must not be updated/spawned, updates=%d", repo.updates)
	}
	if repo.execs[1].Status != model.StatusRunning {
		t.Errorf("status changed to %q", repo.execs[1].Status)
	}
}

func TestCallbackIdempotentForSuccess(t *testing.T) {
	repo := &stubExecRepo{execs: map[int64]*model.ReportExecution{
		2: {ID: 2, Status: model.StatusSuccess},
	}}
	h := NewJobHandler(repo, nil, discardLogger())

	if code := callGenerate(h, `{"execution_id":2}`); code != 200 {
		t.Fatalf("status = %d, want 200", code)
	}
	if repo.updates != 0 {
		t.Errorf("success execution must not be re-run, updates=%d", repo.updates)
	}
}

func TestCallbackUnknownExecution(t *testing.T) {
	repo := &stubExecRepo{execs: map[int64]*model.ReportExecution{}}
	h := NewJobHandler(repo, nil, discardLogger())
	if code := callGenerate(h, `{"execution_id":999}`); code != 404 {
		t.Errorf("unknown id should return 404, got %d", code)
	}
}

func TestCallbackBadPayload(t *testing.T) {
	repo := &stubExecRepo{execs: map[int64]*model.ReportExecution{}}
	h := NewJobHandler(repo, nil, discardLogger())
	if code := callGenerate(h, `not-json`); code != 400 {
		t.Errorf("bad payload should return 400, got %d", code)
	}
}
