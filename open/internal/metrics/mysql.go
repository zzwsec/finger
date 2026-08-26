package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"github.com/zzwsec/finger/open/internal/config"
)

const (
	registerCountQuery = `SELECT COUNT(1) FROM log_register WHERE zone_id = ?`
	rechargeCountQuery = `
		SELECT COUNT(DISTINCT player_id)
		FROM (
			SELECT player_id
			FROM log_recharge
			WHERE zone_id = ?
			GROUP BY player_id
			HAVING SUM(money) >= ?
		) AS qualified_players`
)

type Counts struct {
	Registered int
	Recharged  int
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, cfg config.Database) (*Store, error) {
	driverConfig := mysql.Config{
		User:                 cfg.User,
		Passwd:               cfg.Password,
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)),
		DBName:               cfg.Name,
		AllowNativePasswords: true,
		Timeout:              5 * time.Second,
		ReadTimeout:          10 * time.Second,
		WriteTimeout:         10 * time.Second,
	}
	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open log database: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to log database: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Counts(ctx context.Context, gameID, minimumMoney int) (Counts, error) {
	var counts Counts
	if err := s.db.QueryRowContext(ctx, registerCountQuery, gameID).Scan(&counts.Registered); err != nil {
		return Counts{}, fmt.Errorf("query registrations for game%d: %w", gameID, err)
	}
	if err := s.db.QueryRowContext(ctx, rechargeCountQuery, gameID, minimumMoney).Scan(&counts.Recharged); err != nil {
		return Counts{}, fmt.Errorf("query recharges for game%d: %w", gameID, err)
	}
	return counts, nil
}
