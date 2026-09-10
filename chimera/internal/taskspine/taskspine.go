// Package taskspine is a model-free proving ground for future coding-task
// orchestration. It records an append-only event trail around context preflight,
// explainable routing, and worker execution without performing model calls or
// filesystem writes itself.
package taskspine

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type PrivacyPolicy string

const (
	PrivacyLocalOnly    PrivacyPolicy = "local_only"
	PrivacyLocalFirst   PrivacyPolicy = "local_first"
	PrivacyCloudAllowed PrivacyPolicy = "cloud_allowed"
)

type ContextItem struct {
	Source          string
	ContentSHA256   string
	Bytes           int64
	EstimatedTokens int64
	Reason          string
}

type ContextPacket struct {
	PromptTokens        int64
	SerializedBodyBytes int64
	Items               []ContextItem
}

type Task struct {
	ID              string
	Request         string
	Privacy         PrivacyPolicy
	MaxOutputTokens int64
	Context         ContextPacket
}

type ContextSummary struct {
	PromptTokens  int64
	ContextTokens int64
	TotalTokens   int64
	BodyBytes     int64
	Items         int
}

func PreflightContext(task Task) (ContextSummary, error) {
	if strings.TrimSpace(task.ID) == "" {
		return ContextSummary{}, fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(task.Request) == "" {
		return ContextSummary{}, fmt.Errorf("task request is required")
	}
	switch task.Privacy {
	case PrivacyLocalOnly, PrivacyLocalFirst, PrivacyCloudAllowed:
	default:
		return ContextSummary{}, fmt.Errorf("recognized privacy policy is required")
	}
	if task.Context.PromptTokens < 0 || task.Context.SerializedBodyBytes < 0 || task.MaxOutputTokens < 0 {
		return ContextSummary{}, fmt.Errorf("token estimates must not be negative")
	}
	summary := ContextSummary{
		PromptTokens: task.Context.PromptTokens,
		BodyBytes:    task.Context.SerializedBodyBytes,
		Items:        len(task.Context.Items),
	}
	for i, item := range task.Context.Items {
		if strings.TrimSpace(item.Source) == "" {
			return ContextSummary{}, fmt.Errorf("context item %d source is required", i)
		}
		if item.Bytes < 0 || item.EstimatedTokens < 0 {
			return ContextSummary{}, fmt.Errorf("context item %d estimates must not be negative", i)
		}
		summary.ContextTokens += item.EstimatedTokens
	}
	summary.TotalTokens = summary.PromptTokens + summary.ContextTokens
	return summary, nil
}

type Event struct {
	Sequence  int
	TaskID    string
	Kind      string
	At        time.Time
	Candidate string
	Reason    string
	Fields    map[string]any
}

type EventSink interface {
	Append(Event) error
}

type MemoryJournal struct {
	events []Event
}

func (j *MemoryJournal) Append(event Event) error {
	j.events = append(j.events, event)
	return nil
}

func (j *MemoryJournal) Events() []Event {
	return append([]Event(nil), j.events...)
}

type Worker interface {
	Execute(context.Context, Task, RouteResult) (WorkerResult, error)
}

type WorkerResult struct {
	Summary string
}

type RunResult struct {
	Status  string
	Context ContextSummary
	Route   RouteResult
	Worker  WorkerResult
}

type Runner struct {
	Sink   EventSink
	Worker Worker
	Now    func() time.Time
}

func (r Runner) Run(ctx context.Context, task Task, candidates []Candidate) (RunResult, error) {
	if r.Sink == nil {
		return RunResult{}, fmt.Errorf("event sink is required")
	}
	seq := 0
	emit := func(event Event) error {
		seq++
		event.Sequence = seq
		event.TaskID = task.ID
		if r.Now != nil {
			event.At = r.Now()
		} else {
			event.At = time.Now()
		}
		return r.Sink.Append(event)
	}
	if err := emit(Event{Kind: "task.created"}); err != nil {
		return RunResult{}, err
	}

	summary, err := PreflightContext(task)
	if err != nil {
		_ = emit(Event{Kind: "task.blocked", Reason: "invalid_context_packet", Fields: map[string]any{"detail": err.Error()}})
		return RunResult{Status: "blocked"}, nil
	}
	if err := emit(Event{Kind: "context.preflight.completed", Fields: map[string]any{
		"prompt_tokens": summary.PromptTokens, "context_tokens": summary.ContextTokens,
		"body_bytes": summary.BodyBytes, "items": summary.Items,
	}}); err != nil {
		return RunResult{}, err
	}

	route := Route(task, summary, candidates)
	for _, decision := range route.Decisions {
		if err := emit(Event{
			Kind:      "routing.candidate_checked",
			Candidate: decision.ModelID,
			Reason:    decision.PrimaryReason(),
			Fields:    map[string]any{"eligible": decision.Eligible, "reasons": decision.Reasons},
		}); err != nil {
			return RunResult{}, err
		}
	}
	if route.SelectedModel == "" {
		if err := emit(Event{Kind: "task.blocked", Reason: "no_eligible_model"}); err != nil {
			return RunResult{}, err
		}
		return RunResult{Status: "blocked", Context: summary, Route: route}, nil
	}
	if err := emit(Event{Kind: "routing.selected", Candidate: route.SelectedModel}); err != nil {
		return RunResult{}, err
	}
	if r.Worker == nil {
		return RunResult{}, fmt.Errorf("worker is required after routing")
	}
	if err := emit(Event{Kind: "worker.started", Candidate: route.SelectedModel}); err != nil {
		return RunResult{}, err
	}
	workerResult, err := r.Worker.Execute(ctx, task, route)
	if err != nil {
		_ = emit(Event{Kind: "worker.failed", Candidate: route.SelectedModel, Reason: err.Error()})
		_ = emit(Event{Kind: "task.failed", Reason: "worker_failed"})
		return RunResult{Status: "failed", Context: summary, Route: route}, err
	}
	if err := emit(Event{Kind: "worker.completed", Candidate: route.SelectedModel, Fields: map[string]any{"summary": workerResult.Summary}}); err != nil {
		return RunResult{}, err
	}
	if err := emit(Event{Kind: "task.completed"}); err != nil {
		return RunResult{}, err
	}
	return RunResult{Status: "completed", Context: summary, Route: route, Worker: workerResult}, nil
}

// FakeWorker returns a configured result and records calls. It deliberately has
// no model client or tool executor, so task-spine tests cannot make real calls.
type FakeWorker struct {
	Result WorkerResult
	Err    error
	Calls  int
}

func (w *FakeWorker) Execute(_ context.Context, _ Task, _ RouteResult) (WorkerResult, error) {
	w.Calls++
	return w.Result, w.Err
}
