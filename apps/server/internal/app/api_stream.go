package app

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"
)

const (
	defaultStreamPoll = time.Second
	streamHeartbeat   = 15 * time.Second
)

type jobStreamInput struct {
	ID          int64 `path:"id"`
	After       int   `query:"after" minimum:"0" doc:"Only events with a higher seq"`
	LastEventID int   `header:"Last-Event-ID" doc:"Set by EventSource on reconnect; resumes after this seq"`
}

func (a *App) registerJobStream(api huma.API) {
	sse.Register(api, huma.Operation{
		OperationID: "streamJob",
		Method:      http.MethodGet,
		Path:        apiV1 + "/jobs/{id}/stream",
		Summary:     "Stream a job's progress",
		Description: "Sends each progress event as `event`, with the event's seq as the message id, and the job itself as `job` whenever its status changes. The stream ends once the job has finished.",
		Tags:        []string{"jobs"},
		Middlewares: huma.Middlewares{a.requireJob(api)},
	}, map[string]any{
		"event": APIJobEvent{},
		"job":   APIJob{},
	}, a.apiJobStream)
}

func (a *App) requireJob(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
		if err == nil {
			_, err = a.jobs.Job(ctx.Context(), id)
		}
		if err != nil {
			huma.WriteErr(api, ctx, http.StatusNotFound, "job not found")
			return
		}
		next(ctx)
	}
}

func (a *App) apiJobStream(ctx context.Context, in *jobStreamInput, send sse.Sender) {
	after := max(in.After, in.LastEventID)
	var status string
	quiet := time.Now()
	for {
		j, err := a.jobs.Job(ctx, in.ID)
		if err != nil {
			if ctx.Err() == nil {
				a.log.ErrorContext(ctx, "job stream", "job", in.ID, "err", err)
			}
			return
		}
		es, err := a.jobEventsAfter(ctx, in.ID, after)
		if err != nil {
			if ctx.Err() == nil {
				a.log.ErrorContext(ctx, "job stream", "job", in.ID, "err", err)
			}
			return
		}
		for _, e := range es {
			if send(sse.Message{ID: e.Seq, Data: e}) != nil {
				return
			}
			after, quiet = e.Seq, time.Now()
		}
		if string(j.Status) != status {
			if send.Data(apiJob(j)) != nil {
				return
			}
			status, quiet = string(j.Status), time.Now()
		}
		if !j.Status.Active() {
			return
		}
		if time.Since(quiet) >= streamHeartbeat {
			if send.Comment("keepalive") != nil {
				return
			}
			quiet = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(a.streamPoll):
		}
	}
}
