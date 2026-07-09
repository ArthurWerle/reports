package service

import (
	"sort"
	"sync"

	"github.com/ArthurWerle/events/client"
	"github.com/ArthurWerle/reports/internal/model"
	"gorm.io/gorm"
)

// fakeReportRepo is an in-memory ReportRepository.
type fakeReportRepo struct {
	mu      sync.Mutex
	reports map[int64]*model.Report
	nextID  int64
}

func newFakeReportRepo() *fakeReportRepo {
	return &fakeReportRepo{reports: map[int64]*model.Report{}, nextID: 1}
}

func (f *fakeReportRepo) add(r *model.Report) *model.Report {
	f.mu.Lock()
	defer f.mu.Unlock()
	r.ID = f.nextID
	f.nextID++
	f.reports[r.ID] = r
	return r
}

func (f *fakeReportRepo) List() ([]model.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.Report
	for _, r := range f.reports {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeReportRepo) ListEnabled() ([]model.Report, error) {
	all, _ := f.List()
	var out []model.Report
	for _, r := range all {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeReportRepo) Get(id int64) (*model.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.reports[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeReportRepo) Create(r *model.Report) error {
	f.add(r)
	return nil
}

func (f *fakeReportRepo) Update(r *model.Report) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	f.reports[r.ID] = &cp
	return nil
}

func (f *fakeReportRepo) Delete(id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.reports, id)
	return nil
}

// fakeExecRepo is an in-memory ExecutionRepository.
type fakeExecRepo struct {
	mu     sync.Mutex
	execs  map[int64]*model.ReportExecution
	charts map[string]*model.ReportChart
	nextID int64
}

func newFakeExecRepo() *fakeExecRepo {
	return &fakeExecRepo{
		execs:  map[int64]*model.ReportExecution{},
		charts: map[string]*model.ReportChart{},
		nextID: 1,
	}
}

func (f *fakeExecRepo) Create(e *model.ReportExecution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e.ID = f.nextID
	f.nextID++
	cp := *e
	f.execs[e.ID] = &cp
	return nil
}

func (f *fakeExecRepo) Get(id int64) (*model.ReportExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.execs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *e
	return &cp, nil
}

func (f *fakeExecRepo) Update(e *model.ReportExecution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *e
	f.execs[e.ID] = &cp
	return nil
}

func (f *fakeExecRepo) List(reportID *int64, limit int) ([]model.ReportExecution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []model.ReportExecution
	for _, e := range f.execs {
		if reportID != nil && e.ReportID != *reportID {
			continue
		}
		cp := *e
		cp.HTML = nil
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeExecRepo) HasActiveScheduled(reportID int64, year, month int) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.execs {
		if e.ReportID == reportID && e.PeriodYear == year && e.PeriodMonth == month &&
			e.Trigger == model.TriggerScheduled && e.Status != model.StatusFailed {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeExecRepo) SaveChart(c *model.ReportChart) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *c
	f.charts[chartKey(c.ExecutionID, c.Name)] = &cp
	return nil
}

func (f *fakeExecRepo) GetChart(executionID int64, name string) (*model.ReportChart, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.charts[chartKey(executionID, name)]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *c
	return &cp, nil
}

func (f *fakeExecRepo) chartCount(execID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.charts {
		if c.ExecutionID == execID {
			n++
		}
	}
	return n
}

func chartKey(execID int64, name string) string {
	return name + "@" + itoa(execID)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	s := string(buf[i:])
	if neg {
		s = "-" + s
	}
	return s
}

// fakeEnqueuer records enqueue calls and returns a synthetic event id.
type fakeEnqueuer struct {
	mu     sync.Mutex
	calls  []client.Event
	nextID uint
	err    error
}

func (f *fakeEnqueuer) Enqueue(jobType, payload, callbackURL string) (client.Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return client.Event{}, f.err
	}
	f.nextID++
	ev := client.Event{ID: f.nextID, JobType: jobType, Payload: payload, CallbackURL: callbackURL, Status: "pending"}
	f.calls = append(f.calls, ev)
	return ev, nil
}

func (f *fakeEnqueuer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}
