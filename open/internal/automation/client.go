package automation

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/apenella/go-ansible/v2/pkg/playbook"

	"github.com/zzwsec/finger/open/internal/config"
	"github.com/zzwsec/finger/open/internal/topology"
)

type Client struct {
	root   string
	logger *slog.Logger
}

func New(root string, logger *slog.Logger) *Client {
	return &Client{root: root, logger: logger}
}

func (c *Client) Package(ctx context.Context, game topology.Game) error {
	return c.run(ctx, "package.yml", []string{game.Host}, map[string]any{
		"area_id": game.ID,
	})
}

func (c *Client) Install(ctx context.Context, game topology.Game, cfg config.Install) error {
	discovers := make([]map[string]any, 0, len(cfg.Discoveries))
	for _, endpoint := range cfg.Discoveries {
		discovers = append(discovers, map[string]any{
			"zkip":   endpoint.Host,
			"zkport": endpoint.Port,
		})
	}

	return c.run(ctx, "install.yml", []string{game.Host}, map[string]any{
		"area_id":        game.ID,
		"current_ip":     game.Host,
		"domain":         cfg.Domain,
		"game_port":      cfg.BasePort + game.Index*1000,
		"thread":         cfg.Threads,
		"pay_notify_url": cfg.PayNotifyURL,
		"discovers":      discovers,
		"group_id":       cfg.GroupID,
		"game_index_num": cfg.GameIndexCount,
		"app_binary":     "p8_app_server",
		"game_db": map[string]any{
			"db_host":     cfg.GameDatabase.Host,
			"db_user":     cfg.GameDatabase.User,
			"db_password": cfg.GameDatabase.Password,
			"db_name":     fmt.Sprintf("%s%d", cfg.GameDatabase.Name, game.ID),
		},
	})
}

func (c *Client) RemoveWhitelist(ctx context.Context, hosts []string, gameID int) error {
	return c.run(ctx, "whitelist.yml", hosts, map[string]any{
		"area_id": gameID,
	})
}

func (c *Client) AddLimit(ctx context.Context, hosts []string, gameID int) error {
	return c.run(ctx, "limit.yml", hosts, map[string]any{
		"area_id": gameID,
	})
}

func (c *Client) ReloadLogins(ctx context.Context, hosts []string) error {
	return c.run(ctx, "reload-login.yml", hosts, nil)
}

func (c *Client) run(ctx context.Context, name string, hosts []string, extraVars map[string]any) error {
	if len(hosts) == 0 {
		return fmt.Errorf("run %s: inventory is empty", name)
	}
	inventory := strings.Join(hosts, ",") + ","
	if extraVars == nil {
		extraVars = make(map[string]any)
	}
	extraVars["target_hosts"] = "all"

	path := filepath.Join(c.root, "playbooks", name)
	c.logger.Info("running ansible playbook", "playbook", name, "hosts", hosts)
	options := &playbook.AnsiblePlaybookOptions{
		Inventory: inventory,
		ExtraVars: extraVars,
	}
	if err := playbook.NewAnsiblePlaybookExecute(path).
		WithPlaybookOptions(options).
		Execute(ctx); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}
