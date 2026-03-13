package db

import (
	"database/sql"
	"fmt"
)

func RunMigrations(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS config (
			id    SERIAL PRIMARY KEY,
			key   VARCHAR(64) UNIQUE NOT NULL,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id         SERIAL PRIMARY KEY,
			login      VARCHAR(128) UNIQUE NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id          SERIAL PRIMARY KEY,
			function    VARCHAR(64) NOT NULL,
			params      JSONB NOT NULL DEFAULT '{}',
			status      VARCHAR(16) NOT NULL DEFAULT 'running',
			error       TEXT,
			started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			finished_at TIMESTAMPTZ
		)`,
		`CREATE TABLE IF NOT EXISTS run_output (
			id         SERIAL PRIMARY KEY,
			run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			line_num   INTEGER NOT NULL,
			text       TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_run_output_run_id ON run_output(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_function ON runs(function)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_started_at ON runs(started_at DESC)`,
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}
