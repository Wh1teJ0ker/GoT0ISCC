package theory

import "testing"

func TestNormalizeAIReviewOutcome(t *testing.T) {
	t.Run("clear approved stays approved", func(t *testing.T) {
		status, reason := normalizeAIReviewOutcome("single", []string{"A"}, []string{"选项A"}, 0.96, "approved", "答案明确")
		if status != "approved" {
			t.Fatalf("expected approved, got %s", status)
		}
		if reason != "答案明确" {
			t.Fatalf("unexpected reason: %s", reason)
		}
	})

	t.Run("low confidence goes to manual review", func(t *testing.T) {
		status, reason := normalizeAIReviewOutcome("single", []string{"A"}, []string{"选项A"}, 0.41, "approved", "答案可能正确")
		if status != "pending" {
			t.Fatalf("expected pending, got %s", status)
		}
		if reason == "" {
			t.Fatalf("expected manual review reason")
		}
	})

	t.Run("ambiguous reason goes to manual review", func(t *testing.T) {
		status, reason := normalizeAIReviewOutcome("single", []string{"A"}, []string{"选项A"}, 0.95, "approved", "存在歧义，建议人工确认")
		if status != "pending" {
			t.Fatalf("expected pending, got %s", status)
		}
		if reason == "" {
			t.Fatalf("expected manual review reason")
		}
	})

	t.Run("single choice count mismatch goes to manual review", func(t *testing.T) {
		status, reason := normalizeAIReviewOutcome("single", []string{"A", "B"}, []string{"选项A", "选项B"}, 0.99, "approved", "")
		if status != "pending" {
			t.Fatalf("expected pending, got %s", status)
		}
		if reason == "" {
			t.Fatalf("expected manual review reason")
		}
	})
}
