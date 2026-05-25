package dashboard

import (
	"context"

	combatdomain "got0iscc/desktop/internal/domain/combat"
	trackdomain "got0iscc/desktop/internal/domain/tracks"
	theorydomain "got0iscc/desktop/internal/domain/theory"
	wpdomain "got0iscc/desktop/internal/domain/wp"
)

type TrackLoader interface {
	Practice(ctx context.Context) (trackdomain.Payload, error)
	Arena(ctx context.Context) (trackdomain.Payload, error)
}

type TheoryLoader interface {
	Snapshot(ctx context.Context) (theorydomain.Payload, error)
}

type CombatLoader interface {
	Snapshot(ctx context.Context) (combatdomain.Payload, error)
}

type WPLoader interface {
	List(ctx context.Context) (wpdomain.Payload, error)
}

type Service struct {
	tracks TrackLoader
	theory TheoryLoader
	combat CombatLoader
	wp     WPLoader
}

type Row struct {
	Track     string `json:"track"`
	Total     int    `json:"total"`
	Submitted int    `json:"submitted"`
	WP        int    `json:"wp"`
	Missing   int    `json:"missing"`
	Status    string `json:"status"`
}

type Payload struct {
	Rows []Row `json:"rows"`
}

func NewService(tracks TrackLoader, theory TheoryLoader, combat CombatLoader, wp WPLoader) *Service {
	return &Service{tracks: tracks, theory: theory, combat: combat, wp: wp}
}

func (s *Service) Summary(ctx context.Context) (Payload, error) {
	practice, err := s.tracks.Practice(ctx)
	if err != nil {
		return Payload{}, err
	}
	arena, err := s.tracks.Arena(ctx)
	if err != nil {
		return Payload{}, err
	}
	theory, err := s.theory.Snapshot(ctx)
	if err != nil {
		return Payload{}, err
	}
	combat, err := s.combat.Snapshot(ctx)
	if err != nil {
		return Payload{}, err
	}
	writeups, err := s.wp.List(ctx)
	if err != nil {
		return Payload{}, err
	}

	wpBySection := map[string]int{}
	wpGapBySection := map[string]int{}
	for _, item := range writeups.Items {
		if item.Status == "submitted" {
			wpBySection[item.SectionLabel]++
		}
		if item.Status == "missing" || item.Status == "needs_fix" || item.Status == "sync_failed" {
			wpGapBySection[item.SectionLabel]++
		}
	}

	rows := []Row{
		{
			Track:     "练武题",
			Total:     practice.Summary.TotalChallenges,
			Submitted: practice.Summary.SolvedChallenges,
			WP:        wpBySection["练武题"],
			Missing:   maxInt(practice.Summary.PendingChallenges, wpGapBySection["练武题"]),
			Status:    "live",
		},
		{
			Track:     "擂台题",
			Total:     arena.Summary.TotalChallenges,
			Submitted: arena.Summary.SolvedChallenges,
			WP:        wpBySection["擂台题"],
			Missing:   maxInt(arena.Summary.PendingChallenges, wpGapBySection["擂台题"]),
			Status:    "live",
		},
		{
			Track:     "理论题",
			Total:     theory.Statistics.QuestionNumber,
			Submitted: maxInt(theory.Statistics.QuestionNumber-1, 0),
			WP:        0,
			Missing:   boolInt(theory.Question.Title != ""),
			Status:    "live",
		},
		{
			Track:     "实战题",
			Total:     combat.Summary.ChallengeCount,
			Submitted: 0,
			WP:        wpBySection["实战题"],
			Missing:   maxInt(combat.Summary.ChallengeCount-wpBySection["实战题"], 0),
			Status:    "live",
		},
	}

	return Payload{Rows: rows}, nil
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
