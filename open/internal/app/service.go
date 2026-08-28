package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/zzwsec/finger/open/internal/config"
	"github.com/zzwsec/finger/open/internal/metrics"
	workflowstate "github.com/zzwsec/finger/open/internal/state"
	"github.com/zzwsec/finger/open/internal/topology"
)

var errNoNextGame = errors.New("no next game configured")

const (
	stepPackage              = "package"
	stepInstall              = "install"
	stepRemoveWhitelist      = "remove_whitelist"
	stepReloadAfterWhitelist = "reload_after_whitelist"
	stepCDN                  = "cdn"
	stepWaitBeforeLimit      = "wait_before_limit"
	stepAddLimit             = "add_limit"
	stepReloadAfterLimit     = "reload_after_limit"
	stepCommit               = "commit"
)

type State interface {
	Current() (int, error)
	Set(int) error
	Pending() (*workflowstate.Pending, error)
	SetPending(workflowstate.Pending) error
	ClearPending() error
}

type Metrics interface {
	Counts(context.Context, int, int) (metrics.Counts, error)
}

type Automation interface {
	Package(context.Context, topology.Game) error
	Install(context.Context, topology.Game, config.Install) error
	RemoveWhitelist(context.Context, []string, int) error
	AddLimit(context.Context, []string, int) error
	ReloadLogins(context.Context, []string) error
}

type CDN interface {
	Flush(context.Context, int) error
}

type Dependencies struct {
	Config     config.Config
	Topology   *topology.Topology
	State      State
	Metrics    Metrics
	Automation Automation
	CDN        CDN
	Logger     *slog.Logger
}

type Service struct {
	cfg        config.Config
	topology   *topology.Topology
	state      State
	metrics    Metrics
	automation Automation
	cdn        CDN
	logger     *slog.Logger
}

func New(dependencies Dependencies) *Service {
	return &Service{
		cfg:        dependencies.Config,
		topology:   dependencies.Topology,
		state:      dependencies.State,
		metrics:    dependencies.Metrics,
		automation: dependencies.Automation,
		cdn:        dependencies.CDN,
		logger:     dependencies.Logger,
	}
}

func (s *Service) Run(ctx context.Context) error {
	currentID, err := s.state.Current()
	if err != nil {
		return err
	}
	if _, exists := s.topology.Game(currentID); !exists {
		return fmt.Errorf("current game%d is not present in topology", currentID)
	}

	for {
		var opened bool
		pending, pendingErr := s.state.Pending()
		if pendingErr != nil {
			return pendingErr
		}
		if pending != nil {
			s.logger.Info("resuming pending open",
				"current_game", pending.CurrentGame,
				"next_game", pending.NextGame,
				"next_step", pending.NextStep,
			)
			opened, err = s.resumeOpen(ctx, currentID, *pending)
		} else {
			opened, err = s.check(ctx, currentID)
		}
		if errors.Is(err, errNoNextGame) {
			s.logger.Info("no next game configured", "current_game", currentID)
			return nil
		} else if err != nil {
			s.logger.Error("open check failed", "game", currentID, "error", err)
		} else if opened {
			currentID, err = s.state.Current()
			if err != nil {
				return err
			}
		}

		if err := wait(ctx, s.cfg.PollInterval); err != nil {
			return nil
		}
	}
}

func (s *Service) check(ctx context.Context, currentID int) (bool, error) {
	next, exists := s.topology.Game(currentID + 1)
	if !exists {
		return false, errNoNextGame
	}

	counts, err := s.metrics.Counts(ctx, currentID, s.cfg.MoneyThreshold)
	if err != nil {
		return false, err
	}
	s.logger.Info("threshold check",
		"game", currentID,
		"registered", counts.Registered,
		"register_threshold", s.cfg.RegisterThreshold,
		"recharged", counts.Recharged,
		"recharge_threshold", s.cfg.RechargeThreshold,
		"minimum_money", s.cfg.MoneyThreshold,
	)
	if counts.Registered < s.cfg.RegisterThreshold && counts.Recharged < s.cfg.RechargeThreshold {
		return false, nil
	}

	current, _ := s.topology.Game(currentID)
	pending := workflowstate.Pending{
		CurrentGame: current.ID,
		NextGame:    next.ID,
		NextStep:    stepPackage,
	}
	if err := s.state.SetPending(pending); err != nil {
		return false, err
	}
	return s.resumeOpen(ctx, currentID, pending)
}

func (s *Service) resumeOpen(ctx context.Context, currentID int, pending workflowstate.Pending) (bool, error) {
	if pending.NextGame != pending.CurrentGame+1 {
		return false, fmt.Errorf("pending open game IDs are invalid")
	}
	if currentID == pending.NextGame && pending.NextStep == stepCommit {
		if err := s.state.ClearPending(); err != nil {
			return false, err
		}
		return true, nil
	}
	if currentID != pending.CurrentGame {
		return false, fmt.Errorf(
			"pending open expects current game%d, state contains game%d",
			pending.CurrentGame,
			currentID,
		)
	}

	current, currentExists := s.topology.Game(pending.CurrentGame)
	next, nextExists := s.topology.Game(pending.NextGame)
	if !currentExists || !nextExists {
		return false, fmt.Errorf("pending open game is not present in topology")
	}

	login := []string{s.cfg.LoginHost}
	steps := []struct {
		key  string
		name string
		run  func(context.Context) error
	}{
		{stepPackage, "package source game", func(ctx context.Context) error {
			return s.automation.Package(ctx, current)
		}},
		{stepInstall, "install next game", func(ctx context.Context) error {
			return s.automation.Install(ctx, next, s.cfg.Install)
		}},
		{stepRemoveWhitelist, "remove whitelist entry", func(ctx context.Context) error {
			return s.automation.RemoveWhitelist(ctx, login, next.ID)
		}},
		{stepReloadAfterWhitelist, "reload login services", func(ctx context.Context) error {
			return s.automation.ReloadLogins(ctx, login)
		}},
		{stepCDN, "flush CDN", func(ctx context.Context) error { return s.cdn.Flush(ctx, next.ID) }},
		{stepWaitBeforeLimit, "wait before limit update", func(ctx context.Context) error {
			return wait(ctx, s.cfg.LimitDelay)
		}},
		{stepAddLimit, "add previous game to limit", func(ctx context.Context) error {
			return s.automation.AddLimit(ctx, login, current.ID)
		}},
		{stepReloadAfterLimit, "reload login services after limit", func(ctx context.Context) error {
			return s.automation.ReloadLogins(ctx, login)
		}},
		{stepCommit, "commit current game state", func(context.Context) error {
			return s.state.Set(next.ID)
		}},
	}

	start := -1
	for index, step := range steps {
		if step.key == pending.NextStep {
			start = index
			break
		}
	}
	if start == -1 {
		return false, fmt.Errorf("pending open contains unknown step %q", pending.NextStep)
	}

	for index := start; index < len(steps); index++ {
		step := steps[index]
		if err := s.runStep(ctx, step.name, step.run); err != nil {
			return false, err
		}
		if index+1 < len(steps) {
			pending.NextStep = steps[index+1].key
			if err := s.state.SetPending(pending); err != nil {
				return false, err
			}
		}
	}

	if err := s.state.ClearPending(); err != nil {
		return false, err
	}
	s.logger.Info("game opened", "game", next.ID, "host", next.Host)
	return true, nil
}

func (s *Service) runStep(ctx context.Context, name string, operation func(context.Context) error) error {
	const attempts = 4
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		s.logger.Info("running open step", "step", name, "attempt", attempt, "attempts", attempts)
		if err := operation(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < attempts {
			if err := wait(ctx, time.Duration(attempt*5)*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("%s failed after %d attempts: %w", name, attempts, lastErr)
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
