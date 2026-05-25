package theory

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	docxParagraphCloseRe  = regexp.MustCompile(`</w:p>`)
	docxTagRe             = regexp.MustCompile(`<[^>]+>`)
	docxQuestionStartRe   = regexp.MustCompile(`^第\s*\d+\s*题`)
	docxQuestionPrefixRe  = regexp.MustCompile(`^第\s*(\d+)\s*题`)
	docxEmbeddedAnswerRe  = regexp.MustCompile(`_{2,}\s*([A-D]{1,4})\s*_{2,}`)
	docxTrailingAnswerRe  = regexp.MustCompile(`([A-D]{1,4})\s*[。．.]?\s*$`)
	docxOptionMarkerRe    = regexp.MustCompile(`[A-D][\.．、]`)
)

func discoverBankSources(primaryPath string) ([]string, error) {
	absPrimary, err := filepath.Abs(primaryPath)
	if err != nil {
		return nil, err
	}

	sources := []string{}
	if _, err := os.Stat(absPrimary); err == nil {
		sources = append(sources, absPrimary)
	}

	baseName := strings.ToLower(filepath.Base(absPrimary))
	if strings.Contains(baseName, ".standardized.") || strings.Contains(baseName, ".normalized.") {
		if len(sources) == 0 {
			return nil, fmt.Errorf("no theory bank sources found near %s", absPrimary)
		}
		return sources, nil
	}

	pattern := filepath.Join(filepath.Dir(absPrimary), "*题库*.docx")
	docxMatches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(docxMatches)
	for _, item := range docxMatches {
		absItem, err := filepath.Abs(item)
		if err != nil {
			return nil, err
		}
		if containsString(sources, absItem) {
			continue
		}
		sources = append(sources, absItem)
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no theory bank sources found near %s", absPrimary)
	}
	return sources, nil
}

func buildBankSourceSignature(paths []string) string {
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		parts = append(parts, filepath.Base(path))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func latestBankSourceModTime(paths []string) (time.Time, error) {
	var latest time.Time
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest, nil
}

func loadRawBankEntries(paths []string) ([]rawBankEntry, error) {
	entries := make([]rawBankEntry, 0, 512)
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".json":
			items, err := loadRawBankEntriesFromJSON(path)
			if err != nil {
				return nil, err
			}
			entries = append(entries, items...)
		case ".docx":
			items, err := loadRawBankEntriesFromDocx(path)
			if err != nil {
				return nil, err
			}
			entries = append(entries, items...)
		}
	}
	return entries, nil
}

func loadRawBankEntriesFromJSON(path string) ([]rawBankEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entries []rawBankEntry
	if err := json.Unmarshal(data, &entries); err == nil {
		return entries, nil
	}

	var normalized struct {
		Items []struct {
			Question       string   `json:"question"`
			AnswerTexts    []string `json:"answer_texts"`
			AnswerKeys     []string `json:"answer_keys"`
			CorrectTexts   []string `json:"correct_texts"`
			CorrectOptions []string `json:"correct_options"`
			Options        []struct {
				Key          string `json:"key"`
				Content      string `json:"content"`
				IsCorrect    bool   `json:"isCorrect"`
				IsCorrectAlt bool   `json:"is_correct"`
			} `json:"options"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}

	results := make([]rawBankEntry, 0, len(normalized.Items))
	for _, item := range normalized.Items {
		correctKeySet := map[string]struct{}{}
		for _, key := range item.AnswerKeys {
			key = strings.TrimSpace(strings.ToUpper(key))
			if key != "" {
				correctKeySet[key] = struct{}{}
			}
		}
		for _, key := range item.CorrectOptions {
			key = strings.TrimSpace(strings.ToUpper(key))
			if key != "" {
				correctKeySet[key] = struct{}{}
			}
		}
		options := make([]rawBankOption, 0, len(item.Options))
		for _, option := range item.Options {
			isCorrect := option.IsCorrect || option.IsCorrectAlt
			if !isCorrect {
				_, isCorrect = correctKeySet[strings.TrimSpace(strings.ToUpper(option.Key))]
			}
			options = append(options, rawBankOption{
				Content:   option.Content,
				IsCorrect: isCorrect,
			})
		}
		fallbackAnswerTexts := append([]string(nil), item.AnswerTexts...)
		if len(fallbackAnswerTexts) == 0 {
			fallbackAnswerTexts = append(fallbackAnswerTexts, item.CorrectTexts...)
		}
		if len(options) == 0 && len(fallbackAnswerTexts) > 0 {
			for _, answerText := range fallbackAnswerTexts {
				options = append(options, rawBankOption{
					Content:   answerText,
					IsCorrect: true,
				})
			}
		}
		if strings.TrimSpace(item.Question) == "" || len(options) == 0 {
			continue
		}
		results = append(results, rawBankEntry{
			Question: item.Question,
			Options:  options,
		})
	}
	return results, nil
}

func loadRawBankEntriesFromDocx(path string) ([]rawBankEntry, error) {
	lines, err := extractDocxLines(path)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}

	blocks := make([][]string, 0, 256)
	current := []string{}
	for _, line := range lines {
		if docxQuestionStartRe.MatchString(line) {
			if len(current) > 0 {
				blocks = append(blocks, current)
			}
			current = []string{line}
			continue
		}
		if len(current) == 0 {
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}

	entries := make([]rawBankEntry, 0, len(blocks))
	for _, block := range blocks {
		entry, ok := parseDocxQuestionBlock(block)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func extractDocxLines(path string) ([]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var documentFile *zip.File
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			documentFile = file
			break
		}
	}
	if documentFile == nil {
		return nil, fmt.Errorf("document.xml not found in %s", path)
	}

	handle, err := documentFile.Open()
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	payload, err := ioReadAll(handle)
	if err != nil {
		return nil, err
	}

	text := string(payload)
	text = docxParagraphCloseRe.ReplaceAllString(text, "\n")
	text = docxTagRe.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\u00a0", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	result := make([]string, 0, 512)
	for _, line := range strings.Split(text, "\n") {
		cleaned := normalizeDocxLine(line)
		if cleaned == "" {
			continue
		}
		if cleaned == "窗体底端" || strings.Contains(cleaned, "HYPERLINK ") {
			continue
		}
		result = append(result, cleaned)
	}
	return result, nil
}

func normalizeDocxLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	line = strings.ReplaceAll(line, "　", " ")
	line = strings.Join(strings.Fields(line), " ")
	line = strings.TrimSpace(line)
	return line
}

func parseDocxQuestionBlock(lines []string) (rawBankEntry, bool) {
	joined := strings.Join(lines, " ")
	joined = strings.TrimSpace(joined)
	if joined == "" {
		return rawBankEntry{}, false
	}

	joined = strings.ReplaceAll(joined, "窗体底端", " ")
	joined = strings.Join(strings.Fields(joined), " ")

	questionText := docxQuestionPrefixRe.ReplaceAllString(joined, "")
	optionStart := strings.Index(questionText, "A.")
	if optionStart < 0 {
		optionStart = strings.Index(questionText, "A．")
	}
	if optionStart < 0 {
		optionStart = strings.Index(questionText, "A、")
	}
	if optionStart < 0 {
		return rawBankEntry{}, false
	}

	questionPart := strings.TrimSpace(questionText[:optionStart])
	optionsPart := strings.TrimSpace(questionText[optionStart:])
	answerLetters := ""

	if match := docxEmbeddedAnswerRe.FindStringSubmatch(questionPart); len(match) > 1 {
		answerLetters = match[1]
		questionPart = strings.TrimSpace(docxEmbeddedAnswerRe.ReplaceAllString(questionPart, " "))
	} else if match := docxTrailingAnswerRe.FindStringSubmatch(questionPart); len(match) > 1 {
		candidate := strings.TrimSpace(match[1])
		if candidate != "" && !strings.HasSuffix(questionPart, "______") {
			answerLetters = candidate
			questionPart = strings.TrimSpace(docxTrailingAnswerRe.ReplaceAllString(questionPart, " "))
		}
	}

	options := parseDocxOptions(optionsPart, answerLetters)
	if len(options) < 2 {
		return rawBankEntry{}, false
	}

	return rawBankEntry{
		Question: cleanQuestionText(questionPart),
		Options:  options,
	}, strings.TrimSpace(questionPart) != ""
}

func parseDocxOptions(raw string, answerLetters string) []rawBankOption {
	matches := docxOptionMarkerRe.FindAllStringIndex(raw, -1)
	if len(matches) == 0 {
		return nil
	}

	correctSet := map[string]struct{}{}
	for _, value := range strings.Split(answerLetters, "") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		correctSet[value] = struct{}{}
	}

	options := make([]rawBankOption, 0, len(matches))
	for index, match := range matches {
		key := raw[match[0] : match[0]+1]
		start := match[1]
		end := len(raw)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		content := strings.TrimSpace(raw[start:end])
		if content == "" {
			continue
		}
		options = append(options, rawBankOption{
			Content:   cleanOptionText(content),
			IsCorrect: containsKey(correctSet, key),
		})
	}
	return options
}

func containsKey(items map[string]struct{}, key string) bool {
	_, ok := items[key]
	return ok
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func ioReadAll(reader io.Reader) ([]byte, error) {
	return io.ReadAll(reader)
}
