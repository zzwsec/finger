package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileCurrentAndSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current_game")
	if err := os.WriteFile(path, []byte("3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := New(path)

	current, err := state.Current()
	if err != nil || current != 3 {
		t.Fatalf("Current() = %d, %v", current, err)
	}
	if err := state.Set(4); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	current, err = state.Current()
	if err != nil || current != 4 {
		t.Fatalf("Current() after Set = %d, %v", current, err)
	}
}

func TestFilePendingLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current_game")
	if err := os.WriteFile(path, []byte("62\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := New(path)

	pending, err := state.Pending()
	if err != nil || pending != nil {
		t.Fatalf("Pending() before set = %#v, %v", pending, err)
	}
	want := Pending{CurrentGame: 62, NextGame: 63, NextStep: "cdn"}
	if err := state.SetPending(want); err != nil {
		t.Fatalf("SetPending() error = %v", err)
	}
	pending, err = state.Pending()
	if err != nil || pending == nil || *pending != want {
		t.Fatalf("Pending() = %#v, %v, want %#v", pending, err, want)
	}
	if err := state.ClearPending(); err != nil {
		t.Fatalf("ClearPending() error = %v", err)
	}
	pending, err = state.Pending()
	if err != nil || pending != nil {
		t.Fatalf("Pending() after clear = %#v, %v", pending, err)
	}
}
