package greywallapi

import (
	"database/sql"
	"fmt"
)

var migrations = []string{
	// Migration 1: Create rules table
	`CREATE TABLE IF NOT EXISTS rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		container_pattern TEXT NOT NULL,
		destination_pattern TEXT NOT NULL,
		port_pattern TEXT NOT NULL DEFAULT '*',
		rule_type TEXT NOT NULL DEFAULT 'permanent' CHECK (rule_type IN ('permanent', 'temporary')),
		action TEXT NOT NULL DEFAULT 'allow' CHECK (action IN ('allow', 'deny')),
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		expires_at DATETIME,
		last_used_at DATETIME,
		created_by TEXT NOT NULL DEFAULT 'admin',
		notes TEXT,
		UNIQUE(container_pattern, destination_pattern, port_pattern, action)
	);
	CREATE INDEX IF NOT EXISTS idx_rules_container ON rules(container_pattern);
	CREATE INDEX IF NOT EXISTS idx_rules_destination ON rules(destination_pattern);
	CREATE INDEX IF NOT EXISTS idx_rules_expires ON rules(expires_at);`,

	// Migration 2: Create pending_requests table
	`CREATE TABLE IF NOT EXISTS pending_requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		container_name TEXT NOT NULL,
		container_id TEXT NOT NULL DEFAULT '',
		destination_host TEXT NOT NULL,
		destination_port INTEGER NOT NULL,
		resolved_hostname TEXT,
		first_seen DATETIME NOT NULL DEFAULT (datetime('now')),
		last_seen DATETIME NOT NULL DEFAULT (datetime('now')),
		attempt_count INTEGER NOT NULL DEFAULT 1,
		UNIQUE(container_name, destination_host, destination_port)
	);
	CREATE INDEX IF NOT EXISTS idx_pending_container ON pending_requests(container_name);
	CREATE INDEX IF NOT EXISTS idx_pending_last_seen ON pending_requests(last_seen);`,

	// Migration 3: Create request_logs table
	`CREATE TABLE IF NOT EXISTS request_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT (datetime('now')),
		container_name TEXT NOT NULL,
		container_id TEXT,
		destination_host TEXT NOT NULL,
		destination_port INTEGER,
		resolved_hostname TEXT,
		method TEXT,
		result TEXT NOT NULL CHECK (result IN ('allowed', 'blocked')),
		rule_id INTEGER REFERENCES rules(id) ON DELETE SET NULL,
		response_time_ms INTEGER
	);
	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON request_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_container ON request_logs(container_name);
	CREATE INDEX IF NOT EXISTS idx_logs_destination ON request_logs(destination_host);
	CREATE INDEX IF NOT EXISTS idx_logs_result ON request_logs(result);`,
}

func runMigrations(db *sql.DB) error {
	// Create migrations tracking table
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	for i, m := range migrations {
		version := i + 1
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 {
			continue
		}

		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("run migration %d: %w", version, err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return fmt.Errorf("record migration %d: %w", version, err)
		}
	}
	return nil
}
