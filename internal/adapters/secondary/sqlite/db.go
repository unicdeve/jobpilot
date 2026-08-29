package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func NewDB(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database at %s: %w", dbPath, err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &DB{conn: db}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Conn() *sql.DB {
	return db.conn
}

// InitializeSchema creates the tables defined in the master specification.
func (db *DB) InitializeSchema() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS candidates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			contact TEXT,
			location TEXT,
			summary TEXT,
			work_preferences TEXT,
			salary_preferences TEXT,
			work_authorization TEXT,
			application_preferences TEXT,
			skills TEXT,
			experiences TEXT,
			education TEXT,
			certifications TEXT,
			links TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS resumes (
			id TEXT PRIMARY KEY,
			candidate_id TEXT NOT NULL,
			name TEXT NOT NULL,
			original_file_path TEXT,
			markdown_content TEXT,
			parsed_metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (candidate_id) REFERENCES candidates(id)
		);`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			canonical_url TEXT UNIQUE,
			source_id TEXT,
			source_job_id TEXT,
			title TEXT NOT NULL,
			company_name TEXT,
			company_url TEXT,
			description_markdown TEXT,
			locations TEXT,
			remote_type TEXT,
			employment_type TEXT,
			salary_min REAL,
			salary_max REAL,
			salary_currency TEXT,
			salary_period TEXT,
			experience_min INTEGER,
			experience_max INTEGER,
			skills TEXT,
			posted_at DATETIME,
			expires_at DATETIME,
			discovered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			raw_source_reference TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS job_matches (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			candidate_id TEXT NOT NULL,
			score REAL,
			confidence REAL,
			strengths TEXT,
			gaps TEXT,
			concerns TEXT,
			matched_skills TEXT,
			missing_skills TEXT,
			rationale TEXT,
			model_provider TEXT,
			model_name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (job_id) REFERENCES jobs(id),
			FOREIGN KEY (candidate_id) REFERENCES candidates(id)
		);`,
		`CREATE TABLE IF NOT EXISTS applications (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			candidate_id TEXT NOT NULL,
			resume_id TEXT,
			mode TEXT,
			state TEXT,
			cover_letter TEXT,
			questions TEXT,
			policy_result TEXT,
			approval_status TEXT,
			submitted_at DATETIME,
			failure_reason TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (job_id) REFERENCES jobs(id),
			FOREIGN KEY (candidate_id) REFERENCES candidates(id),
			FOREIGN KEY (resume_id) REFERENCES resumes(id)
		);`,
		`CREATE TABLE IF NOT EXISTS application_questions (
			id TEXT PRIMARY KEY,
			application_id TEXT NOT NULL,
			label TEXT NOT NULL,
			type TEXT,
			required BOOLEAN,
			answer TEXT,
			answer_source TEXT,
			confidence REAL,
			user_edited BOOLEAN,
			status TEXT,
			FOREIGN KEY (application_id) REFERENCES applications(id)
		);`,
		`CREATE TABLE IF NOT EXISTS agent_runs (
			id TEXT PRIMARY KEY,
			task_type TEXT,
			status TEXT,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			error TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS agent_events (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			action TEXT,
			tool TEXT,
			status TEXT,
			input_summary TEXT,
			output_summary TEXT,
			policy_result TEXT,
			duration_ms INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (run_id) REFERENCES agent_runs(id)
		);`,
		`CREATE TABLE IF NOT EXISTS user_settings (
			key TEXT PRIMARY KEY,
			value TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS policies (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			rule TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, schema := range schemas {
		if _, err := db.conn.Exec(schema); err != nil {
			return fmt.Errorf("failed to execute schema creation: %s: %w", schema, err)
		}
	}

	// Migration: Ensure candidates table has application_preferences
	_, _ = db.conn.Exec(`ALTER TABLE candidates ADD COLUMN application_preferences TEXT;`)

	return nil
}
