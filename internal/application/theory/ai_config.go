package theory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	domain "got0iscc/desktop/internal/domain/theory"
	"net/http"
	"strings"
	"time"
)

const (
	defaultAIBaseURL         = "https://api.openai.com/v1"
	defaultAIModel           = "gpt-5.4"
	defaultAIReasoningEffort = "high"
	metaTheoryAISettings     = "theory.ai_settings"
)

var supportedAIModels = []string{"gpt-5.4"}

func (s *Service) AISettings(_ context.Context) (domain.AISettingsPayload, error) {
	settings, err := s.loadAISettings(context.Background())
	if err != nil {
		return domain.AISettingsPayload{}, err
	}
	return aiSettingsPayload(settings), nil
}

func (s *Service) SaveAISettings(ctx context.Context, input domain.AISettings) (domain.AISettingsPayload, error) {
	settings, err := normalizeAISettings(input)
	if err != nil {
		return domain.AISettingsPayload{}, err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return domain.AISettingsPayload{}, err
	}
	if s.repo == nil {
		return domain.AISettingsPayload{}, errors.New("理论题配置仓库不可用")
	}
	if err := s.repo.SetMetaValue(ctx, metaTheoryAISettings, string(data)); err != nil {
		return domain.AISettingsPayload{}, err
	}
	return aiSettingsPayload(settings), nil
}

func (s *Service) TestAISettings(ctx context.Context, input domain.AISettings) (domain.AIAvailability, error) {
	settings, err := normalizeAISettings(input)
	if err != nil {
		return domain.AIAvailability{}, err
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		return domain.AIAvailability{
			OK:        false,
			Status:    "not_ready",
			Model:     settings.Model,
			BaseURL:   strings.TrimRight(settings.BaseURL, "/"),
			Message:   "缺少 API Key",
			CheckedAt: theoryNowTS(),
		}, nil
	}

	body := map[string]any{
		"model": settings.Model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "ping"},
				},
			},
		},
		"max_output_tokens": 1,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.AIAvailability{}, err
	}

	startedAt := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(settings.BaseURL, "/")+"/responses", bytes.NewReader(payload))
	if err != nil {
		return domain.AIAvailability{}, err
	}
	req.Header.Set("Authorization", "Bearer "+settings.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	latency := time.Since(startedAt).Milliseconds()
	if err != nil {
		return domain.AIAvailability{
			OK:        false,
			Status:    "error",
			Model:     settings.Model,
			BaseURL:   strings.TrimRight(settings.BaseURL, "/"),
			LatencyMS: latency,
			Message:   err.Error(),
			CheckedAt: theoryNowTS(),
		}, nil
	}
	defer resp.Body.Close()

	result := domain.AIAvailability{
		OK:             resp.StatusCode < 400,
		Model:          settings.Model,
		BaseURL:        strings.TrimRight(settings.BaseURL, "/"),
		LatencyMS:      latency,
		HTTPStatusCode: resp.StatusCode,
		CheckedAt:      theoryNowTS(),
	}
	if resp.StatusCode < 400 {
		result.Status = "ok"
		result.Message = "可用"
		return result, nil
	}

	result.Status = "error"
	result.Message = fmt.Sprintf("请求失败: %s", resp.Status)
	return result, nil
}

func (s *Service) loadAISettings(ctx context.Context) (domain.AISettings, error) {
	settings := domain.AISettings{
		Enabled:         false,
		BaseURL:         defaultAIBaseURL,
		Model:           defaultAIModel,
		ReasoningEffort: defaultAIReasoningEffort,
		Prompt:          defaultTheoryPrompt(),
	}
	if s.repo == nil {
		return settings, nil
	}
	raw, err := s.repo.MetaValue(ctx, metaTheoryAISettings)
	if err != nil {
		return domain.AISettings{}, fmt.Errorf("读取理论题 AI 配置失败: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return settings, nil
	}
	var merged domain.AISettings
	if err := json.Unmarshal([]byte(raw), &merged); err != nil {
		return domain.AISettings{}, fmt.Errorf("解析理论题 AI 配置失败: %w", err)
	}
	if strings.TrimSpace(merged.BaseURL) == "" {
		merged.BaseURL = settings.BaseURL
	}
	if strings.TrimSpace(merged.Model) == "" {
		merged.Model = settings.Model
	}
	if strings.TrimSpace(merged.ReasoningEffort) == "" {
		merged.ReasoningEffort = settings.ReasoningEffort
	}
	if strings.TrimSpace(merged.Prompt) == "" {
		merged.Prompt = settings.Prompt
	}
	return merged, nil
}

func normalizeAISettings(input domain.AISettings) (domain.AISettings, error) {
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.APIKey = strings.TrimSpace(input.APIKey)
	input.Model = strings.TrimSpace(input.Model)
	input.ReasoningEffort = strings.TrimSpace(strings.ToLower(input.ReasoningEffort))
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.BaseURL == "" {
		input.BaseURL = defaultAIBaseURL
	}
	if input.Model == "" {
		input.Model = defaultAIModel
	}
	if input.ReasoningEffort == "" {
		input.ReasoningEffort = defaultAIReasoningEffort
	}
	if input.Prompt == "" {
		input.Prompt = defaultTheoryPrompt()
	}
	if input.Model != defaultAIModel {
		return domain.AISettings{}, fmt.Errorf("当前只支持模型: %s", defaultAIModel)
	}
	switch input.ReasoningEffort {
	case "minimal", "low", "medium", "high", "xhigh":
	default:
		return domain.AISettings{}, errors.New("reasoning_effort 仅支持 minimal/low/medium/high/xhigh")
	}
	input.UpdatedAt = theoryNowTS()
	return input, nil
}

func aiSettingsPayload(settings domain.AISettings) domain.AISettingsPayload {
	ready := settings.Enabled && strings.TrimSpace(settings.APIKey) != ""
	return domain.AISettingsPayload{
		Settings:      settings,
		ConfigPath:    "sqlite:meta/theory.ai_settings",
		MaskedAPIKey:  maskTheoryAPIKey(settings.APIKey),
		Ready:         ready,
		ProviderLabel: "OpenAI Responses API",
		SupportsModel: append([]string(nil), supportedAIModels...),
	}
}

func maskTheoryAPIKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 8 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:4]) + strings.Repeat("*", len(runes)-8) + string(runes[len(runes)-4:])
}

func defaultTheoryPrompt() string {
	return strings.TrimSpace(`你是 ISCC 理论题辅助判题器。
请结合当前题目、选项、以及本地题库候选结果，判断最可能的正确答案。
如果题目是多选题，请返回多个答案。
不要解释过长，只输出结论、置信度和简短原因。`)
}
