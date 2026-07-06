package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func initTimescale() {
	dsn := env("POSTGRES_DSN", "postgres://geo:geo_dev_pw@localhost:5432/geo")
	var err error
	pool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Printf("timescale: connect failed (%v) — persistence disabled", err)
		pool = nil
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS surge_windows (
			ts          TIMESTAMPTZ NOT NULL,
			zone        TEXT        NOT NULL,
			demand      INT         NOT NULL,
			supply      INT         NOT NULL,
			multiplier  DOUBLE PRECISION NOT NULL
		)`,
		`SELECT create_hypertable('surge_windows', 'ts', if_not_exists => TRUE)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			log.Printf("timescale: setup warn: %v", err)
		}
	}
	log.Printf("timescale: surge_windows hypertable ready")
}

func persist(ctx context.Context, ts time.Time, zone string, demand, supply int, m float64) {
	if pool == nil {
		return
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO surge_windows (ts, zone, demand, supply, multiplier) VALUES ($1,$2,$3,$4,$5)`,
		ts, zone, demand, supply, m)
	if err != nil {
		log.Printf("timescale insert: %v", err)
	}
}
