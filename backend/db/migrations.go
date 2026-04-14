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
		`CREATE TABLE IF NOT EXISTS run_events (
			id         SERIAL PRIMARY KEY,
			run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			seq        INTEGER NOT NULL,
			event_type VARCHAR(20) NOT NULL,
			data       TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_run_events_run_id ON run_events(run_id)`,
		`CREATE TABLE IF NOT EXISTS task_cache (
			key         VARCHAR(32) PRIMARY KEY,
			project     VARCHAR(32) NOT NULL,
			issue_type  VARCHAR(64) NOT NULL,
			summary     TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status      VARCHAR(64) NOT NULL DEFAULT '',
			cached_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_cache_project ON task_cache(project)`,
		`CREATE TABLE IF NOT EXISTS resolved_commits (
			id         SERIAL PRIMARY KEY,
			issue_key  VARCHAR(32) NOT NULL,
			commit_sha VARCHAR(64) NOT NULL,
			resolved_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(issue_key, commit_sha)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_resolved_commits_issue ON resolved_commits(issue_key)`,
		`CREATE TABLE IF NOT EXISTS user_vacations (
			id         SERIAL PRIMARY KEY,
			user_login VARCHAR(128) NOT NULL,
			date_from  DATE NOT NULL,
			date_to    DATE NOT NULL,
			comment    TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_vacations_login ON user_vacations(user_login)`,
	}

	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}
