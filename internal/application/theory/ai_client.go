package theory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	domain "got0iscc/desktop/internal/domain/theory"
)

const aiReviewApproveConfidenceThreshold = 0.8

var aiReviewManualReasonKeywords = []string{
	"不确定",
	"无法确定",
	"不能确定",
	"难以确定",
	"存疑",
	"模糊",
	"歧义",
	"信息不足",
	"证据不足",
	"需人工",
	"请人工",
	"manual review",
	"human review",
	"uncertain",
	"ambiguous",
	"unclear",
	"insufficient",
	"not enough information",
}

type aiClientResponse struct {
	Output []struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
}

type aiDecision struct {
	RecommendedOptions []string `json:"recommended_options"`
	RecommendedTexts   []string `json:"recommended_texts"`
	Confidence         float64  `json:"confidence"`
	Reason             string   `json:"reason"`
}

type aiReviewDecision struct {
	ID            int64               `json:"id"`
	Question      string              `json:"question"`
	SelectionType string              `json:"selection_type"`
	Options       []domain.BankOption `json:"options"`
	AnswerKeys    []string            `json:"answer_keys"`
	AnswerTexts   []string            `json:"answer_texts"`
	ReviewStatus  string              `json:"review_status"`
	ReviewReason  string              `json:"review_reason"`
	Confidence    float64             `json:"confidence"`
}

func (s *Service) evaluateWithAI(ctx context.Context, settings domain.AISettings, question domain.Question, match domain.Match) domain.AIInsight {
	insight := domain.AIInsight{
		Enabled: settings.Enabled,
		Ready:   strings.TrimSpace(settings.APIKey) != "",
		Status:  "disabled",
		Model:   settings.Model,
	}
	if !settings.Enabled {
		insight.Reason = "AI 判题未启用。"
		return insight
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		insight.Status = "not_ready"
		insight.Error = "已启用 AI，但尚未填写 API Key。"
		return insight
	}

	decision, err := requestAIDecision(ctx, settings, question, match)
	if err != nil {
		insight.Status = "error"
		insight.Error = err.Error()
		return insight
	}
	insight.Status = "ready"
	insight.RecommendedOptions = decision.RecommendedOptions
	insight.RecommendedTexts = decision.RecommendedTexts
	insight.Confidence = decision.Confidence
	insight.Reason = decision.Reason
	return insight
}

func requestAIDecision(ctx context.Context, settings domain.AISettings, question domain.Question, match domain.Match) (aiDecision, error) {
	body := map[string]any{
		"model": settings.Model,
		"reasoning": map[string]any{
			"effort": settings.ReasoningEffort,
		},
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]any{
					{"type": "input_text", "text": settings.Prompt},
				},
			},
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": buildAIQuestionPrompt(question, match)},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type": "json_schema",
				"name": "theory_answer",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"recommended_options": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"recommended_texts": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
						"confidence": map[string]any{
							"type": "number",
						},
						"reason": map[string]any{
							"type": "string",
						},
					},
					"required":             []string{"recommended_options", "recommended_texts", "confidence", "reason"},
					"additionalProperties": false,
				},
			},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return aiDecision{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(settings.BaseURL, "/")+"/responses", bytes.NewReader(payload))
	if err != nil {
		return aiDecision{}, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return aiDecision{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return aiDecision{}, fmt.Errorf("AI 请求失败: %s", resp.Status)
	}

	var result aiClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return aiDecision{}, err
	}
	text := extractAIText(result)
	if strings.TrimSpace(text) == "" {
		return aiDecision{}, fmt.Errorf("AI 未返回可解析内容")
	}
	var decision aiDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		return aiDecision{}, fmt.Errorf("AI 返回内容解析失败: %w", err)
	}
	if decision.Confidence < 0 {
		decision.Confidence = 0
	}
	if decision.Confidence > 1 {
		decision.Confidence = 1
	}
	return decision, nil
}

func extractAIText(result aiClientResponse) string {
	var parts []string
	for _, output := range result.Output {
		for _, content := range output.Content {
			if content.Type == "output_text" || content.Type == "text" {
				parts = append(parts, content.Text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildAIQuestionPrompt(question domain.Question, match domain.Match) string {
	type candidate struct {
		Question           string   `json:"question"`
		RecommendedOptions []string `json:"recommended_options"`
		RecommendedTexts   []string `json:"recommended_texts"`
		Confidence         float64  `json:"confidence"`
	}
	payload := map[string]any{
		"question": map[string]any{
			"title":          question.Title,
			"selection_type": question.SelectionType,
			"options":        question.Options,
		},
		"local_match": map[string]any{
			"status":              match.Status,
			"confidence":          match.Confidence,
			"recommended_option":  match.RecommendedOption,
			"recommended_options": match.RecommendedOptions,
			"recommended_texts":   match.RecommendedTexts,
			"reason":              match.Reason,
		},
	}
	candidates := make([]candidate, 0, len(match.Candidates))
	for _, item := range match.Candidates {
		candidates = append(candidates, candidate{
			Question:           item.Question,
			RecommendedOptions: item.RecommendedOptions,
			RecommendedTexts:   item.RecommendedTexts,
			Confidence:         item.Confidence,
		})
	}
	payload["candidates"] = candidates
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func requestAIReviewBatch(ctx context.Context, settings domain.AISettings, batch []domain.ReviewItem, reasoningEffort string, timeoutSeconds int) ([]aiReviewDecision, error) {
	type reviewItemPayload struct {
		ID            int64               `json:"id"`
		Question      string              `json:"question"`
		SelectionType string              `json:"selection_type"`
		Options       []domain.BankOption `json:"options"`
		AnswerKeys    []string            `json:"answer_keys_hint"`
		AnswerTexts   []string            `json:"answer_texts_hint"`
		ReviewStatus  string              `json:"review_status"`
		ReviewReason  string              `json:"review_reason"`
	}
	body := map[string]any{
		"model": settings.Model,
		"reasoning": map[string]any{
			"effort": reasoningEffort,
		},
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": "You review ISCC theory question-bank records currently pending manual review. Determine the most reliable correct answer keys only from the question and options. Only return review_status=approved when the answer is unambiguous and high-confidence. If the question is ambiguous, incomplete, unsafe, or not confidently answerable, return review_status=pending and explain why manual review is needed. Return JSON only.",
					},
				},
			},
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": mustJSON(map[string]any{
							"items": mapReviewItems(batch),
						}),
					},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type": "json_schema",
				"name": "theory_review_batch",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"items": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"id":             map[string]any{"type": "integer"},
									"question":       map[string]any{"type": "string"},
									"selection_type": map[string]any{"type": "string"},
									"answer_keys": map[string]any{
										"type":  "array",
										"items": map[string]any{"type": "string"},
									},
									"review_status": map[string]any{"type": "string"},
									"review_reason": map[string]any{"type": "string"},
									"confidence":    map[string]any{"type": "number"},
								},
								"required":             []string{"id", "question", "selection_type", "answer_keys", "review_status", "review_reason", "confidence"},
								"additionalProperties": false,
							},
						},
					},
					"required":             []string{"items"},
					"additionalProperties": false,
				},
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(settings.BaseURL, "/")+"/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("AI 请求失败: %s", resp.Status)
	}

	var result aiClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	text := extractAIText(result)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("AI 未返回可解析内容")
	}

	var parsed struct {
		Items []struct {
			ID            int64    `json:"id"`
			Question      string   `json:"question"`
			SelectionType string   `json:"selection_type"`
			AnswerKeys    []string `json:"answer_keys"`
			ReviewStatus  string   `json:"review_status"`
			ReviewReason  string   `json:"review_reason"`
			Confidence    float64  `json:"confidence"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("AI 返回内容解析失败: %w", err)
	}

	byID := make(map[int64]domain.ReviewItem, len(batch))
	for _, item := range batch {
		byID[item.ID] = item
	}

	decisions := make([]aiReviewDecision, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		source := byID[item.ID]
		answerKeySet := make(map[string]struct{}, len(item.AnswerKeys))
		answerKeys := make([]string, 0, len(item.AnswerKeys))
		answerTexts := make([]string, 0, len(item.AnswerKeys))
		for _, key := range item.AnswerKeys {
			key = strings.TrimSpace(strings.ToUpper(key))
			if key == "" {
				continue
			}
			if _, exists := answerKeySet[key]; exists {
				continue
			}
			answerKeySet[key] = struct{}{}
			answerKeys = append(answerKeys, key)
		}
		for _, option := range source.Options {
			if _, ok := answerKeySet[strings.TrimSpace(strings.ToUpper(option.Key))]; ok {
				answerTexts = append(answerTexts, option.Content)
			}
		}
		confidence := item.Confidence
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		selectionType := normalizeSelectionType(firstNonEmpty(item.SelectionType, source.SelectionType), len(answerKeys))
		reviewStatus := normalizeAIReviewStatus(item.ReviewStatus, len(answerKeys))
		reviewReason := strings.TrimSpace(item.ReviewReason)
		reviewStatus, reviewReason = normalizeAIReviewOutcome(selectionType, answerKeys, answerTexts, confidence, reviewStatus, reviewReason)
		decisions = append(decisions, aiReviewDecision{
			ID:            item.ID,
			Question:      firstNonEmpty(item.Question, source.Question),
			SelectionType: selectionType,
			Options:       source.Options,
			AnswerKeys:    answerKeys,
			AnswerTexts:   answerTexts,
			ReviewStatus:  reviewStatus,
			ReviewReason:  reviewReason,
			Confidence:    confidence,
		})
	}
	return decisions, nil
}

func mapReviewItems(items []domain.ReviewItem) []map[string]any {
	payload := make([]map[string]any, 0, len(items))
	for _, item := range items {
		payload = append(payload, map[string]any{
			"id":                item.ID,
			"question":          item.Question,
			"selection_type":    item.SelectionType,
			"options":           item.Options,
			"answer_keys_hint":  item.AnswerKeys,
			"answer_texts_hint": item.AnswerTexts,
			"review_status":     item.ReviewStatus,
			"review_reason":     item.ReviewReason,
		})
	}
	return payload
}

func mustJSON(value any) string {
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func normalizeSelectionType(value string, answerCount int) string {
	value = strings.TrimSpace(value)
	if value == "single" || value == "multiple" {
		return value
	}
	if answerCount > 1 {
		return "multiple"
	}
	return "single"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeAIReviewStatus(value string, answerCount int) string {
	status := strings.TrimSpace(strings.ToLower(value))
	if answerCount == 0 {
		return "pending"
	}
	switch status {
	case "", "pending", "captured", "unknown", "uncertain", "needs_review", "review":
		return "pending"
	case "approved", "approve", "reviewed", "solved", "confirmed", "ok", "correct":
		return "approved"
	case "rejected", "reject", "wrong", "incorrect":
		return "rejected"
	default:
		return "approved"
	}
}

func normalizeAIReviewOutcome(selectionType string, answerKeys []string, answerTexts []string, confidence float64, reviewStatus string, reviewReason string) (string, string) {
	reviewReason = strings.TrimSpace(reviewReason)
	manualReasons := make([]string, 0, 4)

	if reviewStatus != "approved" {
		manualReasons = append(manualReasons, "AI 未给出明确通过结论，转人工复核")
	}
	if len(answerKeys) == 0 {
		manualReasons = append(manualReasons, "AI 未给出明确答案，转人工复核")
	}
	if len(answerTexts) != len(answerKeys) {
		manualReasons = append(manualReasons, "AI 返回答案与选项匹配不完整，转人工复核")
	}
	if selectionType == "single" && len(answerKeys) != 1 {
		manualReasons = append(manualReasons, "AI 返回答案数量与单选题不匹配，转人工复核")
	}
	if confidence < aiReviewApproveConfidenceThreshold {
		manualReasons = append(manualReasons, fmt.Sprintf("AI 置信度 %.2f 偏低，转人工复核", confidence))
	}
	if containsAIManualReviewKeyword(reviewReason) {
		manualReasons = append(manualReasons, "AI 审核理由存在模糊表述，转人工复核")
	}

	if len(manualReasons) == 0 {
		return "approved", reviewReason
	}
	return "pending", mergeAIReviewReason(reviewReason, strings.Join(uniqueAIReviewReasons(manualReasons), "；"))
}

func containsAIManualReviewKeyword(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, keyword := range aiReviewManualReasonKeywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func mergeAIReviewReason(base string, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	if base == "" {
		return extra
	}
	if extra == "" || base == extra {
		return base
	}
	if strings.Contains(base, extra) {
		return base
	}
	return base + "；" + extra
}

func uniqueAIReviewReasons(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
