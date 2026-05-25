package theory

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	domain "got0iscc/desktop/internal/domain/theory"
)

type rawBankOption struct {
	Content   string `json:"content"`
	IsCorrect bool   `json:"isCorrect"`
}

type rawBankEntry struct {
	Question string `json:"question"`
	Options  []rawBankOption `json:"options"`
}

type bankIndex struct {
	GeneratedAt     string            `json:"generated_at"`
	SourcePath      string            `json:"source_path"`
	SourcePaths     []string          `json:"source_paths,omitempty"`
	SourceSignature string            `json:"source_signature,omitempty"`
	IndexPath       string            `json:"index_path"`
	RawCount        int               `json:"raw_count"`
	Items           []bankIndexedItem `json:"items"`
}

type bankIndexedItem struct {
	ID                 string              `json:"id"`
	ReviewID           int64               `json:"review_id,omitempty"`
	QuestionHash       string              `json:"question_hash,omitempty"`
	Question           string              `json:"question"`
	NormalizedQuestion string              `json:"normalized_question"`
	CompactQuestion    string              `json:"compact_question"`
	SearchText         string              `json:"search_text"`
	Keywords           []string            `json:"keywords"`
	CorrectOptions     []string            `json:"correct_options"`
	CorrectTexts       []string            `json:"correct_texts"`
	DuplicateGroup     string              `json:"duplicate_group,omitempty"`
	Options            []domain.BankOption `json:"options"`
	MultiAnswer        bool                `json:"multi_answer"`
}

type bankStore struct {
	sourcePath string
	indexPath  string
	signature  string
	summary    domain.BankSummary
	items      []bankIndexedItem
}

type bankSearchRequest struct {
	Query string
	Limit int
}

var bankCache struct {
	mu    sync.Mutex
	store map[string]*bankStore
}

func loadBankStore(path string) (*bankStore, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	bankCache.mu.Lock()
	defer bankCache.mu.Unlock()

	if bankCache.store == nil {
		bankCache.store = map[string]*bankStore{}
	}

	sources, err := discoverBankSources(absPath)
	if err != nil {
		return nil, err
	}
	sourceSignature := buildBankSourceSignature(sources)
	latestSourceTime, err := latestBankSourceModTime(sources)
	if err != nil {
		return nil, err
	}

	indexPath := normalizedBankPath(absPath)
	indexInfo, _ := os.Stat(indexPath)

	if cached := bankCache.store[absPath]; cached != nil && cached.signature == sourceSignature && indexInfo != nil && !latestSourceTime.After(indexInfo.ModTime()) {
		return cached, nil
	}

	store, err := buildBankStore(absPath, indexPath, sources, sourceSignature)
	if err != nil {
		return nil, err
	}
	bankCache.store[absPath] = store
	return store, nil
}

func buildBankStore(sourcePath string, indexPath string, sources []string, sourceSignature string) (*bankStore, error) {
	rawEntries, err := loadRawBankEntries(sources)
	if err != nil {
		return nil, err
	}

	index := bankIndex{
		GeneratedAt:     time.Now().Format("2006-01-02 15:04:05"),
		SourcePath:      sourcePath,
		SourcePaths:     append([]string(nil), sources...),
		SourceSignature: sourceSignature,
		IndexPath:       indexPath,
		RawCount:        len(rawEntries),
		Items:           make([]bankIndexedItem, 0, len(rawEntries)),
	}

	duplicateBuckets := map[string]int{}
	for _, item := range rawEntries {
		key := compactTheoryText(item.Question)
		if key != "" {
			duplicateBuckets[key]++
		}
	}

	duplicateGroups := 0
	multiAnswerCount := 0
	for _, count := range duplicateBuckets {
		if count > 1 {
			duplicateGroups++
		}
	}

	for idx, entry := range rawEntries {
		indexed := normalizeBankEntry(idx, entry)
		if indexed.ID == "" || indexed.NormalizedQuestion == "" {
			continue
		}
		if duplicateBuckets[indexed.CompactQuestion] > 1 {
			indexed.DuplicateGroup = indexed.CompactQuestion
		}
		if indexed.MultiAnswer {
			multiAnswerCount++
		}
		index.Items = append(index.Items, indexed)
	}

	sort.Slice(index.Items, func(i, j int) bool {
		left := index.Items[i]
		right := index.Items[j]
		if left.NormalizedQuestion == right.NormalizedQuestion {
			return left.ID < right.ID
		}
		return left.NormalizedQuestion < right.NormalizedQuestion
	})

	if err := persistBankIndex(indexPath, index); err != nil {
		return nil, err
	}

	summary := domain.BankSummary{
		RawCount:         len(rawEntries),
		SearchableCount:  len(index.Items),
		DuplicateGroups:  duplicateGroups,
		MultiAnswerCount: multiAnswerCount,
		GeneratedAt:      index.GeneratedAt,
		SourcePath:       sourcePath,
		IndexPath:        indexPath,
	}

	return &bankStore{
		sourcePath: sourcePath,
		indexPath:  indexPath,
		signature:  sourceSignature,
		summary:    summary,
		items:      index.Items,
	}, nil
}

func persistBankIndex(indexPath string, index bankIndex) error {
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, payload, 0o644)
}

func normalizedBankPath(sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	base := strings.TrimSuffix(filepath.Base(sourcePath), ext)
	return filepath.Join(filepath.Dir(sourcePath), base+".normalized.json")
}

func normalizeBankEntry(index int, entry rawBankEntry) bankIndexedItem {
	question := cleanQuestionText(entry.Question)
	normalizedQuestion := normalizeTheoryText(question)
	compactQuestion := compactTheoryText(question)
	correctOptions := make([]string, 0, len(entry.Options))
	correctTexts := make([]string, 0, len(entry.Options))
	options := make([]domain.BankOption, 0, len(entry.Options))
	keywords := make([]string, 0, 12)
	keywordSet := map[string]struct{}{}

	for idx, option := range entry.Options {
		key := string(rune('A' + idx))
		content := cleanOptionText(option.Content)
		normalizedOption := normalizeTheoryText(content)
		options = append(options, domain.BankOption{
			Key:       key,
			Content:   content,
			IsCorrect: option.IsCorrect,
		})
		addTheoryKeywords(keywordSet, &keywords, content)
		addTheoryKeywords(keywordSet, &keywords, normalizedOption)
		if option.IsCorrect {
			correctOptions = append(correctOptions, key)
			correctTexts = append(correctTexts, content)
		}
	}

	addTheoryKeywords(keywordSet, &keywords, question)
	addTheoryKeywords(keywordSet, &keywords, normalizedQuestion)

	sum := sha1.Sum([]byte(fmt.Sprintf("%d:%s:%s", index, normalizedQuestion, strings.Join(correctTexts, "|"))))
	searchTextParts := []string{normalizedQuestion, compactQuestion}
	searchTextParts = append(searchTextParts, keywords...)
	searchText := strings.Join(uniqueTheoryStrings(searchTextParts), " ")

	return bankIndexedItem{
		ID:                 hex.EncodeToString(sum[:8]),
		QuestionHash:       theoryQuestionHash(question, options),
		Question:           question,
		NormalizedQuestion: normalizedQuestion,
		CompactQuestion:    compactQuestion,
		SearchText:         searchText,
		Keywords:           keywords,
		CorrectOptions:     correctOptions,
		CorrectTexts:       correctTexts,
		DuplicateGroup:     "",
		Options:            options,
		MultiAnswer:        len(correctOptions) > 1,
	}
}

func searchBank(store *bankStore, req bankSearchRequest) domain.BankSearchResponse {
	query := strings.TrimSpace(req.Query)
	normalizedQuery := normalizeTheoryText(query)
	compactQuery := compactTheoryText(query)
	limit := req.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}

	response := domain.BankSearchResponse{
		Query:           query,
		NormalizedQuery: normalizedQuery,
		Limit:           limit,
		Summary:         store.summary,
		Items:           []domain.BankSearchHit{},
	}
	if normalizedQuery == "" {
		return response
	}

	type scored struct {
		item   bankIndexedItem
		score  float64
		reason string
	}

	results := make([]scored, 0, len(store.items))
	for _, item := range store.items {
		score, reason := scoreBankItem(normalizedQuery, compactQuery, item)
		if score <= 0 {
			continue
		}
		results = append(results, scored{item: item, score: score, reason: reason})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score == results[j].score {
			return results[i].item.ID < results[j].item.ID
		}
		return results[i].score > results[j].score
	})

	response.Total = len(results)
	if len(results) > limit {
		results = results[:limit]
	}
	for _, item := range results {
		response.Items = append(response.Items, domain.BankSearchHit{
			ID:                 item.item.ID,
			Question:           item.item.Question,
			NormalizedQuestion: item.item.NormalizedQuestion,
			CorrectOptions:     append([]string(nil), item.item.CorrectOptions...),
			CorrectTexts:       append([]string(nil), item.item.CorrectTexts...),
			Score:              round(item.score),
			MatchReason:        item.reason,
			Keywords:           append([]string(nil), item.item.Keywords...),
			DuplicateGroup:     item.item.DuplicateGroup,
			MultiAnswer:        item.item.MultiAnswer,
			Options:            append([]domain.BankOption(nil), item.item.Options...),
		})
	}

	return response
}

func scoreBankItem(normalizedQuery string, compactQuery string, item bankIndexedItem) (float64, string) {
	if normalizedQuery == "" {
		return 0, ""
	}

	score := similarity(normalizedQuery, item.NormalizedQuestion)
	reason := "相似题干匹配"

	if compactQuery != "" && compactQuery == item.CompactQuestion {
		return 1, "归一化题干完全一致"
	}
	if compactQuery != "" && strings.Contains(item.CompactQuestion, compactQuery) {
		score = math.Max(score, 0.95)
		reason = "题干子串命中"
	}
	if compactQuery != "" && strings.Contains(compactQuery, item.CompactQuestion) {
		score = math.Max(score, 0.9)
		reason = "查询文本覆盖题干"
	}
	if strings.Contains(item.SearchText, normalizedQuery) {
		score = math.Max(score, 0.86)
		reason = "归一化搜索文本命中"
	}

	for _, keyword := range item.Keywords {
		if keyword != "" && strings.Contains(keyword, normalizedQuery) {
			score = math.Max(score, 0.8)
			reason = "关键词命中"
			break
		}
	}

	if score < 0.2 {
		return 0, ""
	}
	return score, reason
}

func addTheoryKeywords(seen map[string]struct{}, bucket *[]string, raw string) {
	value := normalizeTheoryText(raw)
	if value == "" {
		return
	}
	for _, field := range strings.Fields(value) {
		if len([]rune(field)) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		*bucket = append(*bucket, field)
	}
	if compact := compactTheoryText(value); compact != "" {
		if _, ok := seen[compact]; !ok {
			seen[compact] = struct{}{}
			*bucket = append(*bucket, compact)
		}
	}
}

func uniqueTheoryStrings(values []string) []string {
	seen := map[string]struct{}{}
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
