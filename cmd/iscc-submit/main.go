package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultBaseURL = "https://iscc.isclab.org.cn"
	loginPath      = "/login"
	defaultDBPath  = "data/got0iscc.db"
)

var (
	browserUserAgents = []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
	}
	browserUAIndex uint64
	reNonceInput   = regexp.MustCompile(`(?i)(?:id|name)=["']nonce["'][^>]*value=["']?([^"'\s>]+)`)
)

type trackConfig struct {
	Key                string
	IndexPath          string
	SubmitPathTemplate string
}

type credentials struct {
	AccountName string
	Username    string
	Password    string
}

type submitResult struct {
	Track       string                   `json:"track"`
	ChallengeID string                   `json:"challenge_id"`
	Field       string                   `json:"field"`
	Success     bool                     `json:"success"`
	Message     string                   `json:"message"`
	StatusCode  int                      `json:"status_code"`
	SubmittedAt string                   `json:"submitted_at"`
	Raw         string                   `json:"raw,omitempty"`
	Attempts    []map[string]interface{} `json:"attempts,omitempty"`
}

func main() {
	var (
		trackArg   = flag.String("track", "", "赛道: challenges|arena")
		idsArg     = flag.String("id", "", "题目 ID，多个用逗号分隔")
		flagArg    = flag.String("flag", "", "要提交的 flag")
		accountArg = flag.String("account", "", "数据库中的账号名")
		userArg    = flag.String("username", "", "直接指定用户名")
		passArg    = flag.String("password", "", "直接指定密码")
		dbArg      = flag.String("db", defaultDBPath, "账号数据库路径")
		baseURLArg = flag.String("base-url", defaultBaseURL, "ISCC 基础地址")
		timeoutArg = flag.Duration("timeout", 20*time.Second, "HTTP 超时时间")
		jsonArg    = flag.Bool("json", false, "以 JSON 输出结果")
	)

	flag.Usage = func() {
		prog := filepath.Base(os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "用法:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s --track challenges --id 12 --flag 'iscc{...}' --account Wh1teJ0ker\n", prog)
		fmt.Fprintf(flag.CommandLine.Output(), "  %s --track arena --id 101,102 --flag 'iscc{...}' --username demo --password secret\n\n", prog)
		fmt.Fprintf(flag.CommandLine.Output(), "说明:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  只支持练武题(challenges)和擂台题(arena)。\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  优先使用 --username/--password；未提供时从 --db 指向的 SQLite 中读取账号。\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	track, err := resolveTrack(*trackArg)
	exitIfErr(err)

	challengeIDs := splitCSV(*idsArg)
	if len(challengeIDs) == 0 {
		exitIfErr(errors.New("必须提供 --id"))
	}

	flagValue := strings.TrimSpace(*flagArg)
	if flagValue == "" {
		exitIfErr(errors.New("必须提供 --flag"))
	}

	creds, err := resolveCredentials(*accountArg, *userArg, *passArg, *dbArg)
	exitIfErr(err)

	client, err := newHTTPClient(*timeoutArg)
	exitIfErr(err)

	baseURL := strings.TrimRight(strings.TrimSpace(*baseURLArg), "/")
	if baseURL == "" {
		exitIfErr(errors.New("base-url 不能为空"))
	}

	if err := login(client, baseURL, creds.Username, creds.Password); err != nil {
		exitIfErr(err)
	}

	results := make([]submitResult, 0, len(challengeIDs))
	failures := 0
	for _, id := range challengeIDs {
		result, err := submitChallenge(client, baseURL, track, id, flagValue)
		if err != nil {
			failures++
			results = append(results, submitResult{
				Track:       track.Key,
				ChallengeID: id,
				Success:     false,
				Message:     err.Error(),
				SubmittedAt: nowTS(),
			})
			continue
		}
		if !result.Success {
			failures++
		}
		results = append(results, result)
	}

	if *jsonArg {
		payload := map[string]interface{}{
			"account":      creds.AccountName,
			"username":     creds.Username,
			"track":        track.Key,
			"challenge_ids": challengeIDs,
			"results":      results,
		}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		exitIfErr(err)
		fmt.Println(string(encoded))
	} else {
		fmt.Printf("account: %s\n", firstNonEmpty(creds.AccountName, creds.Username))
		fmt.Printf("username: %s\n", creds.Username)
		fmt.Printf("track: %s\n", track.Key)
		for _, item := range results {
			status := "FAIL"
			if item.Success {
				status = "OK"
			}
			field := item.Field
			if field == "" {
				field = "-"
			}
			fmt.Printf("[%s] id=%s field=%s message=%s\n", status, item.ChallengeID, field, item.Message)
		}
	}

	if failures > 0 {
		os.Exit(1)
	}
}

func resolveTrack(value string) (trackConfig, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "challenges", "challenge", "practice", "练武题":
		return trackConfig{
			Key:                "challenges",
			IndexPath:          "/challenges",
			SubmitPathTemplate: "/chal/{id}",
		}, nil
	case "arena", "擂台", "擂台题":
		return trackConfig{
			Key:                "arena",
			IndexPath:          "/arena",
			SubmitPathTemplate: "/are/{id}",
		}, nil
	default:
		return trackConfig{}, fmt.Errorf("不支持的 track: %q，仅支持 challenges 或 arena", value)
	}
}

func resolveCredentials(accountName string, username string, password string, dbPath string) (credentials, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username != "" || password != "" {
		if username == "" || password == "" {
			return credentials{}, errors.New("--username 和 --password 必须同时提供")
		}
		return credentials{
			AccountName: strings.TrimSpace(accountName),
			Username:    username,
			Password:    password,
		}, nil
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return credentials{}, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	query := `
SELECT name, username, password
FROM accounts
WHERE enabled = 1 AND TRIM(username) <> '' AND TRIM(password) <> ''
`
	args := make([]interface{}, 0, 1)
	accountName = strings.TrimSpace(accountName)
	if accountName != "" {
		query += ` AND name = ?`
		args = append(args, accountName)
	}
	query += ` ORDER BY submit_priority ASC, id ASC LIMIT 1`

	var creds credentials
	if err := db.QueryRow(query, args...).Scan(&creds.AccountName, &creds.Username, &creds.Password); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if accountName != "" {
				return credentials{}, fmt.Errorf("数据库中未找到可用账号: %s", accountName)
			}
			return credentials{}, errors.New("数据库中没有可用账号，请改用 --username/--password")
		}
		return credentials{}, fmt.Errorf("读取账号失败: %w", err)
	}
	return creds, nil
}

func newHTTPClient(timeout time.Duration) (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: timeout,
		Jar:     jar,
	}, nil
}

func login(client *http.Client, baseURL string, username string, password string) error {
	form := url.Values{}
	form.Set("name", username)
	form.Set("password", password)

	req, err := http.NewRequest(http.MethodPost, baseURL+loginPath, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	applyBrowserHeaders(req, baseURL+"/", true)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取登录响应失败: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("登录失败: %s", resp.Status)
	}
	if looksLikeLoginPage(resp.Request.URL.String(), string(body)) {
		return errors.New("登录失败，返回了登录页")
	}
	return nil
}

func submitChallenge(client *http.Client, baseURL string, track trackConfig, challengeID string, flagValue string) (submitResult, error) {
	nonce, err := fetchNonce(client, baseURL, track)
	if err != nil {
		return submitResult{}, err
	}

	submitPath := strings.ReplaceAll(track.SubmitPathTemplate, "{id}", challengeID)
	fields := []string{"key", "flag", "answer", "submission"}
	attempts := make([]map[string]interface{}, 0, len(fields))
	for _, field := range fields {
		form := url.Values{}
		form.Set(field, flagValue)
		if nonce != "" {
			form.Set("nonce", nonce)
		}

		req, err := http.NewRequest(http.MethodPost, baseURL+submitPath, strings.NewReader(form.Encode()))
		if err != nil {
			return submitResult{}, err
		}
		applyBrowserHeaders(req, baseURL+track.IndexPath, true)

		resp, err := client.Do(req)
		if err != nil {
			return submitResult{}, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return submitResult{}, fmt.Errorf("读取提交响应失败: %w", readErr)
		}

		result := interpretSubmitResponse(resp.StatusCode, resp.Request.URL.String(), string(body))
		result["field"] = field
		attempts = append(attempts, result)

		if accepted, _ := result["accepted"].(bool); accepted {
			return submitResult{
				Track:       track.Key,
				ChallengeID: challengeID,
				Field:       field,
				Success:     true,
				Message:     firstNonEmpty(toString(result["message"]), "success"),
				StatusCode:  toInt(result["status"]),
				SubmittedAt: nowTS(),
				Raw:         toString(result["body"]),
				Attempts:    attempts,
			}, nil
		}
		if authErr, _ := result["auth_error"].(bool); authErr {
			return submitResult{}, errors.New("会话失效，请重新执行")
		}
	}

	last := map[string]interface{}{}
	if len(attempts) > 0 {
		last = attempts[len(attempts)-1]
	}
	return submitResult{
		Track:       track.Key,
		ChallengeID: challengeID,
		Success:     false,
		Message:     firstNonEmpty(toString(last["message"]), "提交失败，所有候选字段均未通过"),
		StatusCode:  toInt(last["status"]),
		SubmittedAt: nowTS(),
		Raw:         toString(last["body"]),
		Attempts:    attempts,
	}, nil
}

func fetchNonce(client *http.Client, baseURL string, track trackConfig) (string, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+track.IndexPath, nil)
	if err != nil {
		return "", err
	}
	applyBrowserHeaders(req, baseURL+"/", false)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取 nonce 页面失败: %w", err)
	}
	text := string(body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || looksLikeLoginPage(resp.Request.URL.String(), text) {
		return "", errors.New("会话失效，无法获取 nonce")
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("获取 nonce 失败: %s", resp.Status)
	}

	match := reNonceInput.FindStringSubmatch(text)
	if len(match) == 2 {
		return strings.TrimSpace(match[1]), nil
	}
	return "", errors.New("未找到 nonce")
}

func interpretSubmitResponse(statusCode int, finalURL string, body string) map[string]interface{} {
	text := strings.TrimSpace(body)
	result := map[string]interface{}{
		"status":       statusCode,
		"url":          finalURL,
		"body":         tailText(text, 2000),
		"accepted":     false,
		"auth_error":   false,
		"message":      tailText(text, 200),
		"submitted_at": nowTS(),
	}

	switch text {
	case "-1":
		result["auth_error"] = true
		result["message"] = "unauthorized"
		return result
	case "1":
		result["accepted"] = true
		result["message"] = "success"
		return result
	case "2":
		result["accepted"] = true
		result["message"] = "already_submitted"
		return result
	case "0":
		result["message"] = "failed"
		return result
	}

	var payload interface{}
	if json.Unmarshal([]byte(text), &payload) == nil {
		blob := mustJSON(payload)
		lowered := strings.ToLower(blob)
		if strings.Contains(lowered, `"success"`) ||
			strings.Contains(lowered, `"correct"`) ||
			strings.Contains(lowered, "accepted") ||
			strings.Contains(lowered, "passed") ||
			strings.Contains(lowered, `"true"`) {
			result["accepted"] = true
		}

		var object map[string]interface{}
		if json.Unmarshal([]byte(text), &object) == nil {
			msg := firstNonEmpty(
				toString(object["message"]),
				toString(object["msg"]),
				toString(object["status"]),
				toString(object["result"]),
			)
			if msg != "" {
				result["message"] = msg
			} else {
				result["message"] = tailText(blob, 200)
			}
			if value, ok := object["success"].(bool); ok && value {
				result["accepted"] = true
			}
			if value, ok := object["code"].(float64); ok && int(value) == 1 {
				result["accepted"] = true
			}
			return result
		}

		result["message"] = tailText(blob, 200)
		return result
	}

	lowered := strings.ToLower(text)
	if strings.Contains(lowered, "success") ||
		strings.Contains(lowered, "accepted") ||
		strings.Contains(lowered, "correct") ||
		strings.Contains(lowered, "passed") ||
		strings.Contains(lowered, "already") {
		result["accepted"] = true
	}
	if strings.Contains(lowered, "login") || strings.Contains(lowered, "signin") {
		result["auth_error"] = true
	}
	return result
}

func applyBrowserHeaders(req *http.Request, referer string, isForm bool) {
	if req == nil {
		return
	}
	req.Header.Set("User-Agent", nextBrowserUA())
	if isForm {
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	}
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	referer = strings.TrimSpace(referer)
	if referer != "" {
		req.Header.Set("Referer", referer)
		if origin := originOf(referer); origin != "" {
			req.Header.Set("Origin", origin)
		}
	}
}

func looksLikeLoginPage(urlText string, body string) bool {
	loweredURL := strings.ToLower(strings.TrimSpace(urlText))
	loweredBody := strings.ToLower(body)
	return strings.Contains(loweredURL, "/login") ||
		strings.Contains(loweredBody, `name="password"`) ||
		strings.Contains(loweredBody, `id="password"`) ||
		strings.Contains(loweredBody, `action="/login"`)
}

func nextBrowserUA() string {
	index := atomic.AddUint64(&browserUAIndex, 1) - 1
	return browserUserAgents[index%uint64(len(browserUserAgents))]
}

func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func splitCSV(value string) []string {
	raw := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func tailText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func mustJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func toString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func toInt(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}

func nowTS() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func exitIfErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
