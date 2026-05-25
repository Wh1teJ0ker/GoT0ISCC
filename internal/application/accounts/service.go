package accounts

import (
	"context"
	"errors"
	"strings"

	domain "got0iscc/desktop/internal/domain/accounts"
	"got0iscc/desktop/internal/platform/runtime"
)

type Repository interface {
	ListAccounts(ctx context.Context) ([]domain.Account, error)
	SaveAccount(ctx context.Context, account domain.Account) (domain.Account, error)
	DeleteAccount(ctx context.Context, id int64) error
	AccountSummary(ctx context.Context) (domain.Summary, error)
}

type Service struct {
	repo   Repository
	layout runtime.Layout
}

type Payload struct {
	Accounts []domain.Account `json:"accounts"`
	Summary  domain.Summary   `json:"summary"`
}

func NewService(repo Repository, layout runtime.Layout) *Service {
	return &Service{repo: repo, layout: layout}
}

func (s *Service) List(ctx context.Context) (Payload, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return Payload{}, err
	}
	summary, err := s.repo.AccountSummary(ctx)
	if err != nil {
		return Payload{}, err
	}
	return Payload{Accounts: accounts, Summary: summary}, nil
}

func (s *Service) Save(ctx context.Context, account domain.Account) (domain.Account, error) {
	normalized, err := normalizeAccount(account)
	if err != nil {
		return domain.Account{}, err
	}
	return s.repo.SaveAccount(ctx, normalized)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("account id is required")
	}
	return s.repo.DeleteAccount(ctx, id)
}

func normalizeAccount(account domain.Account) (domain.Account, error) {
	account.Name = strings.TrimSpace(account.Name)
	account.Username = strings.TrimSpace(account.Username)

	if account.Name == "" {
		return domain.Account{}, errors.New("账号名称不能为空")
	}
	return account, nil
}
