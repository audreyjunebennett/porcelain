package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestShouldContinueHotReloadAfterSession(t *testing.T) {
	t.Parallel()
	if !shouldContinueHotReloadAfterSession(errSupervisedReload) {
		t.Fatal("expected reload marker to continue hot-reload loop")
	}
	wrapped := fmt.Errorf("session ended: %w", errSupervisedReload)
	if !shouldContinueHotReloadAfterSession(wrapped) {
		t.Fatal("expected wrapped reload marker to continue hot-reload loop")
	}
	if shouldContinueHotReloadAfterSession(nil) {
		t.Fatal("nil session error should not continue loop")
	}
	if shouldContinueHotReloadAfterSession(context.Canceled) {
		t.Fatal("unrelated errors should not continue loop")
	}
	if shouldContinueHotReloadAfterSession(errors.New("watcher exited")) {
		t.Fatal("unrelated errors should not continue loop")
	}
}

func TestShouldCycleSupervisedReload(t *testing.T) {
	t.Parallel()
	if !shouldCycleSupervisedReload(context.Canceled, true) {
		t.Fatal("reload pending should cycle even on context.Canceled")
	}
	if shouldCycleSupervisedReload(context.Canceled, false) {
		t.Fatal("context.Canceled without reload pending should not cycle")
	}
	if !shouldCycleSupervisedReload(errSupervisedReload, false) {
		t.Fatal("reload marker should cycle without reload pending flag")
	}
}
