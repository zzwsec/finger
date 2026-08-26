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
