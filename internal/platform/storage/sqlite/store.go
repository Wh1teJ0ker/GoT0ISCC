package sqlite

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domain "got0iscc/desktop/internal/domain/accounts"
	logdomain "got0iscc/desktop/internal/domain/logs"
	theorydomain "got0iscc/desktop/internal/domain/theory"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db     *sql.DB
	dbPath string
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, dbPath: dbPath}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  username TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  submit_priority INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_accounts_enabled ON accounts(enabled);
CREATE INDEX IF NOT EXISTS idx_accounts_priority ON accounts(submit_priority, id);

CREATE TABLE IF NOT EXISTS account_runtime (
  account_name TEXT PRIMARY KEY,
  cycle_status TEXT NOT NULL DEFAULT '',
  login_status TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  last_login_at TEXT NOT NULL DEFAULT '',
  last_cycle_started_at TEXT NOT NULL DEFAULT '',
  last_cycle_finished_at TEXT NOT NULL DEFAULT '',
  processed_challenges INTEGER NOT NULL DEFAULT 0,
  processed_sections INTEGER NOT NULL DEFAULT 0,
  remote_submission_count INTEGER NOT NULL DEFAULT 0,
  last_remote_submissions_sync_at TEXT NOT NULL DEFAULT '',
  session_token_file TEXT NOT NULL DEFAULT '',
  session_token_exists INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT '',
  raw_json TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  source_type TEXT NOT NULL DEFAULT '',
  command_text TEXT NOT NULL DEFAULT '',
  log_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  account TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  pid INTEGER,
  returncode INTEGER,
  tail_text TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_account ON jobs(account, started_at DESC);

CREATE TABLE IF NOT EXISTS challenges (
  challenge_key TEXT PRIMARY KEY,
  challenge_id TEXT NOT NULL,
  section_name TEXT NOT NULL,
  title TEXT NOT NULL,
  category TEXT NOT NULL,
  challenge_kind TEXT NOT NULL,
  expects_attachments INTEGER NOT NULL DEFAULT 0,
  expects_remote INTEGER NOT NULL DEFAULT 0,
  asset_warnings_json TEXT NOT NULL DEFAULT '[]',
  dir_name TEXT NOT NULL DEFAULT '',
  dir_path TEXT NOT NULL DEFAULT '',
  detail_url TEXT NOT NULL DEFAULT '',
  description_path TEXT NOT NULL DEFAULT '',
  description_sha256 TEXT NOT NULL DEFAULT '',
  remote_summary_path TEXT NOT NULL DEFAULT '',
  remote_sha256 TEXT NOT NULL DEFAULT '',
  remote_targets_json TEXT NOT NULL DEFAULT '[]',
  fingerprint TEXT NOT NULL DEFAULT '',
  changed INTEGER NOT NULL DEFAULT 0,
  attachments_json TEXT NOT NULL DEFAULT '[]',
  updated_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_challenges_section ON challenges(section_name, challenge_id);
CREATE INDEX IF NOT EXISTS idx_challenges_updated_at ON challenges(updated_at DESC);

CREATE TABLE IF NOT EXISTS challenge_accounts (
  challenge_key TEXT NOT NULL,
  account_name TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (challenge_key, account_name)
);

CREATE INDEX IF NOT EXISTS idx_challenge_accounts_account ON challenge_accounts(account_name, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_challenge_accounts_key ON challenge_accounts(challenge_key, account_name);

CREATE TABLE IF NOT EXISTS service_logs (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  source_type TEXT NOT NULL DEFAULT '',
  file_path TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  modified_at TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS theory_bank_questions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  question_hash TEXT NOT NULL UNIQUE,
  question TEXT NOT NULL,
  normalized_question TEXT NOT NULL DEFAULT '',
  compact_question TEXT NOT NULL DEFAULT '',
  selection_type TEXT NOT NULL DEFAULT 'single',
  source_kind TEXT NOT NULL DEFAULT '',
  source_ref TEXT NOT NULL DEFAULT '',
  options_json TEXT NOT NULL DEFAULT '[]',
  answer_keys_json TEXT NOT NULL DEFAULT '[]',
  answer_texts_json TEXT NOT NULL DEFAULT '[]',
  keywords_json TEXT NOT NULL DEFAULT '[]',
  search_text TEXT NOT NULL DEFAULT '',
  confidence REAL NOT NULL DEFAULT 0,
  needs_review INTEGER NOT NULL DEFAULT 0,
  review_status TEXT NOT NULL DEFAULT 'pending',
  review_reason TEXT NOT NULL DEFAULT '',
  duplicate_group TEXT NOT NULL DEFAULT '',
  raw_payload_json TEXT NOT NULL DEFAULT '{}',
  captured_count INTEGER NOT NULL DEFAULT 0,
  last_captured_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_theory_bank_review ON theory_bank_questions(review_status, needs_review, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_theory_bank_source ON theory_bank_questions(source_kind, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_theory_bank_question_hash ON theory_bank_questions(question_hash);

CREATE TABLE IF NOT EXISTS theory_captured_questions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  question_hash TEXT NOT NULL,
  question TEXT NOT NULL,
  normalized_question TEXT NOT NULL DEFAULT '',
  selection_type TEXT NOT NULL DEFAULT 'single',
  options_json TEXT NOT NULL DEFAULT '[]',
  source_url TEXT NOT NULL DEFAULT '',
  account TEXT NOT NULL DEFAULT '',
  matched_question_id INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_theory_captured_hash ON theory_captured_questions(question_hash, created_at DESC);

CREATE TABLE IF NOT EXISTS theory_runtime_snapshot (
  account_name TEXT PRIMARY KEY,
  username TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  question_title TEXT NOT NULL DEFAULT '',
  question_number INTEGER NOT NULL DEFAULT 0,
  cached_at TEXT NOT NULL DEFAULT '',
  last_remote_sync_at TEXT NOT NULL DEFAULT '',
  last_remote_error TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
`)
	if err != nil {
		return err
	}
	return s.cleanupLegacyCompatibility(ctx)
}

func (s *Store) cleanupLegacyCompatibility(ctx context.Context) error {
	if err := s.deleteLegacyMeta(ctx); err != nil {
		return err
	}
	if err := s.rebuildJobsTableWithoutLegacyColumn(ctx); err != nil {
		return err
	}
	return s.rebuildAccountsTableWithoutLegacyProxyColumns(ctx)
}

func (s *Store) deleteLegacyMeta(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
DELETE FROM meta
WHERE key IN (
  'accounts.last_import_source',
  'accounts.last_imported_at',
  'accounts.runtime_last_import_source',
  'accounts.runtime_last_imported_at',
  'logs.last_import_source',
  'logs.last_imported_at',
  'logs.service_last_import_source'
)
`)
	return err
}

func (s *Store) rebuildJobsTableWithoutLegacyColumn(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info(jobs)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasLegacyJobID := false
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == "legacy_job_id" {
			hasLegacyJobID = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !hasLegacyJobID {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	statements := []string{
		"DROP INDEX IF EXISTS idx_jobs_status",
		"DROP INDEX IF EXISTS idx_jobs_account",
		"ALTER TABLE jobs RENAME TO jobs_legacy_backup",
		`CREATE TABLE jobs (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  source_type TEXT NOT NULL DEFAULT '',
  command_text TEXT NOT NULL DEFAULT '',
  log_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  account TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL DEFAULT '',
  finished_at TEXT NOT NULL DEFAULT '',
  pid INTEGER,
  returncode INTEGER,
  tail_text TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
)`,
		`INSERT INTO jobs (
  id, title, source, source_type, command_text, log_path, status, account,
  started_at, finished_at, pid, returncode, tail_text, updated_at
)
SELECT
  id, title, source, source_type, command_text, log_path, status, account,
  started_at, finished_at, pid, returncode, tail_text, updated_at
FROM jobs_legacy_backup`,
		"DROP TABLE jobs_legacy_backup",
		"CREATE INDEX idx_jobs_status ON jobs(status, started_at DESC)",
		"CREATE INDEX idx_jobs_account ON jobs(account, started_at DESC)",
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) rebuildAccountsTableWithoutLegacyProxyColumns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info(accounts)")
	if err != nil {
		return err
	}
	defer rows.Close()

	legacyColumns := map[string]bool{
		"proxy_type":           true,
		"proxy_host":           true,
		"proxy_port":           true,
		"proxy_username":       true,
		"proxy_password":       true,
		"proxy_script_path":    true,
		"proxy_config_path":    true,
		"proxy_script_args":    true,
		"proxy_script_timeout": true,
	}
	hasLegacyProxyColumns := false
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if legacyColumns[name] {
			hasLegacyProxyColumns = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !hasLegacyProxyColumns {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	statements := []string{
		"DROP INDEX IF EXISTS idx_accounts_enabled",
		"DROP INDEX IF EXISTS idx_accounts_priority",
		"ALTER TABLE accounts RENAME TO accounts_legacy_proxy_backup",
		`CREATE TABLE accounts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  username TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  submit_priority INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
)`,
		`INSERT INTO accounts (
  id, name, username, password, enabled, submit_priority, created_at, updated_at
)
SELECT
  id, name, username, password, enabled, submit_priority, created_at, updated_at
FROM accounts_legacy_proxy_backup`,
		"DROP TABLE accounts_legacy_proxy_backup",
		"CREATE INDEX idx_accounts_enabled ON accounts(enabled)",
		"CREATE INDEX idx_accounts_priority ON accounts(submit_priority, id)",
	}
	for _, statement := range statements {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.name, a.username, a.password, a.enabled, a.submit_priority,
       a.created_at, a.updated_at,
       r.account_name, r.cycle_status, r.login_status, r.last_error, r.last_login_at,
       r.last_cycle_started_at, r.last_cycle_finished_at, r.processed_challenges,
       r.processed_sections, r.remote_submission_count, r.last_remote_submissions_sync_at,
       r.session_token_file, r.session_token_exists, r.source, r.updated_at, r.raw_json
FROM accounts a
LEFT JOIN account_runtime r ON r.account_name = a.name
ORDER BY submit_priority ASC, id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := []domain.Account{}
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (s *Store) SaveAccount(ctx context.Context, account domain.Account) (domain.Account, error) {
	now := nowTs()
	if account.ID > 0 {
		var existingName string
		if err := s.db.QueryRowContext(ctx, "SELECT name FROM accounts WHERE id = ?", account.ID).Scan(&existingName); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Account{}, errors.New("账号不存在")
			}
			return domain.Account{}, err
		}
		result, err := s.db.ExecContext(ctx, `
UPDATE accounts SET
  name = ?, username = ?, password = ?, enabled = ?, submit_priority = ?,
  updated_at = ?
WHERE id = ?
`, account.Name, account.Username, account.Password, boolInt(account.Enabled), account.SubmitPriority,
			now, account.ID)
		if err != nil {
			return domain.Account{}, normalizeSQLiteError(err)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return domain.Account{}, errors.New("账号不存在")
		}
		if existingName != account.Name {
			if _, err := s.db.ExecContext(ctx, `
UPDATE account_runtime
SET account_name = ?, updated_at = ?
WHERE account_name = ?
`, account.Name, now, existingName); err != nil {
				return domain.Account{}, err
			}
		}
		return s.accountByID(ctx, account.ID)
	}

	result, err := s.db.ExecContext(ctx, `
INSERT INTO accounts (
  name, username, password, enabled, submit_priority, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
`, account.Name, account.Username, account.Password, boolInt(account.Enabled), account.SubmitPriority,
		now, now)
	if err != nil {
		return domain.Account{}, normalizeSQLiteError(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.Account{}, err
	}
	return s.accountByID(ctx, id)
}

func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	var name string
	if err := s.db.QueryRowContext(ctx, "SELECT name FROM accounts WHERE id = ?", id).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("账号不存在")
		}
		return err
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errors.New("账号不存在")
	}
	_, _ = s.db.ExecContext(ctx, "DELETE FROM account_runtime WHERE account_name = ?", name)
	return nil
}

func (s *Store) AccountSummary(ctx context.Context) (domain.Summary, error) {
	var summary domain.Summary
	err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS total,
  COALESCE(SUM(CASE WHEN a.enabled = 1 THEN 1 ELSE 0 END), 0) AS enabled,
  COALESCE(SUM(CASE WHEN a.enabled = 0 THEN 1 ELSE 0 END), 0) AS disabled,
  COALESCE(SUM(CASE WHEN a.password = '' THEN 1 ELSE 0 END), 0) AS missing_password,
  COALESCE(SUM(CASE WHEN r.account_name IS NOT NULL THEN 1 ELSE 0 END), 0) AS runtime_available,
  COALESCE(SUM(CASE WHEN r.login_status = 'ok' THEN 1 ELSE 0 END), 0) AS login_ok,
  COALESCE(SUM(CASE WHEN r.session_token_exists = 1 THEN 1 ELSE 0 END), 0) AS session_ready,
  COALESCE(SUM(CASE WHEN r.cycle_status = 'running' THEN 1 ELSE 0 END), 0) AS running_cycles
FROM accounts a
LEFT JOIN account_runtime r ON r.account_name = a.name
`).Scan(
		&summary.Total,
		&summary.Enabled,
		&summary.Disabled,
		&summary.MissingPassword,
		&summary.RuntimeAvailable,
		&summary.LoginOK,
		&summary.SessionReady,
		&summary.RunningCycles,
	)
	if err != nil {
		return domain.Summary{}, err
	}
	summary.DatabasePath = s.dbPath
	return summary, nil
}

func (s *Store) accountByID(ctx context.Context, id int64) (domain.Account, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT a.id, a.name, a.username, a.password, a.enabled, a.submit_priority,
       a.created_at, a.updated_at,
       r.account_name, r.cycle_status, r.login_status, r.last_error, r.last_login_at,
       r.last_cycle_started_at, r.last_cycle_finished_at, r.processed_challenges,
       r.processed_sections, r.remote_submission_count, r.last_remote_submissions_sync_at,
       r.session_token_file, r.session_token_exists, r.source, r.updated_at, r.raw_json
FROM accounts a
LEFT JOIN account_runtime r ON r.account_name = a.name
WHERE a.id = ?
`, id)
	return scanAccount(row)
}

func (s *Store) MetaValue(ctx context.Context, key string) (string, error) {
	return s.metaValue(ctx, key)
}

func (s *Store) SetMetaValue(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO meta(key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, key, value, nowTs())
	return err
}

func (s *Store) SaveTheoryRuntimeSnapshot(ctx context.Context, account string, username string, payload theorydomain.Payload, cache theorydomain.CacheStatus) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO theory_runtime_snapshot (
  account_name, username, payload_json, question_title, question_number,
  cached_at, last_remote_sync_at, last_remote_error, source, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account_name) DO UPDATE SET
  username = excluded.username,
  payload_json = excluded.payload_json,
  question_title = excluded.question_title,
  question_number = excluded.question_number,
  cached_at = excluded.cached_at,
  last_remote_sync_at = excluded.last_remote_sync_at,
  last_remote_error = excluded.last_remote_error,
  source = excluded.source,
  updated_at = excluded.updated_at
`, account, username, string(data), payload.Question.Title, payload.Question.Number,
		cache.CachedAt, cache.LastRemoteSyncAt, cache.LastRemoteError, cache.Source, nowTs())
	return err
}

func (s *Store) LoadTheoryRuntimeSnapshot(ctx context.Context, account string) (theorydomain.Payload, theorydomain.CacheStatus, error) {
	var raw string
	var cachedAt string
	var lastRemoteSyncAt string
	var lastRemoteError string
	var source string
	err := s.db.QueryRowContext(ctx, `
SELECT payload_json, cached_at, last_remote_sync_at, last_remote_error, source
FROM theory_runtime_snapshot
WHERE account_name = ?
`, account).Scan(&raw, &cachedAt, &lastRemoteSyncAt, &lastRemoteError, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return theorydomain.Payload{}, theorydomain.CacheStatus{}, nil
	}
	if err != nil {
		return theorydomain.Payload{}, theorydomain.CacheStatus{}, err
	}
	var payload theorydomain.Payload
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return theorydomain.Payload{}, theorydomain.CacheStatus{}, err
		}
	}
	return payload, theorydomain.CacheStatus{
		HasSnapshot:      strings.TrimSpace(raw) != "",
		HasQuestion:      strings.TrimSpace(payload.Question.Title) != "",
		Answerable:       payload.Statistics.Answerable || (strings.TrimSpace(payload.Question.Title) != "" && len(payload.Question.Options) > 0 && strings.TrimSpace(payload.AnswerForm.Nonce) != "" && strings.TrimSpace(payload.AnswerForm.NumberValue) != ""),
		Completed:        payload.Statistics.Completed,
		CachedAt:         cachedAt,
		LastRemoteSyncAt: lastRemoteSyncAt,
		LastRemoteError:  lastRemoteError,
		Source:           source,
		Message:          payload.Statistics.ProgressMessage,
	}, nil
}

func (s *Store) ListTheoryCapturedQuestions(ctx context.Context, account string, limit int) ([]theorydomain.CapturedQuestionRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
SELECT id, account, question, selection_type, options_json, question_hash, matched_question_id, source_url, created_at
FROM theory_captured_questions
`
	args := make([]any, 0, 2)
	if strings.TrimSpace(account) != "" {
		query += `WHERE account = ? `
		args = append(args, account)
	}
	query += `ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]theorydomain.CapturedQuestionRecord, 0, limit)
	for rows.Next() {
		var item theorydomain.CapturedQuestionRecord
		var optionsJSON string
		if err := rows.Scan(&item.ID, &item.Account, &item.Question, &item.SelectionType, &optionsJSON, &item.QuestionHash, &item.MatchedReviewID, &item.SourceURL, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(optionsJSON), &item.Options)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListTheoryRuntimeSnapshots(ctx context.Context) ([]theorydomain.RuntimeSnapshotRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT account_name, username, question_title, question_number, cached_at, last_remote_sync_at, last_remote_error, source
FROM theory_runtime_snapshot
ORDER BY cached_at DESC, account_name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]theorydomain.RuntimeSnapshotRecord, 0, 16)
	for rows.Next() {
		var item theorydomain.RuntimeSnapshotRecord
		if err := rows.Scan(&item.Account, &item.Username, &item.QuestionTitle, &item.QuestionNumber, &item.CachedAt, &item.LastRemoteSyncAt, &item.LastRemoteError, &item.Source); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) metaValue(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) ExecuteSeedSQL(ctx context.Context, script string) error {
	statements := splitSeedSQLStatements(script)
	for _, statement := range statements {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func splitSeedSQLStatements(script string) []string {
	scanner := bufio.NewScanner(strings.NewReader(script))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	statements := make([]string, 0, 32)
	var builder strings.Builder
	inSingleQuote := false

	flush := func() {
		statement := strings.TrimSpace(builder.String())
		if statement != "" {
			statements = append(statements, statement)
		}
		builder.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		for index := 0; index < len(line); index++ {
			char := line[index]
			if char == '\'' {
				if inSingleQuote && index+1 < len(line) && line[index+1] == '\'' {
					builder.WriteByte(char)
					builder.WriteByte(line[index+1])
					index++
					continue
				}
				inSingleQuote = !inSingleQuote
			}
			if char == ';' && !inSingleQuote {
				flush()
				continue
			}
			builder.WriteByte(char)
		}
		builder.WriteByte('\n')
	}
	flush()
	return statements
}

func (s *Store) ListJobs(ctx context.Context, limit int) ([]logdomain.Job, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, source, source_type, command_text, log_path, status, account,
       started_at, finished_at, pid, returncode, tail_text, updated_at
FROM jobs
ORDER BY started_at DESC, id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]logdomain.Job, 0, limit)
	for rows.Next() {
		var item logdomain.Job
		var pid sql.NullInt64
		var returnCode sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Source,
			&item.SourceType,
			&item.Command,
			&item.LogPath,
			&item.Status,
			&item.Account,
			&item.StartedAt,
			&item.FinishedAt,
			&pid,
			&returnCode,
			&item.Tail,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if pid.Valid {
			value := int(pid.Int64)
			item.PID = &value
		}
		if returnCode.Valid {
			value := int(returnCode.Int64)
			item.ReturnCode = &value
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UpsertJobs(ctx context.Context, jobs []logdomain.Job) error {
	_, err := s.saveJobs(ctx, jobs)
	return err
}

func (s *Store) saveJobs(ctx context.Context, jobs []logdomain.Job) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	changed := 0
	now := nowTs()
	for _, job := range jobs {
		if strings.TrimSpace(job.ID) == "" {
			continue
		}
		var existing string
		scanErr := tx.QueryRowContext(ctx, "SELECT id FROM jobs WHERE id = ?", job.ID).Scan(&existing)
		exists := scanErr == nil
		if scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
			err = scanErr
			return 0, err
		}

		if exists {
			_, err = tx.ExecContext(ctx, `
UPDATE jobs SET
  title = ?, source = ?, source_type = ?, command_text = ?, log_path = ?,
  status = ?, account = ?, started_at = ?, finished_at = ?, pid = ?, returncode = ?,
  tail_text = ?, updated_at = ?
WHERE id = ?
`, job.Title, job.Source, job.SourceType, job.Command, job.LogPath,
				job.Status, job.Account, job.StartedAt, job.FinishedAt, intPointerValue(job.PID), intPointerValue(job.ReturnCode),
				job.Tail, now, job.ID)
			if err != nil {
				return 0, err
			}
			changed++
			continue
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO jobs (
  id, title, source, source_type, command_text, log_path, status, account,
  started_at, finished_at, pid, returncode, tail_text, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, job.ID, job.Title, job.Source, job.SourceType, job.Command, job.LogPath, job.Status, job.Account,
			job.StartedAt, job.FinishedAt, intPointerValue(job.PID), intPointerValue(job.ReturnCode), job.Tail, now)
		if err != nil {
			return 0, err
		}
		changed++
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return changed, nil
}

func (s *Store) ListServiceLogs(ctx context.Context) ([]logdomain.ServiceLog, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, source, source_type, file_path, size_bytes, modified_at, description
FROM service_logs
ORDER BY source_type ASC, name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]logdomain.ServiceLog, 0)
	for rows.Next() {
		var item logdomain.ServiceLog
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Source,
			&item.SourceType,
			&item.FilePath,
			&item.SizeBytes,
			&item.ModifiedAt,
			&item.Description,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) JobsSummary(ctx context.Context) (logdomain.Summary, error) {
	var summary logdomain.Summary
	err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS total_jobs,
  COALESCE(SUM(CASE WHEN status IN ('running', 'starting', 'stopping') THEN 1 ELSE 0 END), 0) AS running_jobs,
  COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed_jobs,
  COALESCE(SUM(CASE WHEN status = 'finished' THEN 1 ELSE 0 END), 0) AS finished_jobs,
  COALESCE(SUM(CASE WHEN status IN ('stopped', 'cancelled') THEN 1 ELSE 0 END), 0) AS stopped_jobs
FROM jobs
`).Scan(&summary.TotalJobs, &summary.RunningJobs, &summary.FailedJobs, &summary.FinishedJobs, &summary.StoppedJobs)
	if err != nil {
		return logdomain.Summary{}, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM service_logs").Scan(&summary.ServiceLogCount); err != nil {
		return logdomain.Summary{}, err
	}
	return summary, nil
}

func (s *Store) JobByID(ctx context.Context, id string) (logdomain.Job, error) {
	var item logdomain.Job
	var pid sql.NullInt64
	var returnCode sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT id, title, source, source_type, command_text, log_path, status, account,
       started_at, finished_at, pid, returncode, tail_text, updated_at
FROM jobs
WHERE id = ?
`, id).Scan(
		&item.ID,
		&item.Title,
		&item.Source,
		&item.SourceType,
		&item.Command,
		&item.LogPath,
		&item.Status,
		&item.Account,
		&item.StartedAt,
		&item.FinishedAt,
		&pid,
		&returnCode,
		&item.Tail,
		&item.UpdatedAt,
	)
	if err != nil {
		return logdomain.Job{}, err
	}
	if pid.Valid {
		value := int(pid.Int64)
		item.PID = &value
	}
	if returnCode.Valid {
		value := int(returnCode.Int64)
		item.ReturnCode = &value
	}
	return item, nil
}

func (s *Store) ServiceLogByID(ctx context.Context, id string) (logdomain.ServiceLog, error) {
	var item logdomain.ServiceLog
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, source, source_type, file_path, size_bytes, modified_at, description
FROM service_logs
WHERE id = ?
`, id).Scan(
		&item.ID,
		&item.Name,
		&item.Source,
		&item.SourceType,
		&item.FilePath,
		&item.SizeBytes,
		&item.ModifiedAt,
		&item.Description,
	)
	return item, err
}

func (s *Store) ReplaceTheoryQuestions(ctx context.Context, items []theorydomain.ReviewItem, source string) (theorydomain.ReviewDashboard, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return theorydomain.ReviewDashboard{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, "DELETE FROM theory_bank_questions WHERE source_kind IN ('raw-json', 'normalized-json', 'standardized-json', 'docx')"); err != nil {
		return theorydomain.ReviewDashboard{}, err
	}

	now := nowTs()
	for _, item := range items {
		optionsJSON, _ := json.Marshal(item.Options)
		answerKeysJSON, _ := json.Marshal(item.AnswerKeys)
		answerTextsJSON, _ := json.Marshal(item.AnswerTexts)
		keywordsJSON, _ := json.Marshal([]string{})
		rawPayloadJSON, _ := json.Marshal(map[string]any{
			"source_kind": item.SourceKind,
			"source_ref":  item.SourceRef,
		})
		if _, err = tx.ExecContext(ctx, `
INSERT INTO theory_bank_questions (
  question_hash, question, normalized_question, compact_question, selection_type,
  source_kind, source_ref, options_json, answer_keys_json, answer_texts_json,
  keywords_json, search_text, confidence, needs_review, review_status, review_reason,
  duplicate_group, raw_payload_json, captured_count, last_captured_at, created_at, updated_at
) VALUES (?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, ?)
ON CONFLICT(question_hash) DO UPDATE SET
  question = excluded.question,
  normalized_question = excluded.normalized_question,
  selection_type = excluded.selection_type,
  source_kind = excluded.source_kind,
  source_ref = excluded.source_ref,
  options_json = excluded.options_json,
  answer_keys_json = excluded.answer_keys_json,
  answer_texts_json = excluded.answer_texts_json,
  keywords_json = excluded.keywords_json,
  search_text = excluded.search_text,
  confidence = excluded.confidence,
  needs_review = excluded.needs_review,
  review_status = excluded.review_status,
  review_reason = excluded.review_reason,
  duplicate_group = excluded.duplicate_group,
  raw_payload_json = excluded.raw_payload_json,
  updated_at = excluded.updated_at
`, item.QuestionHash, item.Question, item.NormalizedQuestion, item.SelectionType,
			item.SourceKind, item.SourceRef, string(optionsJSON), string(answerKeysJSON), string(answerTextsJSON),
			string(keywordsJSON), item.NormalizedQuestion, item.Confidence, boolInt(item.NeedsReview), item.ReviewStatus, item.ReviewReason,
			"", string(rawPayloadJSON), now, now); err != nil {
			return theorydomain.ReviewDashboard{}, err
		}
	}

	if _, err = tx.ExecContext(ctx, `
INSERT INTO meta(key, value, updated_at)
VALUES ('theory.bank_last_import_source', ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, source, now); err != nil {
		return theorydomain.ReviewDashboard{}, err
	}
	if _, err = tx.ExecContext(ctx, `
INSERT INTO meta(key, value, updated_at)
VALUES ('theory.bank_last_imported_at', ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`, now, now); err != nil {
		return theorydomain.ReviewDashboard{}, err
	}

	if err = tx.Commit(); err != nil {
		return theorydomain.ReviewDashboard{}, err
	}
	return s.TheoryReviewDashboard(ctx)
}

func (s *Store) TheoryReviewDashboard(ctx context.Context) (theorydomain.ReviewDashboard, error) {
	var summary theorydomain.ReviewDashboard
	err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS total_questions,
  COALESCE(SUM(CASE WHEN review_status = 'approved' THEN 1 ELSE 0 END), 0) AS reviewed_questions,
  COALESCE(SUM(CASE WHEN needs_review = 1 OR review_status IN ('pending', 'captured') THEN 1 ELSE 0 END), 0) AS pending_review,
  COALESCE(SUM(CASE WHEN captured_count > 0 THEN 1 ELSE 0 END), 0) AS captured_questions,
  COALESCE(SUM(captured_count), 0) AS capture_hits,
  COALESCE(MAX(last_captured_at), '') AS last_captured_at
FROM theory_bank_questions
	`).Scan(&summary.TotalQuestions, &summary.ReviewedQuestions, &summary.PendingReview, &summary.CapturedQuestions, &summary.CaptureHits, &summary.LastCapturedAt)
	if err != nil {
		return theorydomain.ReviewDashboard{}, err
	}
	summary.DatabasePath = s.dbPath
	return summary, nil
}

func (s *Store) ListTheoryReviewItems(ctx context.Context, limit int) (theorydomain.ReviewListResponse, error) {
	if limit <= 0 {
		limit = 80
	}
	summary, err := s.TheoryReviewDashboard(ctx)
	if err != nil {
		return theorydomain.ReviewListResponse{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, question, normalized_question, selection_type, source_kind, source_ref,
       options_json, answer_keys_json, answer_texts_json, needs_review, review_status,
       review_reason, confidence, question_hash, last_captured_at, created_at, updated_at
FROM theory_bank_questions
WHERE needs_review = 1 OR review_status IN ('pending', 'captured')
ORDER BY updated_at DESC, id DESC
LIMIT ?
	`, limit)
	if err != nil {
		return theorydomain.ReviewListResponse{}, err
	}
	defer rows.Close()

	items := make([]theorydomain.ReviewItem, 0, limit)
	for rows.Next() {
		item, scanErr := scanTheoryReviewItem(rows)
		if scanErr != nil {
			return theorydomain.ReviewListResponse{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return theorydomain.ReviewListResponse{}, err
	}
	return theorydomain.ReviewListResponse{Summary: summary, Items: items}, nil
}

func (s *Store) ListTheoryReviewItemsAll(ctx context.Context) ([]theorydomain.ReviewItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, question, normalized_question, selection_type, source_kind, source_ref,
       options_json, answer_keys_json, answer_texts_json, needs_review, review_status,
       review_reason, confidence, question_hash, last_captured_at, created_at, updated_at
FROM theory_bank_questions
ORDER BY updated_at DESC, id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]theorydomain.ReviewItem, 0, 256)
	for rows.Next() {
		item, scanErr := scanTheoryReviewItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListTheorySearchableItems(ctx context.Context) ([]theorydomain.ReviewItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, question, normalized_question, selection_type, source_kind, source_ref,
       options_json, answer_keys_json, answer_texts_json, needs_review, review_status,
       review_reason, confidence, question_hash, last_captured_at, created_at, updated_at
FROM theory_bank_questions
WHERE review_status = 'approved' AND (json_array_length(answer_keys_json) > 0 OR json_array_length(answer_texts_json) > 0)
ORDER BY updated_at DESC, id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]theorydomain.ReviewItem, 0, 256)
	for rows.Next() {
		item, scanErr := scanTheoryReviewItem(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CaptureTheoryQuestion(ctx context.Context, captured theorydomain.CapturedQuestion, sourceURL string, account string) (theorydomain.CapturedQuestion, error) {
	now := nowTs()
	optionsJSON, _ := json.Marshal(captured.Options)
	result, err := s.db.ExecContext(ctx, `
INSERT INTO theory_captured_questions (
  question_hash, question, normalized_question, selection_type, options_json, source_url, account, matched_question_id, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, captured.QuestionHash, captured.Question, captured.Question, captured.SelectionType, string(optionsJSON), sourceURL, account, captured.MatchedReviewID, now)
	if err != nil {
		return theorydomain.CapturedQuestion{}, err
	}
	_ = result

	if captured.MatchedReviewID > 0 {
		if _, err := s.db.ExecContext(ctx, `
UPDATE theory_bank_questions
SET captured_count = captured_count + 1, last_captured_at = ?, updated_at = ?
WHERE id = ?
`, now, now, captured.MatchedReviewID); err != nil {
			return theorydomain.CapturedQuestion{}, err
		}
	} else {
		optionsRaw, _ := json.Marshal(captured.Options)
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO theory_bank_questions (
  question_hash, question, normalized_question, compact_question, selection_type,
  source_kind, source_ref, options_json, answer_keys_json, answer_texts_json,
  keywords_json, search_text, confidence, needs_review, review_status, review_reason,
  duplicate_group, raw_payload_json, captured_count, last_captured_at, created_at, updated_at
) VALUES (?, ?, ?, '', ?, 'captured-live', ?, ?, '[]', '[]', '[]', ?, 0, 1, 'captured', '抓取到新题，等待人工复核', '', '{}', 1, ?, ?, ?)
ON CONFLICT(question_hash) DO UPDATE SET
  selection_type = excluded.selection_type,
  options_json = excluded.options_json,
  source_kind = excluded.source_kind,
  source_ref = excluded.source_ref,
  needs_review = 1,
  review_status = 'captured',
  review_reason = '抓取到新题，等待人工复核',
  captured_count = theory_bank_questions.captured_count + 1,
  last_captured_at = excluded.last_captured_at,
  updated_at = excluded.updated_at
`, captured.QuestionHash, captured.Question, captured.Question, captured.SelectionType, sourceURL, string(optionsRaw), captured.Question, now, now, now); err != nil {
			return theorydomain.CapturedQuestion{}, err
		}
	}

	captured.CapturedAt = now
	return captured, nil
}

func (s *Store) SaveTheoryReviewDecision(ctx context.Context, decision theorydomain.ReviewDecision) (theorydomain.ReviewItem, error) {
	optionsJSON, _ := json.Marshal(decision.Options)
	answerKeysJSON, _ := json.Marshal(decision.AnswerKeys)
	answerTextsJSON, _ := json.Marshal(decision.AnswerTexts)
	now := nowTs()
	if _, err := s.db.ExecContext(ctx, `
UPDATE theory_bank_questions
SET question = ?, selection_type = ?, options_json = ?, answer_keys_json = ?, answer_texts_json = ?,
    needs_review = CASE WHEN ? = 'approved' THEN 0 ELSE 1 END,
    review_status = ?, review_reason = ?, updated_at = ?
WHERE id = ?
`, decision.Question, decision.SelectionType, string(optionsJSON), string(answerKeysJSON), string(answerTextsJSON),
		decision.ReviewStatus, decision.ReviewStatus, decision.ReviewReason, now, decision.ID); err != nil {
		return theorydomain.ReviewItem{}, err
	}
	return s.TheoryReviewItemByID(ctx, decision.ID)
}

func (s *Store) TheoryReviewItemByID(ctx context.Context, id int64) (theorydomain.ReviewItem, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, question, normalized_question, selection_type, source_kind, source_ref,
       options_json, answer_keys_json, answer_texts_json, needs_review, review_status,
       review_reason, confidence, question_hash, last_captured_at, created_at, updated_at
FROM theory_bank_questions
WHERE id = ?
`, id)
	return scanTheoryReviewItem(row)
}

func (s *Store) TheoryReviewItemByHash(ctx context.Context, hash string) (theorydomain.ReviewItem, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, question, normalized_question, selection_type, source_kind, source_ref,
       options_json, answer_keys_json, answer_texts_json, needs_review, review_status,
       review_reason, confidence, question_hash, last_captured_at, created_at, updated_at
FROM theory_bank_questions
WHERE question_hash = ?
`, hash)
	return scanTheoryReviewItem(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAccount(row scanner) (domain.Account, error) {
	var account domain.Account
	var enabled int
	var runtimeAccountName sql.NullString
	var cycleStatus sql.NullString
	var loginStatus sql.NullString
	var lastError sql.NullString
	var lastLoginAt sql.NullString
	var lastCycleStartedAt sql.NullString
	var lastCycleFinishedAt sql.NullString
	var processedChallenges sql.NullInt64
	var processedSections sql.NullInt64
	var remoteSubmissionCount sql.NullInt64
	var lastRemoteSyncAt sql.NullString
	var sessionTokenFile sql.NullString
	var sessionTokenExists sql.NullInt64
	var runtimeSource sql.NullString
	var runtimeUpdatedAt sql.NullString
	var runtimeRawJSON sql.NullString
	err := row.Scan(
		&account.ID,
		&account.Name,
		&account.Username,
		&account.Password,
		&enabled,
		&account.SubmitPriority,
		&account.CreatedAt,
		&account.UpdatedAt,
		&runtimeAccountName,
		&cycleStatus,
		&loginStatus,
		&lastError,
		&lastLoginAt,
		&lastCycleStartedAt,
		&lastCycleFinishedAt,
		&processedChallenges,
		&processedSections,
		&remoteSubmissionCount,
		&lastRemoteSyncAt,
		&sessionTokenFile,
		&sessionTokenExists,
		&runtimeSource,
		&runtimeUpdatedAt,
		&runtimeRawJSON,
	)
	account.Enabled = enabled == 1
	if err == nil && runtimeAccountName.Valid {
		account.Runtime = &domain.RuntimeState{
			AccountName:               runtimeAccountName.String,
			CycleStatus:               cycleStatus.String,
			LoginStatus:               loginStatus.String,
			LastError:                 lastError.String,
			LastLoginAt:               lastLoginAt.String,
			LastCycleStartedAt:        lastCycleStartedAt.String,
			LastCycleFinishedAt:       lastCycleFinishedAt.String,
			ProcessedChallenges:       int(processedChallenges.Int64),
			ProcessedSections:         int(processedSections.Int64),
			RemoteSubmissionCount:     int(remoteSubmissionCount.Int64),
			LastRemoteSubmissionsSync: lastRemoteSyncAt.String,
			SessionTokenFile:          sessionTokenFile.String,
			SessionTokenExists:        sessionTokenExists.Int64 == 1,
			Source:                    runtimeSource.String,
			UpdatedAt:                 runtimeUpdatedAt.String,
			RawJSON:                   runtimeRawJSON.String,
		}
	}
	return account, err
}

func scanTheoryReviewItem(row scanner) (theorydomain.ReviewItem, error) {
	var item theorydomain.ReviewItem
	var optionsJSON string
	var answerKeysJSON string
	var answerTextsJSON string
	var needsReview int
	err := row.Scan(
		&item.ID,
		&item.Question,
		&item.NormalizedQuestion,
		&item.SelectionType,
		&item.SourceKind,
		&item.SourceRef,
		&optionsJSON,
		&answerKeysJSON,
		&answerTextsJSON,
		&needsReview,
		&item.ReviewStatus,
		&item.ReviewReason,
		&item.Confidence,
		&item.QuestionHash,
		&item.CapturedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return theorydomain.ReviewItem{}, err
	}
	item.NeedsReview = needsReview == 1
	_ = json.Unmarshal([]byte(optionsJSON), &item.Options)
	_ = json.Unmarshal([]byte(answerKeysJSON), &item.AnswerKeys)
	_ = json.Unmarshal([]byte(answerTextsJSON), &item.AnswerTexts)
	return item, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intPointerValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nowTs() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func normalizeSQLiteError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "constraint failed") || strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("账号名称已存在")
	}
	return err
}
