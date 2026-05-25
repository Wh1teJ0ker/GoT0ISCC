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
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		candidate := filepath.Join(previous, "data", "got0iscc.init.sql")
		if _, err := os.Stat(candidate); err == nil {
			layout.InitSeedSQLPath = candidate
			break
		}
		parent := filepath.Dir(previous)
		if parent == previous {
			t.Fatalf("find seed sql")
		}
		previous = parent
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
