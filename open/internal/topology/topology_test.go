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
	if !exists || game.Host != "10.0.0.2" || game.Index != 0 {
		t.Fatalf("Game(2) = %+v, %v", game, exists)
	}
	game3, exists := topology.Game(3)
	if !exists || game3.Host != "10.0.0.1" || game3.Index != 1 {
		t.Fatalf("Game(3) = %+v, %v", game3, exists)
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
