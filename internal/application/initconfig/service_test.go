package initconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	runtimeplatform "got0iscc/desktop/internal/platform/runtime"
	sqlitestore "got0iscc/desktop/internal/platform/storage/sqlite"
)

func TestEnsureInitializedSeedsMainDatabase(t *testing.T) {
	layout, err := runtimeplatform.DetectLayout()
	if err != nil {
		t.Fatalf("detect layout: %v", err)
	}

	tempDir := t.TempDir()
	layout.AppDatabasePath = filepath.Join(tempDir, "got0iscc.db")
	layout.TheoryBankDBPath = layout.AppDatabasePath
	layout.AppDataRoot = tempDir
	layout.AppRuntimeRoot = filepath.Join(tempDir, "runtime")
	layout.InitSeedSQLPath = filepath.Join(tempDir, "got0iscc.init.sql")
	if err := os.WriteFile(layout.InitSeedSQLPath, []byte(`
INSERT INTO theory_bank_questions (
  question_hash,
  question,
  normalized_question,
  compact_question,
  selection_type,
  source_kind,
  source_ref,
  options_json,
  answer_keys_json,
  answer_texts_json,
  keywords_json,
  search_text,
  confidence,
  needs_review,
  review_status,
  review_reason,
  duplicate_group,
  raw_payload_json,
  captured_count,
  last_captured_at,
  created_at,
  updated_at
) VALUES (
  'test-hash',
  'question',
  'question',
  'question',
  'single',
  'test',
  'unit',
  '[]',
  '[]',
  '[]',
  '[]',
  'question',
  1,
  0,
  'approved',
  '',
  '',
  '{}',
  0,
  '',
  '2026-01-01T00:00:00Z',
  '2026-01-01T00:00:00Z'
);
`), 0o644); err != nil {
		t.Fatalf("write seed sql: %v", err)
	}

	store, err := sqlitestore.Open(layout.AppDatabasePath)
	if err != nil {
		t.Fatalf("open main store: %v", err)
	}
	defer store.Close()

	service := NewService(layout, store)
	if err := service.EnsureInitialized(context.Background()); err != nil {
		t.Fatalf("ensure initialized: %v", err)
	}

	applied, err := store.MetaValue(context.Background(), metaInitApplied)
	if err != nil {
		t.Fatalf("read init meta: %v", err)
	}
	if applied != "1" {
		t.Fatalf("expected init applied flag to be 1, got %q", applied)
	}

	var questionCount int
	if err := store.DB().QueryRowContext(context.Background(), "SELECT COUNT(*) FROM theory_bank_questions").Scan(&questionCount); err != nil {
		t.Fatalf("count seeded theory items: %v", err)
	}
	if questionCount == 0 {
		t.Fatalf("expected theory seed data in sqlite")
	}
}
