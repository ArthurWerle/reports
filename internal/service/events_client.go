package service

import (
	"fmt"

	"github.com/ArthurWerle/events/client"
)

// JobTypeGenerateReport is the events job type for report generation.
const JobTypeGenerateReport = "generate-report"

// Enqueuer is the subset of the events client the reports service uses. The
// concrete *client.Client from github.com/ArthurWerle/events satisfies it; a
// fake is injected in tests.
type Enqueuer interface {
	Enqueue(jobType, payload, callbackURL string) (client.Event, error)
}

// NewEventsClient returns the real events client as an Enqueuer.
func NewEventsClient(baseURL string) Enqueuer {
	return client.New(baseURL)
}

// EnqueueGeneration enqueues a generate-report job for the execution and returns
// the events-assigned event id. callbackBaseURL is the address the events
// container uses to call back into this service.
func EnqueueGeneration(enq Enqueuer, execID int64, callbackBaseURL string) (int64, error) {
	payload := fmt.Sprintf(`{"execution_id":%d}`, execID)
	callbackURL := callbackBaseURL + "/api/v1/jobs/generate"
	ev, err := enq.Enqueue(JobTypeGenerateReport, payload, callbackURL)
	if err != nil {
		return 0, err
	}
	return int64(ev.ID), nil
}
