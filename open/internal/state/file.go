package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type File struct {
	path string
}

func New(path string) *File {
	return &File{path: path}
}

func (f *File) Current() (int, error) {
	content, err := os.ReadFile(f.path)
	if err != nil {
		return 0, fmt.Errorf("read current game: %w", err)
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("current game must be a positive integer")
	}
	return id, nil
}

func (f *File) Set(id int) error {
	if id <= 0 {
		return fmt.Errorf("current game must be a positive integer")
	}
	directory := filepath.Dir(f.path)
	temporary, err := os.CreateTemp(directory, ".current-game-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set state permissions: %w", err)
	}
	if _, err := fmt.Fprintf(temporary, "%d\n", id); err != nil {
		temporary.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(temporaryPath, f.path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open state directory: %w", err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}
