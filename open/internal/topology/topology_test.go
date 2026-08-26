package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	directory := t.TempDir()
	gamesFile := filepath.Join(directory, "games.txt")
	writeTestFile(t, gamesFile, "10.0.0.1 [1,3] # game host\n10.0.0.2 [2]\n")

	topology, err := Load(gamesFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	game, exists := topology.Game(2)
	if !exists || game.Host != "10.0.0.2" {
		t.Fatalf("Game(2) = %+v, %v", game, exists)
	}
}

func TestLoadRejectsDuplicateGame(t *testing.T) {
	directory := t.TempDir()
	gamesFile := filepath.Join(directory, "games.txt")
	writeTestFile(t, gamesFile, "10.0.0.1 [1]\n10.0.0.2 [1]\n")

	if _, err := Load(gamesFile); err == nil {
		t.Fatal("Load() error = nil, want duplicate game error")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
