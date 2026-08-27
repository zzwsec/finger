package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/zzwsec/finger/open/internal/config"
	"github.com/zzwsec/finger/open/internal/metrics"
	"github.com/zzwsec/finger/open/internal/topology"
)

type fakeState struct {
	current int
}

func (s *fakeState) Current() (int, error) { return s.current, nil }
func (s *fakeState) Set(id int) error      { s.current = id; return nil }

type fakeMetrics struct {
	counts metrics.Counts
}

func (m fakeMetrics) Counts(context.Context, int, int) (metrics.Counts, error) {
	return m.counts, nil
}

type fakeAutomation struct {
	calls *[]string
}

func (a fakeAutomation) record(name string) error { *a.calls = append(*a.calls, name); return nil }
func (a fakeAutomation) Package(context.Context, topology.Game) error {
	return a.record("package")
}
func (a fakeAutomation) Install(context.Context, topology.Game, config.Install) error {
	return a.record("install")
}
func (a fakeAutomation) RemoveWhitelist(context.Context, []string, int) error {
	return a.record("whitelist")
}
func (a fakeAutomation) AddLimit(context.Context, []string, int) error {
	return a.record("limit")
}
func (a fakeAutomation) ReloadLogins(context.Context, []string) error {
	return a.record("reload")
}

type fakeCDN struct {
	calls *[]string
}

func (c fakeCDN) Flush(context.Context, int) error {
	*c.calls = append(*c.calls, "cdn")
	return nil
}

func TestCheckRunsOpenWorkflow(t *testing.T) {
	topology := testTopology(t)
	state := &fakeState{current: 1}
	var calls []string
	service := New(Dependencies{
		Config: config.Config{
			LoginHost:         "10.0.0.3",
			RegisterThreshold: 10,
			RechargeThreshold: 5,
			MoneyThreshold:    6,
		},
		Topology:   topology,
		State:      state,
		Metrics:    fakeMetrics{counts: metrics.Counts{Registered: 10}},
		Automation: fakeAutomation{calls: &calls},
		CDN:        fakeCDN{calls: &calls},
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	opened, err := service.check(context.Background(), 1)
	if err != nil {
		t.Fatalf("check() error = %v", err)
	}
	if !opened || state.current != 2 {
		t.Fatalf("check() opened = %v, state = %d", opened, state.current)
	}
	want := []string{"package", "install", "whitelist", "reload", "cdn", "limit", "reload"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("workflow calls = %v, want %v", calls, want)
	}
}

func TestCheckDoesNothingBelowThreshold(t *testing.T) {
	var calls []string
	service := New(Dependencies{
		Config: config.Config{
			LoginHost:         "10.0.0.3",
			RegisterThreshold: 10,
			RechargeThreshold: 5,
			MoneyThreshold:    6,
		},
		Topology:   testTopology(t),
		State:      &fakeState{current: 1},
		Metrics:    fakeMetrics{counts: metrics.Counts{Registered: 9, Recharged: 4}},
		Automation: fakeAutomation{calls: &calls},
		CDN:        fakeCDN{calls: &calls},
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	opened, err := service.check(context.Background(), 1)
	if err != nil || opened || len(calls) != 0 {
		t.Fatalf("check() = %v, %v, calls %v", opened, err, calls)
	}
}

func TestRunExitsWhenNextGameIsNotConfigured(t *testing.T) {
	directory := t.TempDir()
	games := filepath.Join(directory, "games.txt")
	if err := os.WriteFile(games, []byte("10.0.0.1 [61]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	topology, err := topology.Load(games)
	if err != nil {
		t.Fatal(err)
	}

	service := New(Dependencies{
		Config:   config.Config{PollInterval: time.Hour},
		Topology: topology,
		State:    &fakeState{current: 61},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("Run() waited for the context instead of exiting immediately")
	}
}

func testTopology(t *testing.T) *topology.Topology {
	t.Helper()
	directory := t.TempDir()
	games := filepath.Join(directory, "games.txt")
	if err := os.WriteFile(games, []byte("10.0.0.1 [1]\n10.0.0.2 [2]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := topology.Load(games)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
