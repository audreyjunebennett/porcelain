package taskspine

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

func TestRunnerSmallPrivateTaskSelectsLocalCandidate(t *testing.T) {
	journal := &MemoryJournal{}
	worker := &FakeWorker{Result: WorkerResult{Summary: "inspected two files; would propose one patch"}}
	runner := Runner{Sink: journal, Worker: worker, Now: fixedNow}
	task := testTask(PrivacyLocalFirst, 900, 300)

	result, err := runner.Run(context.Background(), task, []Candidate{
		candidate("groq/code-cloud", LocationCloud, 16_000),
		candidate("ollama/qwen-coder", LocationLocal, 8_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Route.SelectedModel != "ollama/qwen-coder" {
		t.Fatalf("result=%+v", result)
	}
	if worker.Calls != 1 {
		t.Fatalf("worker calls=%d", worker.Calls)
	}
	wantKinds := []string{
		"task.created", "context.preflight.completed",
		"routing.candidate_checked", "routing.candidate_checked", "routing.selected",
		"worker.started", "worker.completed", "task.completed",
	}
	if got := eventKinds(journal.Events()); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("event kinds=%v want=%v", got, wantKinds)
	}
	for i, event := range journal.Events() {
		if event.Sequence != i+1 || !event.At.Equal(fixedNow()) {
			t.Fatalf("event %d=%+v", i, event)
		}
	}
}

func TestRunnerOversizedPacketRejectsSmallContextThenSelectsCloud(t *testing.T) {
	journal := &MemoryJournal{}
	worker := &FakeWorker{Result: WorkerResult{Summary: "fake success"}}
	task := testTask(PrivacyCloudAllowed, 9_000, 1_000)

	result, err := (Runner{Sink: journal, Worker: worker}).Run(context.Background(), task, []Candidate{
		candidate("ollama/qwen-coder", LocationLocal, 8_000),
		candidate("gemini/gemini-code", LocationCloud, 32_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Route.SelectedModel != "gemini/gemini-code" {
		t.Fatalf("route=%+v", result.Route)
	}
	if result.Route.Decisions[0].Eligible || !strings.Contains(result.Route.Decisions[0].PrimaryReason(), "context cap") {
		t.Fatalf("small-context decision=%+v", result.Route.Decisions[0])
	}
}

func TestRunnerLocalOnlyBlocksWhenNoLocalCandidateFits(t *testing.T) {
	journal := &MemoryJournal{}
	worker := &FakeWorker{}
	task := testTask(PrivacyLocalOnly, 9_000, 1_000)

	result, err := (Runner{Sink: journal, Worker: worker}).Run(context.Background(), task, []Candidate{
		candidate("ollama/qwen-coder", LocationLocal, 8_000),
		candidate("gemini/gemini-code", LocationCloud, 32_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || result.Route.SelectedModel != "" || worker.Calls != 0 {
		t.Fatalf("result=%+v worker calls=%d", result, worker.Calls)
	}
	if got := eventKinds(journal.Events()); got[len(got)-1] != "task.blocked" {
		t.Fatalf("events=%v", got)
	}
	cloud := result.Route.Decisions[1]
	if cloud.Eligible || !containsReason(cloud.Reasons, "privacy: cloud execution is disabled") {
		t.Fatalf("cloud decision=%+v", cloud)
	}
}

func TestRouteRejectsSpecialistUnavailableUnknownContextAndQuota(t *testing.T) {
	rpm := int64(1)
	unknown := candidate("groq/unknown-general", LocationCloud, 0)
	unknown.Limits.ContextWindow = nil
	quota := candidate("groq/qwen-coder", LocationCloud, 16_000)
	quota.Limits.RPM = &rpm
	quota.Usage.MinuteCalls = 1

	result := Route(testTask(PrivacyCloudAllowed, 100, 10), ContextSummary{TotalTokens: 100, BodyBytes: 10}, []Candidate{
		candidate("ollama/nomic-embed-text", LocationLocal, 8_000),
		{ModelID: "gemini/gemini-code", Location: LocationCloud, CatalogPresent: false, Limits: limits(16_000)},
		unknown,
		quota,
	})
	if result.SelectedModel != "" {
		t.Fatalf("selected=%q decisions=%+v", result.SelectedModel, result.Decisions)
	}
	wants := []string{"capability:", "availability:", "context:", "quota:"}
	for i, want := range wants {
		if !strings.HasPrefix(result.Decisions[i].PrimaryReason(), want) {
			t.Fatalf("decision %d=%+v want prefix %q", i, result.Decisions[i], want)
		}
	}
}

func TestRunnerBodyCapBlocksBeforeWorker(t *testing.T) {
	journal := &MemoryJournal{}
	worker := &FakeWorker{}
	task := testTask(PrivacyCloudAllowed, 100, 10)
	task.Context.SerializedBodyBytes = 9_000
	model := candidate("ollama/qwen-coder", LocationLocal, 16_000)
	maxBody := int64(8_000)
	model.Limits.MaxBodyBytes = &maxBody

	result, err := (Runner{Sink: journal, Worker: worker}).Run(context.Background(), task, []Candidate{model})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || worker.Calls != 0 {
		t.Fatalf("result=%+v worker calls=%d", result, worker.Calls)
	}
	if !strings.Contains(result.Route.Decisions[0].PrimaryReason(), "request body") {
		t.Fatalf("decision=%+v", result.Route.Decisions[0])
	}
}

func TestRunnerInvalidContextPacketBlocksBeforeRouting(t *testing.T) {
	journal := &MemoryJournal{}
	worker := &FakeWorker{}
	task := testTask(PrivacyLocalOnly, 100, 10)
	task.Context.Items[0].Source = ""

	result, err := (Runner{Sink: journal, Worker: worker}).Run(context.Background(), task, []Candidate{
		candidate("ollama/qwen-coder", LocationLocal, 16_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || len(result.Route.Decisions) != 0 || worker.Calls != 0 {
		t.Fatalf("result=%+v worker calls=%d", result, worker.Calls)
	}
	want := []string{"task.created", "task.blocked"}
	if got := eventKinds(journal.Events()); !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func testTask(policy PrivacyPolicy, promptTokens, outputTokens int64) Task {
	return Task{
		ID: "task-1", Request: "inspect the selected code", Privacy: policy,
		MaxOutputTokens: outputTokens,
		Context: ContextPacket{PromptTokens: promptTokens, SerializedBodyBytes: 4096, Items: []ContextItem{{
			Source: "src/main.go", ContentSHA256: "abc123", Bytes: 2048,
			EstimatedTokens: 100, Reason: "directly named by the task",
		}}},
	}
}

func candidate(id string, location ExecutionLocation, contextWindow int64) Candidate {
	return Candidate{
		ModelID: id, Capabilities: []string{"chat", "tools"}, CatalogPresent: true,
		Location: location, Limits: limits(contextWindow),
	}
}

func limits(contextWindow int64) providerlimits.Effective {
	factor := 1.0
	return providerlimits.Effective{ContextWindow: &contextWindow, ContextSafetyFactor: &factor}
}

func fixedNow() time.Time {
	return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
}

func eventKinds(events []Event) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.Kind
	}
	return out
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
