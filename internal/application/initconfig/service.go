package initconfig

import (
	"context"
	"errors"
	"os"
	"strings"

	runtimeplatform "got0iscc/desktop/internal/platform/runtime"
	"got0iscc/desktop/internal/platform/storage/sqlite"
)

const metaInitApplied = "app.init.applied"

type Service struct {
	layout runtimeplatform.Layout
	store  *sqlite.Store
}

func NewService(layout runtimeplatform.Layout, store *sqlite.Store) *Service {
	return &Service{layout: layout, store: store}
}

func (s *Service) EnsureInitialized(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	applied, err := s.store.MetaValue(ctx, metaInitApplied)
	if err != nil {
		return err
	}
	if strings.TrimSpace(applied) == "1" {
		return nil
	}

	data, err := os.ReadFile(s.layout.InitSeedSQLPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.store.SetMetaValue(ctx, metaInitApplied, "1")
		}
		return err
	}

	if err := s.store.ExecuteSeedSQL(ctx, string(data)); err != nil {
		return err
	}
	return s.store.SetMetaValue(ctx, metaInitApplied, "1")
}
