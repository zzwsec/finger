package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type File struct {
	path string
}

type Pending struct {
	CurrentGame int    `json:"current_game"`
	NextGame    int    `json:"next_game"`
	NextStep    string `json:"next_step"`
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
	if err := writeAtomic(f.path, []byte(strconv.Itoa(id)+"\n")); err != nil {
		return fmt.Errorf("write current game: %w", err)
	}
	return nil
}

func (f *File) Pending() (*Pending, error) {
	content, err := os.ReadFile(f.pendingPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending open: %w", err)
	}
	var pending Pending
	if err := json.Unmarshal(content, &pending); err != nil {
		return nil, fmt.Errorf("decode pending open: %w", err)
	}
	if pending.CurrentGame <= 0 || pending.NextGame != pending.CurrentGame+1 || strings.TrimSpace(pending.NextStep) == "" {
		return nil, fmt.Errorf("pending open state is invalid")
	}
	return &pending, nil
}

func (f *File) SetPending(pending Pending) error {
	if pending.CurrentGame <= 0 || pending.NextGame != pending.CurrentGame+1 || strings.TrimSpace(pending.NextStep) == "" {
		return fmt.Errorf("pending open state is invalid")
	}
	content, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("encode pending open: %w", err)
	}
	content = append(content, '\n')
	if err := writeAtomic(f.pendingPath(), content); err != nil {
		return fmt.Errorf("write pending open: %w", err)
	}
	return nil
}

func (f *File) ClearPending() error {
	if err := os.Remove(f.pendingPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pending open: %w", err)
	}
	if err := syncDirectory(filepath.Dir(f.path)); err != nil {
		return fmt.Errorf("sync pending open directory: %w", err)
	}
	return nil
}

func (f *File) pendingPath() string {
	return f.path + ".pending"
}

func writeAtomic(path string, content []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".current-game-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return syncDirectory(directory)
}

func syncDirectory(directory string) error {
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer directoryHandle.Close()
	if err := directoryHandle.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
