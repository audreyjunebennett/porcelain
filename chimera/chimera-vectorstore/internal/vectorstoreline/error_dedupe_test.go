package vectorstoreline

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const (
	optErrMsg      = "Optimization error: Service internal error: IO Error: Access is denied. (os error 5)"
	panicOccurred  = "Panic occurred in file lib\\collection\\src\\update_handler.rs at line 387: " + optErrMsg
	panicBacktrace = "Panic backtrace: \n   0: unknown\n   1: unknown"
)

func postProcessRaw(t *testing.T, raw string) map[string]any {
	t.Helper()
	b := postProcessNormalizedLine(NormalizePayload(raw))
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

func lineLevel(m map[string]any) string {
	if m == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(m["level"].(string)))
}

func TestErrorStormDedupe_backtraceAlwaysDebug(t *testing.T) {
	resetErrorDedupeForTest()
	raw := `{"timestamp":"t","level":"ERROR","fields":{"message":` + jsonString(panicBacktrace) + `},"target":"qdrant::startup"}`
	m := postProcessRaw(t, raw)
	if m["msg"] != "vectorstore.runtime.panic" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if lineLevel(m) != "DEBUG" {
		t.Fatalf("level=%v want DEBUG", m["level"])
	}
}

func TestErrorStormDedupe_repeatedTripleYieldsOneOperatorLine(t *testing.T) {
	resetErrorDedupeForTest()
	errorDedupeNow = func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) }
	defer func() { errorDedupeNow = time.Now }()

	traceRaw := `{"timestamp":"t","level":"ERROR","fields":{"message":` + jsonString(optErrMsg) + `},"target":"collection::update_handler"}`
	panicRaw := `{"timestamp":"t","level":"ERROR","fields":{"message":` + jsonString(panicOccurred) + `},"target":"qdrant::startup"}`
	backRaw := `{"timestamp":"t","level":"ERROR","fields":{"message":` + jsonString(panicBacktrace) + `},"target":"qdrant::startup"}`

	operator := 0
	debug := 0
	for i := 0; i < 50; i++ {
		for _, raw := range []string{traceRaw, backRaw, panicRaw} {
			m := postProcessRaw(t, raw)
			switch lineLevel(m) {
			case "DEBUG":
				debug++
			default:
				operator++
			}
		}
	}
	if operator > 2 {
		t.Fatalf("operator-level lines=%d want <=2, debug=%d", operator, debug)
	}
	if operator < 1 {
		t.Fatalf("expected at least one operator-level line, got debug=%d", debug)
	}
}

func TestErrorStormDedupe_differentRootCauseEmitsAgain(t *testing.T) {
	resetErrorDedupeForTest()
	errorDedupeNow = func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) }
	defer func() { errorDedupeNow = time.Now }()

	first := postProcessRaw(t, `{"timestamp":"t","level":"ERROR","fields":{"message":`+jsonString(optErrMsg)+`},"target":"collection::update_handler"}`)
	second := postProcessRaw(t, `{"timestamp":"t","level":"ERROR","fields":{"message":"Optimization error: disk full"},"target":"collection::update_handler"}`)

	if lineLevel(first) != "ERROR" {
		t.Fatalf("first level=%v", first["level"])
	}
	if lineLevel(second) != "ERROR" {
		t.Fatalf("second level=%v", second["level"])
	}
}

func TestErrorStormDedupe_windowRefresh(t *testing.T) {
	resetErrorDedupeForTest()
	t0 := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	errorDedupeNow = func() time.Time { return t0 }
	defer func() { errorDedupeNow = time.Now }()

	traceRaw := `{"timestamp":"t","level":"ERROR","fields":{"message":` + jsonString(optErrMsg) + `},"target":"collection::update_handler"}`

	if lineLevel(postProcessRaw(t, traceRaw)) != "ERROR" {
		t.Fatal("first emit should stay ERROR")
	}
	if lineLevel(postProcessRaw(t, traceRaw)) != "DEBUG" {
		t.Fatal("duplicate within window should demote")
	}

	errorDedupeNow = func() time.Time { return t0.Add(31 * time.Second) }
	if lineLevel(postProcessRaw(t, traceRaw)) != "ERROR" {
		t.Fatal("emit after window should stay ERROR")
	}
}

func TestErrorStormDedupe_compactPanicDetail(t *testing.T) {
	resetErrorDedupeForTest()
	errorDedupeNow = func() time.Time { return time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC) }
	defer func() { errorDedupeNow = time.Now }()

	raw := `{"timestamp":"t","level":"ERROR","fields":{"message":` + jsonString(panicOccurred) + `},"target":"qdrant::startup"}`
	m := postProcessRaw(t, raw)
	detail, _ := m["progress_detail"].(string)
	if !strings.Contains(detail, "update_handler.rs") {
		t.Fatalf("progress_detail=%q", detail)
	}
	if !strings.Contains(detail, optErrMsg) {
		t.Fatalf("progress_detail=%q", detail)
	}
	if strings.Contains(detail, "Panic occurred") {
		t.Fatalf("progress_detail should be compact: %q", detail)
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
