package tasks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"got0iscc/desktop/internal/application/httpx"
	accountdomain "got0iscc/desktop/internal/domain/accounts"
	"got0iscc/desktop/internal/platform/runtime"
	pythonrunner "got0iscc/desktop/internal/platform/sandbox/python"

	xhtml "golang.org/x/net/html"
)

const (
	nativeBaseURL                = "https://iscc.isclab.org.cn"
	nativeLoginPath              = "/login"
	nativeDefaultTimeout         = 60 * time.Second
	nativeDefaultLoginAttempts   = 6
	nativeDefaultLoginRetryDelay = 3 * time.Second
	nativeRequestAttempts        = 6
	nativeCacheFreshTTL          = 2 * time.Minute
	nativeNonceTTL               = 45 * time.Second
	nativeAccountCooldown        = 3 * time.Second
)

var (
	reWhitespace       = regexp.MustCompile(`\s+`)
	reHTMLTag          = regexp.MustCompile(`(?is)<[^>]+>`)
	reRemoteURL        = regexp.MustCompile(`\bhttps?://[^\s"'<>]+`)
	reRemoteAltURL     = regexp.MustCompile(`\b(?:ws|wss|tcp|nc)://[^\s"'<>]+`)
	reRemoteHostPort   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}:\d{1,5}\b`)
	reRemoteDomainPort = regexp.MustCompile(`\b[a-z0-9.-]+\.[a-z]{2,}:\d{1,5}\b`)
	reChallengeLink    = regexp.MustCompile(`/((?:chal|challenge|challenges|arenas|arena))/(\d+)`)
	reNonceInput       = regexp.MustCompile(`(?i)(?:id|name)=["']nonce["'][^>]*value=["']?([^"'\s>]+)`)
	reFlag             = regexp.MustCompile(`(?i)(?:iscc|flag)\{[^\s}]+\}`)
	reFileURLSuffix    = regexp.MustCompile(`(?i)\.(zip|rar|7z|tar|gz|bz2|xz|png|jpg|jpeg|gif|bmp|webp|mp3|wav|mp4|mov|avi|pdf|doc|docx|xls|xlsx|ppt|pptx|txt|md|py|java|c|h|cc|cpp|hpp|go|rs|js|html|css|json|yaml|yml|xml|csv|apk|ipa|so|dylib|dll|exe|bin|jar|class|dex|wasm|elf|o|obj|ko|sys|dat|db|sqlite|pcap|pcapng|cap|iso|img|dmg|msi|sh|ps1|bat)(?:\?.*)?$`)
	reFileRef          = regexp.MustCompile(`(?i)(?:https?://[^\s"'<>),]+|/?static/uploads/[^\s"'<>),]+|/?uploads/[^\s"'<>),]+|/(?:files?|downloads?|attachments?)/[^\s"'<>),]+)`)
	reMeaningfulTitle  = regexp.MustCompile(`(?i)^(?:challenge|challenges|chal|task|problem|题目)[ _-]*\d+$`)
)

type ChallengeRepository interface {
	DB() *sql.DB
}

type pythonTaskRunner interface {
	Run(ctx context.Context, req pythonrunner.RunRequest) (pythonrunner.RunResult, error)
}

type nativeTaskService struct {
	layout        runtime.Layout
	accountRepo   AccountRepository
	challengeRepo ChallengeRepository
	runner        pythonTaskRunner
	pythonBinary  string
	sessionMu     sync.Mutex
	sessions      map[string]*taskHTTPClient
	networkProxy  NetworkProxySettings
}

type nativeTaskResult struct {
	Tail       string
	ReturnCode int
}

type taskLogger interface {
	Printf(format string, args ...any)
}

type sectionConfig struct {
	Name                   string
	IndexPage              string
	ListEndpoint           string
	DetailEndpointTemplate string
	DetailAPIEndpoint      string
	SubmitEndpointTemplate string
	SolvesEndpoint         string
	RealtimeKeywords       []string
}

type challengeRecord struct {
	Key                string
	ChallengeID        string
	Section            string
	Title              string
	Category           string
	ChallengeKind      string
	ExpectsAttachments bool
	ExpectsRemote      bool
	AssetWarnings      []string
	DirName            string
	DirPath            string
	DetailURL          string
	DescriptionPath    string
	DescriptionSHA256  string
	RemoteSummaryPath  string
	RemoteSHA256       string
	RemoteTargets      []remoteTarget
	Fingerprint        string
	Changed            bool
	Attachments        []attachmentRecord
	UpdatedAt          string
}

type challengeAccountRecord struct {
	ChallengeKey string
	AccountName  string
	Data         map[string]any
	UpdatedAt    string
}

type attachmentRecord struct {
	Name        string `json:"name"`
	StoredName  string `json:"stored_name"`
	URL         string `json:"url"`
	LocalPath   string `json:"local_path"`
	SharedPath  string `json:"shared_path"`
	StorageMode string `json:"storage_mode"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Changed     bool   `json:"changed"`
}

type remoteTarget struct {
	Value  string `json:"value"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}

type remoteSolveInfo struct {
	ChallengeID string `json:"challenge_id"`
	Title       string `json:"title"`
	Category    string `json:"category"`
	SubmittedAt string `json:"submitted_at"`
	Value       any    `json:"value"`
	SolveID     any    `json:"solve_id"`
	TeamID      any    `json:"team_id"`
}

type fetchedChallenge struct {
	ChallengeID        string
	Section            string
	Title              string
	Category           string
	Tags               []string
	ChallengeKind      string
	ExpectsAttachments bool
	ExpectsRemote      bool
	DetailURL          string
	DescriptionHTML    string
	DescriptionText    string
	DescriptionSHA256  string
	RemoteTargets      []remoteTarget
	RemoteSHA256       string
	Fingerprint        string
	Attachments        []attachmentRecord
	AssetWarnings      []string
	DirName            string
	DirPath            string
	DescriptionPath    string
	RemoteSummaryPath  string
	Changed            bool
}

type solverResult struct {
	Status            string   `json:"status"`
	ReturnCode        int      `json:"returncode,omitempty"`
	Entrypoint        string   `json:"entrypoint,omitempty"`
	SolverFile        string   `json:"solver_file,omitempty"`
	Python            string   `json:"python,omitempty"`
	TriedInterpreters []string `json:"tried_interpreters,omitempty"`
	Flag              string   `json:"flag,omitempty"`
	Fingerprint       string   `json:"fingerprint,omitempty"`
	RanAt             string   `json:"ran_at,omitempty"`
	StdoutTail        string   `json:"stdout_tail,omitempty"`
	StderrTail        string   `json:"stderr_tail,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	SourceAccount     string   `json:"source_account,omitempty"`
}

type taskHTTPClient struct {
	client          *http.Client
	baseURL         string
	cookiePath      string
	tokenPath       string
	headers         httpx.BrowserHeaderProfile
	loginAttempts   int
	loginRetryDelay time.Duration
	cookieMu        sync.Mutex
	nonceMu         sync.Mutex
	lastNonce       map[string]cachedNonce
}

type cachedNonce struct {
	Value     string
	ExpiresAt time.Time
}

type accountSessionFiles struct {
	Dir        string
	CookiePath string
	TokenPath  string
}

type sharedSolution struct {
	Flag          string
	SourceAccount string
	SourceState   map[string]any
}

var nativeSections = map[string]sectionConfig{
	"challenges": {
		Name:                   "challenges",
		IndexPage:              "/challenges",
		ListEndpoint:           "/chals",
		DetailEndpointTemplate: "/chal/{id}",
		DetailAPIEndpoint:      "/chals/{id}",
		SubmitEndpointTemplate: "/chal/{id}",
		SolvesEndpoint:         "/solves",
		RealtimeKeywords:       []string{"web", "pwn"},
	},
	"arena": {
		Name:                   "arena",
		IndexPage:              "/arena",
		ListEndpoint:           "/arenas",
		DetailEndpointTemplate: "/arenas/{id}",
		DetailAPIEndpoint:      "/arenas/{id}",
		SubmitEndpointTemplate: "/are/{id}",
		SolvesEndpoint:         "/arenasolves",
		RealtimeKeywords:       []string{"web", "pwn"},
	},
}

const solverBootstrap = `
import importlib.util
import inspect
import json
import os
import sys
from pathlib import Path

solver_path = Path(sys.argv[1]).resolve()
spec = importlib.util.spec_from_file_location("got0iscc_solver_entry", solver_path)
module = importlib.util.module_from_spec(spec) if spec and spec.loader else None

if module is None or spec is None or spec.loader is None:
    raise SystemExit("unable to load solver module")

spec.loader.exec_module(module)
submit_flag = getattr(module, "submit_flag", None)
used_submit_flag = False
default_attachment = os.environ.get("AUTO_PRIMARY_ATTACHMENT", "").strip()
default_remote = os.environ.get("AUTO_REMOTE_FILE", "").strip()

if callable(submit_flag):
    try:
        signature = inspect.signature(submit_flag)
        accepts_no_args = all(
            parameter.kind in (inspect.Parameter.VAR_POSITIONAL, inspect.Parameter.VAR_KEYWORD)
            or parameter.default is not inspect._empty
            for parameter in signature.parameters.values()
        )
    except Exception:
        accepts_no_args = False
    if accepts_no_args:
        used_submit_flag = True
        result = submit_flag()
        print("__AUTO_RESULT__=" + json.dumps({"flag": result}, ensure_ascii=False))
if not used_submit_flag:
    main = getattr(module, "main", None)
    if callable(main):
        extra_args = sys.argv[2:]
        if not extra_args:
            if default_attachment:
                extra_args = [default_attachment]
            elif default_remote:
                extra_args = [default_remote]
        original_argv = sys.argv[:]
        sys.argv = [str(solver_path)] + list(extra_args)
        main()
        sys.argv = original_argv
    else:
        raise SystemExit("submit_flag() not found")
`

func newNativeTaskService(layout runtime.Layout, accountRepo AccountRepository, challengeRepo ChallengeRepository, runner pythonTaskRunner, pythonBinary string) *nativeTaskService {
	return &nativeTaskService{
		layout:        layout,
		accountRepo:   accountRepo,
		challengeRepo: challengeRepo,
		runner:        runner,
		pythonBinary:  strings.TrimSpace(pythonBinary),
		sessions:      map[string]*taskHTTPClient{},
	}
}

func (s *nativeTaskService) Execute(ctx context.Context, req StartRequest, logger taskLogger) nativeTaskResult {
	command := strings.TrimSpace(req.Command)
	if command == "" {
		command = "solve"
	}

	switch command {
	case "status":
		return s.runStatus(ctx, req, logger)
	case "submit-flag":
		return s.runSubmitFlag(ctx, req, logger)
	case "sync":
		return s.runSync(ctx, req, logger)
	case "solve":
		return s.runSolve(ctx, req, logger)
	default:
		logger.Printf("不支持的任务命令: %s", command)
		return nativeTaskResult{Tail: "不支持的任务命令", ReturnCode: 2}
	}
}

func (s *nativeTaskService) runStatus(ctx context.Context, req StartRequest, logger taskLogger) nativeTaskResult {
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err != nil {
		logger.Printf("读取账号失败: %v", err)
		return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
	}
	selectedAccounts := filterAccounts(accounts, req.Account)
	sections := selectedSections(req.Section)
	if len(selectedAccounts) == 0 {
		return nativeTaskResult{Tail: "没有可用账号", ReturnCode: 2}
	}
	if len(sections) == 0 {
		return nativeTaskResult{Tail: "没有可用赛道", ReturnCode: 2}
	}

	db := s.challengeRepo.DB()
	if db == nil {
		err = errors.New("挑战缓存数据库不可用")
		logger.Printf("%v", err)
		return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
	}

	ids := parseIDSet(req.IDs)
	totalPending := 0
	totalSolved := 0
	var lastErr error
	for _, account := range selectedAccounts {
		if err := waitAccountCooldown(ctx, logger, account.Name); err != nil {
			return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
		}
		client, loginErr := s.loginAccount(ctx, account)
		if loginErr != nil {
			lastErr = loginErr
			logger.Printf("%s login error=%v", account.Name, loginErr)
			_ = s.updateRuntimeState(ctx, account, "error", loginErr.Error(), false)
			continue
		}
		_ = s.updateRuntimeState(ctx, account, "ok", "", true)

		accountProcessed := 0
		for _, section := range sections {
			if remoteSyncRecentlyFresh(account, section) {
				pending, solved, scanErr := accountStatusCounts(ctx, db, account.Name, section.Name, ids)
				if scanErr != nil {
					lastErr = scanErr
					logger.Printf("%s %s status cached-count error=%v", account.Name, section.Name, scanErr)
					continue
				}
				totalPending += pending
				totalSolved += solved
				logger.Printf("%s %s remote skipped=fresh-cache pending=%d solved=%d", account.Name, section.Name, pending, solved)
				continue
			}
			_, challenges, solvedMap, syncErr := s.syncAccountSectionWithRetry(ctx, db, client, account, section, req, logger)
			if syncErr != nil {
				lastErr = syncErr
				logger.Printf("%s %s status remote sync error=%v", account.Name, section.Name, syncErr)
				continue
			}
			accountProcessed += challenges
			logger.Printf("%s %s remote synced=%d solved=%d", account.Name, section.Name, challenges, len(solvedMap))

			pending, solved, scanErr := accountStatusCounts(ctx, db, account.Name, section.Name, ids)
			if scanErr != nil {
				lastErr = scanErr
				logger.Printf("%s %s status error=%v", account.Name, section.Name, scanErr)
				continue
			}
			totalPending += pending
			totalSolved += solved
			logger.Printf("%s %s pending=%d solved=%d", account.Name, section.Name, pending, solved)
		}
		_ = s.saveProcessedCounters(ctx, account, accountProcessed, len(sections))
	}
	if totalPending == 0 && totalSolved == 0 && lastErr != nil {
		return nativeTaskResult{Tail: lastErr.Error(), ReturnCode: 1}
	}
	tail := fmt.Sprintf("pending_total=%d solved_total=%d", totalPending, totalSolved)
	logger.Printf(tail)
	return nativeTaskResult{Tail: tail, ReturnCode: 0}
}

func (s *nativeTaskService) runSubmitFlag(ctx context.Context, req StartRequest, logger taskLogger) nativeTaskResult {
	flagValue := strings.TrimSpace(req.Flag)
	if flagValue == "" {
		return nativeTaskResult{Tail: "flag 不能为空", ReturnCode: 2}
	}

	ids := parseIDList(req.IDs)
	if len(ids) == 0 {
		return nativeTaskResult{Tail: "题目 ID 不能为空", ReturnCode: 2}
	}
	sections := selectedSections(req.Section)
	if len(sections) == 0 {
		return nativeTaskResult{Tail: "没有可用赛道", ReturnCode: 2}
	}
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err != nil {
		logger.Printf("读取账号失败: %v", err)
		return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
	}
	selectedAccounts := filterAccounts(accounts, req.Account)
	if len(selectedAccounts) == 0 {
		return nativeTaskResult{Tail: "没有可用账号", ReturnCode: 2}
	}
	db := s.challengeRepo.DB()
	if db == nil {
		return nativeTaskResult{Tail: "挑战缓存数据库不可用", ReturnCode: 1}
	}

	successCount := 0
	failureCount := 0
	for _, section := range sections {
		for _, account := range selectedAccounts {
			if err := waitAccountCooldown(ctx, logger, account.Name); err != nil {
				return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
			}
			client, loginErr := s.loginAccount(ctx, account)
			if loginErr != nil {
				logger.Printf("%s %s login error=%v", account.Name, section.Name, loginErr)
				failureCount += len(ids)
				_ = s.updateRuntimeState(ctx, account, "error", loginErr.Error(), false)
				continue
			}
			_ = s.updateRuntimeState(ctx, account, "ok", "", true)

			client, challenges, solvedMap, syncErr := s.syncAccountSectionWithRetry(ctx, db, client, account, section, req, logger)
			if syncErr != nil {
				logger.Printf("%s %s pre-submit remote sync error=%v", account.Name, section.Name, syncErr)
			} else {
				logger.Printf("%s %s pre-submit synced=%d solved=%d", account.Name, section.Name, challenges, len(solvedMap))
			}

			for _, challengeID := range ids {
				record, recErr := s.loadChallengeRecord(ctx, db, section.Name, challengeID)
				if recErr != nil {
					logger.Printf("%s %s #%s challenge load error=%v", account.Name, section.Name, challengeID, recErr)
					failureCount++
					continue
				}
				var result map[string]any
				var submitErr error
				client, result, submitErr = s.submitFlagWithRetry(ctx, client, account, record, flagValue, true, logger)
				if submitErr != nil {
					logger.Printf("%s %s #%s submit error=%v", account.Name, section.Name, challengeID, submitErr)
					failureCount++
					continue
				}
				logger.Printf("%s %s #%s accepted=%v message=%s", account.Name, section.Name, challengeID, boolValue(result["accepted"]), firstString(result["message"]))
				if boolValue(result["accepted"]) {
					successCount++
				} else {
					failureCount++
				}
			}
			if solvedMap, syncErr := s.refreshPlatformSubmissions(ctx, client, section); syncErr != nil {
				logger.Printf("%s %s post-submit solves sync error=%v", account.Name, section.Name, syncErr)
			} else if err := s.applyPlatformSubmissions(ctx, db, account, section, solvedMap); err != nil {
				logger.Printf("%s %s post-submit cache update error=%v", account.Name, section.Name, err)
			} else {
				_ = s.updateRemoteSubmissionState(ctx, account, len(solvedMap))
				logger.Printf("%s %s post-submit solved=%d", account.Name, section.Name, len(solvedMap))
			}
		}
	}

	if failureCount > 0 && successCount == 0 {
		return nativeTaskResult{Tail: fmt.Sprintf("submit failed total=%d", failureCount), ReturnCode: 1}
	}
	return nativeTaskResult{Tail: fmt.Sprintf("submit ok=%d failed=%d", successCount, failureCount), ReturnCode: 0}
}

func (s *nativeTaskService) runSync(ctx context.Context, req StartRequest, logger taskLogger) nativeTaskResult {
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err != nil {
		logger.Printf("读取账号失败: %v", err)
		return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
	}
	selectedAccounts := filterAccounts(accounts, req.Account)
	if len(selectedAccounts) == 0 {
		return nativeTaskResult{Tail: "没有可用账号", ReturnCode: 2}
	}
	sections := selectedSections(req.Section)
	if len(sections) == 0 {
		return nativeTaskResult{Tail: "没有可用赛道", ReturnCode: 2}
	}
	db := s.challengeRepo.DB()
	if db == nil {
		return nativeTaskResult{Tail: "挑战缓存数据库不可用", ReturnCode: 1}
	}

	workers := normalizedWorkers(req.Workers)
	logger.Printf("同步资产并发 workers=%d accounts=%d sections=%d force_download=%v", workers, len(selectedAccounts), len(sections), req.ForceDownload)

	var lastErr error
	totalChallenges := 0
	for _, account := range selectedAccounts {
		if err := waitAccountCooldown(ctx, logger, account.Name); err != nil {
			return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
		}
		client, loginErr := s.loginAccount(ctx, account)
		if loginErr != nil {
			lastErr = loginErr
			logger.Printf("%s login error=%v", account.Name, loginErr)
			_ = s.updateRuntimeState(ctx, account, "error", loginErr.Error(), false)
			continue
		}
		_ = s.updateRuntimeState(ctx, account, "ok", "", true)

		accountProcessed := 0
		for _, section := range sections {
			_, challenges, solvedMap, syncErr := s.syncAccountSectionWithRetry(ctx, db, client, account, section, req, logger)
			if syncErr != nil {
				lastErr = syncErr
				logger.Printf("%s %s sync error=%v", account.Name, section.Name, syncErr)
				continue
			}
			accountProcessed += challenges
			totalChallenges += challenges
			logger.Printf("%s %s synced=%d solved=%d", account.Name, section.Name, challenges, len(solvedMap))
		}
		_ = s.saveProcessedCounters(ctx, account, accountProcessed, len(sections))
	}

	if totalChallenges == 0 && lastErr != nil {
		return nativeTaskResult{Tail: lastErr.Error(), ReturnCode: 1}
	}
	return nativeTaskResult{Tail: fmt.Sprintf("synced_total=%d", totalChallenges), ReturnCode: 0}
}

func (s *nativeTaskService) runSolve(ctx context.Context, req StartRequest, logger taskLogger) nativeTaskResult {
	accounts, err := s.accountRepo.ListAccounts(ctx)
	if err != nil {
		logger.Printf("读取账号失败: %v", err)
		return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
	}
	selectedAccounts := filterAccounts(accounts, req.Account)
	if len(selectedAccounts) == 0 {
		return nativeTaskResult{Tail: "没有可用账号", ReturnCode: 2}
	}
	sections := selectedSections(req.Section)
	if len(sections) == 0 {
		return nativeTaskResult{Tail: "没有可用赛道", ReturnCode: 2}
	}
	db := s.challengeRepo.DB()
	if db == nil {
		return nativeTaskResult{Tail: "挑战缓存数据库不可用", ReturnCode: 1}
	}

	totalSolved := 0
	totalSubmitted := 0
	totalSkipped := 0
	totalErrors := 0
	var lastErr error
	idSet := parseIDSet(req.IDs)

	for _, account := range selectedAccounts {
		if err := waitAccountCooldown(ctx, logger, account.Name); err != nil {
			return nativeTaskResult{Tail: err.Error(), ReturnCode: 1}
		}
		client, loginErr := s.loginAccount(ctx, account)
		if loginErr != nil {
			lastErr = loginErr
			logger.Printf("%s login error=%v", account.Name, loginErr)
			_ = s.updateRuntimeState(ctx, account, "error", loginErr.Error(), false)
			continue
		}
		_ = s.updateRuntimeState(ctx, account, "ok", "", true)

		for _, section := range sections {
			var syncErr error
			if remoteSyncRecentlyFresh(account, section) && !req.ForceDownload {
				logger.Printf("%s %s pre-sync skipped=fresh-cache", account.Name, section.Name)
			} else {
				client, _, _, syncErr = s.syncAccountSectionWithRetry(ctx, db, client, account, section, StartRequest{
					Section:       section.Name,
					IDs:           req.IDs,
					ForceDownload: req.ForceDownload,
				}, logger)
				if syncErr != nil {
					lastErr = syncErr
					logger.Printf("%s %s pre-sync error=%v", account.Name, section.Name, syncErr)
					continue
				}
			}

			challenges, loadErr := s.loadSectionChallenges(ctx, db, section.Name, idSet)
			if loadErr != nil {
				lastErr = loadErr
				logger.Printf("%s %s load challenges error=%v", account.Name, section.Name, loadErr)
				continue
			}
			sort.SliceStable(challenges, func(i, j int) bool {
				return numericPrefix(challenges[i].ChallengeID) < numericPrefix(challenges[j].ChallengeID)
			})

			for _, challenge := range challenges {
				state, stateErr := s.loadChallengeAccountState(ctx, db, challenge.Key, account.Name)
				if stateErr != nil {
					lastErr = stateErr
					totalErrors++
					logger.Printf("%s %s #%s load state error=%v", account.Name, section.Name, challenge.ChallengeID, stateErr)
					continue
				}
				if !req.ForceDownload && challengeNeedsResync(challenge) {
					logger.Printf("%s %s #%s pre-sync stale-local-assets dir=%s", account.Name, section.Name, challenge.ChallengeID, challenge.DirPath)
					client, _, _, syncErr = s.syncAccountSectionWithRetry(ctx, db, client, account, section, StartRequest{
						Section:       section.Name,
						IDs:           challenge.ChallengeID,
						ForceDownload: true,
					}, logger)
					if syncErr != nil {
						lastErr = syncErr
						totalErrors++
						logger.Printf("%s %s #%s pre-sync resync error=%v", account.Name, section.Name, challenge.ChallengeID, syncErr)
						continue
					}
					challenge, stateErr = s.loadChallengeRecord(ctx, db, section.Name, challenge.ChallengeID)
					if stateErr != nil {
						lastErr = stateErr
						totalErrors++
						logger.Printf("%s %s #%s reload challenge error=%v", account.Name, section.Name, challenge.ChallengeID, stateErr)
						continue
					}
				}
				if !req.ForceDownload && challengeHasPerAccountAssets(challenge) && !submissionConfirmed(state) {
					logger.Printf("%s %s #%s pre-sync account-attachment-refresh", account.Name, section.Name, challenge.ChallengeID)
					client, _, _, syncErr = s.syncAccountSectionWithRetry(ctx, db, client, account, section, StartRequest{
						Section:       section.Name,
						IDs:           challenge.ChallengeID,
						ForceDownload: true,
					}, logger)
					if syncErr != nil {
						lastErr = syncErr
						totalErrors++
						logger.Printf("%s %s #%s attachment-refresh error=%v", account.Name, section.Name, challenge.ChallengeID, syncErr)
						continue
					}
					challenge, stateErr = s.loadChallengeRecord(ctx, db, section.Name, challenge.ChallengeID)
					if stateErr != nil {
						lastErr = stateErr
						totalErrors++
						logger.Printf("%s %s #%s reload challenge after attachment-refresh error=%v", account.Name, section.Name, challenge.ChallengeID, stateErr)
						continue
					}
					state, stateErr = s.loadChallengeAccountState(ctx, db, challenge.Key, account.Name)
					if stateErr != nil {
						lastErr = stateErr
						totalErrors++
						logger.Printf("%s %s #%s reload state after attachment-refresh error=%v", account.Name, section.Name, challenge.ChallengeID, stateErr)
						continue
					}
				}
				shared, sharedErr := s.findSharedSolution(ctx, db, account.Name, challenge, section.RealtimeKeywords)
				if sharedErr != nil {
					lastErr = sharedErr
					totalErrors++
					logger.Printf("%s %s #%s shared-solution error=%v", account.Name, section.Name, challenge.ChallengeID, sharedErr)
				} else if shared.Flag != "" {
					var adopted bool
					state, adopted = adoptSharedSolutionState(state, challenge, shared)
					if adopted {
						if err := s.upsertChallengeAccounts(ctx, db, []challengeAccountRecord{{
							ChallengeKey: challenge.Key,
							AccountName:  account.Name,
							Data:         state,
							UpdatedAt:    nowTS(),
						}}); err != nil {
							lastErr = err
							logger.Printf("%s %s #%s shared-solution save error=%v", account.Name, section.Name, challenge.ChallengeID, err)
						} else {
							totalSolved++
							logger.Printf("%s %s #%s solver status=reused source=%s flag=%s", account.Name, section.Name, challenge.ChallengeID, shared.SourceAccount, displayFlag(shared.Flag))
						}
					}
				}
				if !req.ForceSolve && !shouldSolve(state, challenge.Fingerprint, challenge.ChallengeKind, section.RealtimeKeywords) {
					totalSkipped++
					flagValue := strings.TrimSpace(firstString(state["last_flag"]))
					if req.NoSubmit || flagValue == "" {
						logger.Printf("%s %s #%s solve skipped reason=%s", account.Name, section.Name, challenge.ChallengeID, solveSkipReason(state, challenge, section.RealtimeKeywords))
						continue
					}
					var submitResult map[string]any
					client, submitResult, stateErr = s.submitFlagWithRetry(ctx, client, account, challenge, flagValue, false, logger)
					if stateErr != nil {
						lastErr = stateErr
						totalErrors++
						logger.Printf("%s %s #%s submit error=%v", account.Name, section.Name, challenge.ChallengeID, stateErr)
						continue
					}
					if boolValue(submitResult["accepted"]) || firstString(submitResult["message"]) == "already_submitted" {
						totalSubmitted++
					}
					logger.Printf("%s %s #%s submit accepted=%v message=%s", account.Name, section.Name, challenge.ChallengeID, boolValue(submitResult["accepted"]), firstString(submitResult["message"]))
					continue
				}

				flagValue, solver, solveErr := s.runChallengeSolver(ctx, client, account, challenge)
				if solveErr != nil {
					lastErr = solveErr
					totalErrors++
					logger.Printf("%s %s #%s solver error=%v", account.Name, section.Name, challenge.ChallengeID, solveErr)
					state = updateSolverState(state, challenge, solver, "")
					_ = s.upsertChallengeAccounts(ctx, db, []challengeAccountRecord{{
						ChallengeKey: challenge.Key,
						AccountName:  account.Name,
						Data:         state,
						UpdatedAt:    nowTS(),
					}})
					continue
				}
				state = updateSolverState(state, challenge, solver, flagValue)
				_ = s.upsertChallengeAccounts(ctx, db, []challengeAccountRecord{{
					ChallengeKey: challenge.Key,
					AccountName:  account.Name,
					Data:         state,
					UpdatedAt:    nowTS(),
				}})
				totalSolved++
				logger.Printf("%s %s #%s solver status=%s flag=%s", account.Name, section.Name, challenge.ChallengeID, solver.Status, displayFlag(flagValue))

				if req.NoSubmit || strings.TrimSpace(flagValue) == "" {
					continue
				}
				var submitResult map[string]any
				var submitErr error
				client, submitResult, submitErr = s.submitFlagWithRetry(ctx, client, account, challenge, flagValue, false, logger)
				if submitErr != nil {
					lastErr = submitErr
					totalErrors++
					logger.Printf("%s %s #%s submit error=%v", account.Name, section.Name, challenge.ChallengeID, submitErr)
					continue
				}
				if boolValue(submitResult["accepted"]) || firstString(submitResult["message"]) == "already_submitted" {
					totalSubmitted++
				}
				logger.Printf("%s %s #%s submit accepted=%v message=%s", account.Name, section.Name, challenge.ChallengeID, boolValue(submitResult["accepted"]), firstString(submitResult["message"]))
			}
		}
	}

	if totalSolved == 0 && lastErr != nil {
		return nativeTaskResult{Tail: lastErr.Error(), ReturnCode: 1}
	}
	return nativeTaskResult{
		Tail:       fmt.Sprintf("solved=%d submitted=%d skipped=%d errors=%d", totalSolved, totalSubmitted, totalSkipped, totalErrors),
		ReturnCode: 0,
	}
}

func (s *nativeTaskService) syncAccountSection(ctx context.Context, db *sql.DB, client *taskHTTPClient, account accountdomain.Account, section sectionConfig, req StartRequest, logger taskLogger) (int, map[string]remoteSolveInfo, error) {
	items, err := s.fetchChallenges(ctx, client, section)
	if err != nil {
		return 0, nil, err
	}
	idSet := parseIDSet(req.IDs)
	filtered := make([]fetchedChallenge, 0, len(items))
	for _, item := range items {
		if len(idSet) > 0 && !idSet[item.ChallengeID] {
			continue
		}
		filtered = append(filtered, item)
	}
	challengeRows, accountRows := s.syncChallengesConcurrently(ctx, db, client, account, section, filtered, req, logger)
	if err := s.upsertChallenges(ctx, db, challengeRows); err != nil {
		return 0, nil, err
	}
	solvedMap, solveErr := s.refreshPlatformSubmissions(ctx, client, section)
	if solveErr != nil {
		logger.Printf("%s %s solves sync error=%v", account.Name, section.Name, solveErr)
	} else {
		_ = s.updateRemoteSubmissionState(ctx, account, len(solvedMap))
	}
	applySolvedMapToRows(accountRows, section, solvedMap)
	if err := s.upsertChallengeAccounts(ctx, db, accountRows); err != nil {
		return 0, nil, err
	}
	return len(challengeRows), solvedMap, nil
}

func (s *nativeTaskService) syncAccountSectionWithRetry(ctx context.Context, db *sql.DB, client *taskHTTPClient, account accountdomain.Account, section sectionConfig, req StartRequest, logger taskLogger) (*taskHTTPClient, int, map[string]remoteSolveInfo, error) {
	count, solvedMap, err := s.syncAccountSection(ctx, db, client, account, section, req, logger)
	if err == nil || !isSessionExpiredError(err) {
		return client, count, solvedMap, err
	}
	logger.Printf("%s %s session expired during remote sync, relogin and retry", account.Name, section.Name)
	refreshed, loginErr := s.reloginAccount(ctx, account)
	if loginErr != nil {
		return client, count, solvedMap, loginErr
	}
	count, solvedMap, err = s.syncAccountSection(ctx, db, refreshed, account, section, req, logger)
	return refreshed, count, solvedMap, err
}

func applySolvedMapToRows(rows []challengeAccountRecord, section sectionConfig, solvedMap map[string]remoteSolveInfo) {
	for i := range rows {
		challengeID := firstString(rows[i].Data["challenge_id"], strings.TrimPrefix(rows[i].ChallengeKey, section.Name+":"))
		if info, ok := solvedMap[challengeID]; ok {
			rows[i].Data["platform_solved"] = true
			rows[i].Data["platform_solved_at"] = info.SubmittedAt
			rows[i].Data["platform_submission"] = info
			rows[i].Data["last_submit_ok"] = true
			if info.SubmittedAt != "" && firstString(rows[i].Data["last_submitted_at"]) == "" {
				rows[i].Data["last_submitted_at"] = info.SubmittedAt
			}
		}
	}
}

func (s *nativeTaskService) applyPlatformSubmissions(ctx context.Context, db *sql.DB, account accountdomain.Account, section sectionConfig, solvedMap map[string]remoteSolveInfo) error {
	if len(solvedMap) == 0 {
		return nil
	}
	rows := make([]challengeAccountRecord, 0, len(solvedMap))
	for challengeID, info := range solvedMap {
		record, err := s.loadChallengeRecord(ctx, db, section.Name, challengeID)
		if err != nil {
			continue
		}
		state, err := s.loadChallengeAccountState(ctx, db, record.Key, account.Name)
		if err != nil {
			continue
		}
		state["challenge_id"] = record.ChallengeID
		state["section"] = record.Section
		state["title"] = record.Title
		state["category"] = record.Category
		state["challenge_kind"] = record.ChallengeKind
		state["platform_solved"] = true
		state["platform_solved_at"] = info.SubmittedAt
		state["platform_submission"] = info
		state["last_submit_ok"] = true
		if info.SubmittedAt != "" && firstString(state["last_submitted_at"]) == "" {
			state["last_submitted_at"] = info.SubmittedAt
		}
		rows = append(rows, challengeAccountRecord{
			ChallengeKey: record.Key,
			AccountName:  account.Name,
			Data:         state,
			UpdatedAt:    nowTS(),
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return s.upsertChallengeAccounts(ctx, db, rows)
}

type syncChallengeResult struct {
	ChallengeRow challengeRecord
	AccountRow   challengeAccountRecord
	OK           bool
}

func (s *nativeTaskService) syncChallengesConcurrently(ctx context.Context, db *sql.DB, client *taskHTTPClient, account accountdomain.Account, section sectionConfig, items []fetchedChallenge, req StartRequest, logger taskLogger) ([]challengeRecord, []challengeAccountRecord) {
	if len(items) == 0 {
		return nil, nil
	}
	workers := normalizedWorkers(req.Workers)
	if workers > len(items) {
		workers = len(items)
	}
	jobs := make(chan fetchedChallenge)
	results := make(chan syncChallengeResult, len(items))
	var wg sync.WaitGroup
	for workerID := 0; workerID < workers; workerID++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				results <- s.syncSingleChallenge(ctx, db, client, account, section, item, req, logger)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, item := range items {
			select {
			case <-ctx.Done():
				return
			case jobs <- item:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	challengeRows := make([]challengeRecord, 0, len(items))
	accountRows := make([]challengeAccountRecord, 0, len(items))
	for result := range results {
		if !result.OK {
			continue
		}
		challengeRows = append(challengeRows, result.ChallengeRow)
		accountRows = append(accountRows, result.AccountRow)
	}
	sort.SliceStable(challengeRows, func(i, j int) bool {
		return challengeRecordLess(challengeRows[i].ChallengeID, challengeRows[j].ChallengeID, challengeRows[i].Key, challengeRows[j].Key)
	})
	sort.SliceStable(accountRows, func(i, j int) bool {
		return challengeRecordLess(strings.TrimPrefix(accountRows[i].ChallengeKey, section.Name+":"), strings.TrimPrefix(accountRows[j].ChallengeKey, section.Name+":"), accountRows[i].ChallengeKey, accountRows[j].ChallengeKey)
	})
	return challengeRows, accountRows
}

func (s *nativeTaskService) syncSingleChallenge(ctx context.Context, db *sql.DB, client *taskHTTPClient, account accountdomain.Account, section sectionConfig, item fetchedChallenge, req StartRequest, logger taskLogger) syncChallengeResult {
	detailed, syncErr := s.fetchChallengeDetail(ctx, client, section, item)
	if syncErr != nil {
		logger.Printf("%s %s #%s detail error=%v", account.Name, section.Name, item.ChallengeID, syncErr)
		return syncChallengeResult{}
	}
	manifest, forceErr := s.materializeChallenge(ctx, client, detailed, req.ForceDownload)
	if forceErr != nil {
		logger.Printf("%s %s #%s materialize error=%v", account.Name, section.Name, item.ChallengeID, forceErr)
		return syncChallengeResult{}
	}
	if len(manifest.Attachments) > 0 {
		logger.Printf("%s %s #%s attachments=%d", account.Name, section.Name, item.ChallengeID, len(manifest.Attachments))
	}
	entry, loadErr := s.loadChallengeAccountState(ctx, db, manifest.Key, account.Name)
	if loadErr != nil {
		logger.Printf("%s %s #%s load state error=%v", account.Name, section.Name, item.ChallengeID, loadErr)
		entry = map[string]any{}
	}
	entry, resetNeeded, previousSignature, currentSignature := reconcileChallengeState(entry, manifest)
	if resetNeeded {
		logger.Printf("%s %s #%s state-reset reason=attachment_signature_mismatch old=%s new=%s", account.Name, section.Name, item.ChallengeID, firstN(previousSignature, 12), firstN(currentSignature, 12))
	}
	return syncChallengeResult{
		ChallengeRow: manifest,
		AccountRow: challengeAccountRecord{
			ChallengeKey: manifest.Key,
			AccountName:  account.Name,
			Data:         entry,
			UpdatedAt:    nowTS(),
		},
		OK: true,
	}
}

func (s *nativeTaskService) loginAccount(ctx context.Context, account accountdomain.Account) (*taskHTTPClient, error) {
	if strings.TrimSpace(account.Username) == "" || strings.TrimSpace(account.Password) == "" {
		return nil, errors.New("缺少用户名或密码")
	}
	key := accountSessionKey(account)
	s.sessionMu.Lock()
	cached := s.sessions[key]
	s.sessionMu.Unlock()
	if cached != nil {
		if ok, nonce, err := cached.validSession(ctx); ok {
			return cached, nil
		} else if err != nil {
			_ = err
		} else if nonce != "" {
			return cached, nil
		}
	}

	sessionFiles := s.accountSessionFiles(account)
	client, err := newTaskHTTPClient(s.currentNetworkProxy(), nativeDefaultTimeout, sessionFiles.CookiePath, sessionFiles.TokenPath)
	if err != nil {
		return nil, err
	}
	if err := client.login(ctx, account.Username, account.Password); err != nil {
		return nil, err
	}
	s.sessionMu.Lock()
	s.sessions[key] = client
	s.sessionMu.Unlock()
	return client, nil
}

func (s *nativeTaskService) reloginAccount(ctx context.Context, account accountdomain.Account) (*taskHTTPClient, error) {
	key := accountSessionKey(account)
	s.sessionMu.Lock()
	delete(s.sessions, key)
	s.sessionMu.Unlock()
	return s.loginAccount(ctx, account)
}

func (s *nativeTaskService) setNetworkProxy(settings NetworkProxySettings) {
	normalized, err := normalizeNetworkProxy(settings)
	if err != nil {
		normalized = NetworkProxySettings{}
	}
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if sameNetworkProxy(s.networkProxy, normalized) {
		return
	}
	s.networkProxy = normalized
	s.sessions = map[string]*taskHTTPClient{}
}

func (s *nativeTaskService) currentNetworkProxy() NetworkProxySettings {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.networkProxy
}

func accountSessionKey(account accountdomain.Account) string {
	return firstNonEmpty(strings.TrimSpace(account.Name), strings.TrimSpace(account.Username))
}

func (s *nativeTaskService) accountSessionFiles(account accountdomain.Account) accountSessionFiles {
	slug := sanitizePathComponent(firstNonEmpty(strings.TrimSpace(account.Name), strings.TrimSpace(account.Username), "account"))
	dir := filepath.Join(s.layout.AppRuntimeRoot, "accounts", slug)
	return accountSessionFiles{
		Dir:        dir,
		CookiePath: filepath.Join(dir, "session.cookies.json"),
		TokenPath:  filepath.Join(dir, "session.token.txt"),
	}
}

func sameNetworkProxy(left NetworkProxySettings, right NetworkProxySettings) bool {
	return left.Enabled == right.Enabled &&
		left.Type == right.Type &&
		left.Host == right.Host &&
		left.Port == right.Port &&
		left.Username == right.Username &&
		left.Password == right.Password &&
		left.LoginAttempts == right.LoginAttempts &&
		left.LoginRetryDelaySeconds == right.LoginRetryDelaySeconds
}

func newTaskHTTPClient(proxySettings NetworkProxySettings, timeout time.Duration, cookiePath string, tokenPath string) (*taskHTTPClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	transport, err := newAccountTransport(proxySettings, timeout)
	if err != nil {
		return nil, err
	}
	client := &taskHTTPClient{
		client: &http.Client{
			Timeout:   timeout,
			Jar:       jar,
			Transport: transport,
		},
		baseURL:    nativeBaseURL,
		cookiePath: strings.TrimSpace(cookiePath),
		tokenPath:  strings.TrimSpace(tokenPath),
		headers:    httpx.NewBrowserHeaderProfile(),
		loginAttempts: func() int {
			if proxySettings.LoginAttempts > 0 {
				return proxySettings.LoginAttempts
			}
			return nativeDefaultLoginAttempts
		}(),
		loginRetryDelay: func() time.Duration {
			if proxySettings.LoginRetryDelaySeconds > 0 {
				return time.Duration(proxySettings.LoginRetryDelaySeconds) * time.Second
			}
			return nativeDefaultLoginRetryDelay
		}(),
		lastNonce: map[string]cachedNonce{},
	}
	client.loadSessionCookies()
	return client, nil
}

func (c *taskHTTPClient) absoluteURL(pathOrURL string) string {
	pathOrURL = strings.TrimSpace(pathOrURL)
	if pathOrURL == "" {
		return c.baseURL
	}
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		return pathOrURL
	}
	base, _ := url.Parse(c.baseURL + "/")
	ref, _ := url.Parse(pathOrURL)
	return base.ResolveReference(ref).String()
}

func (c *taskHTTPClient) do(ctx context.Context, method string, pathOrURL string, form url.Values, headers map[string]string) (*http.Response, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	for attempt := 1; attempt <= nativeRequestAttempts; attempt++ {
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, c.absoluteURL(pathOrURL), body)
		if err != nil {
			return nil, err
		}
		kind := httpx.BrowserNavigationRequest
		if form != nil {
			kind = httpx.BrowserFormRequest
		}
		httpx.ApplyBrowserHeadersWithProfile(req, c.baseURL+"/", kind, c.headers)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			if attempt < nativeRequestAttempts && shouldRetryHTTPError(err) {
				if waitErr := waitRetry(ctx, attempt, ""); waitErr == nil {
					continue
				} else {
					return nil, waitErr
				}
			}
			return nil, err
		}
		c.persistSessionCookies()
		if attempt < nativeRequestAttempts && shouldRetryHTTPStatus(resp.StatusCode) {
			retryAfter := resp.Header.Get("Retry-After")
			resp.Body.Close()
			if waitErr := waitRetry(ctx, attempt, retryAfter); waitErr == nil {
				continue
			} else {
				return nil, waitErr
			}
		}
		return resp, nil
	}
	return nil, errors.New("request attempts exhausted")
}

func (c *taskHTTPClient) get(ctx context.Context, pathOrURL string) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, pathOrURL, nil, nil)
}

func (c *taskHTTPClient) postForm(ctx context.Context, pathOrURL string, form url.Values) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, pathOrURL, form, nil)
}

func (c *taskHTTPClient) login(ctx context.Context, username string, password string) error {
	var lastErr error
	loginAttempts := nativeDefaultLoginAttempts
	loginRetryDelay := nativeDefaultLoginRetryDelay
	if c != nil {
		if attempts := c.loginAttempts; attempts > 0 {
			loginAttempts = attempts
		}
		if delay := c.loginRetryDelay; delay > 0 {
			loginRetryDelay = delay
		}
	}
	for attempt := 1; attempt <= loginAttempts; attempt++ {
		if err := c.loginOnce(ctx, username, password); err != nil {
			lastErr = err
			if !shouldRetryHTTPError(err) || attempt == loginAttempts {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * loginRetryDelay):
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("登录失败，已重试 %d 次: %w", loginAttempts, lastErr)
	}
	return nil
}

func (c *taskHTTPClient) loginOnce(ctx context.Context, username string, password string) error {
	form := url.Values{}
	form.Set("name", username)
	form.Set("password", password)
	resp, err := c.postForm(ctx, nativeLoginPath, form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("登录失败: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if bytes.Contains(body, []byte("用户名或者密码错误")) || bytes.Contains(body, []byte("密码错误")) {
		return errors.New("登录失败：用户名或者密码错误")
	}
	verify, err := c.get(ctx, "/challenges")
	if err != nil {
		return err
	}
	defer verify.Body.Close()
	verifyBody, err := io.ReadAll(verify.Body)
	if err != nil {
		return err
	}
	if looksLikeLoginPage(verify.Request.URL.Path, string(verifyBody)) {
		return errors.New("登录后仍被重定向回登录页")
	}
	return nil
}

func shouldRetryHTTPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "timeout") ||
		strings.Contains(text, "deadline exceeded") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "eof occurred in violation of protocol") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "connection reset by peer") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "remote end closed connection") ||
		strings.Contains(text, "server disconnected") ||
		strings.Contains(text, "tls") ||
		strings.Contains(text, "ssl") ||
		strings.Contains(text, "temporary")
}

func (c *taskHTTPClient) validSession(ctx context.Context) (bool, string, error) {
	if nonce, ok := c.cachedNonce(nativeSections["challenges"].Name); ok {
		return true, nonce, nil
	}
	nonce, err := fetchNonce(ctx, c, nativeSections["challenges"])
	if err == nil && strings.TrimSpace(nonce) != "" {
		return true, nonce, nil
	}
	verify, verifyErr := c.get(ctx, "/challenges")
	if verifyErr != nil {
		return false, "", verifyErr
	}
	body, readErr := ioReadAllAndClose(verify)
	if readErr != nil {
		return false, "", readErr
	}
	if verify.StatusCode == 401 || verify.StatusCode == 403 || looksLikeLoginPage(verify.Request.URL.Path, body) {
		return false, "", nil
	}
	return true, strings.TrimSpace(nonce), nil
}

func (c *taskHTTPClient) cachedNonce(section string) (string, bool) {
	c.nonceMu.Lock()
	defer c.nonceMu.Unlock()
	item, ok := c.lastNonce[strings.TrimSpace(section)]
	if !ok || strings.TrimSpace(item.Value) == "" || time.Now().After(item.ExpiresAt) {
		return "", false
	}
	return item.Value, true
}

func (c *taskHTTPClient) storeNonce(section string, value string) {
	section = strings.TrimSpace(section)
	value = strings.TrimSpace(value)
	if section == "" || value == "" {
		return
	}
	c.nonceMu.Lock()
	defer c.nonceMu.Unlock()
	c.lastNonce[section] = cachedNonce{
		Value:     value,
		ExpiresAt: time.Now().Add(nativeNonceTTL),
	}
}

func (c *taskHTTPClient) loadSessionCookies() {
	base, err := url.Parse(c.baseURL)
	if err != nil || c.client == nil || c.client.Jar == nil {
		return
	}
	cookies := loadPersistedCookies(c.cookiePath)
	if len(cookies) == 0 {
		cookies = parseCookieHeader(loadTokenFile(c.tokenPath))
	}
	if len(cookies) == 0 {
		return
	}
	c.client.Jar.SetCookies(base, cookies)
}

func (c *taskHTTPClient) persistSessionCookies() {
	base, err := url.Parse(c.baseURL)
	if err != nil || c.client == nil || c.client.Jar == nil {
		return
	}
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()
	cookies := c.client.Jar.Cookies(base)
	if strings.TrimSpace(c.cookiePath) != "" {
		_ = savePersistedCookies(c.cookiePath, cookies)
	}
	if strings.TrimSpace(c.tokenPath) != "" {
		_ = saveTokenFile(c.tokenPath, cookieHeaderForCookies(cookies))
	}
}

func (c *taskHTTPClient) sessionToken() string {
	base, err := url.Parse(c.baseURL)
	if err != nil || c.client == nil || c.client.Jar == nil {
		return ""
	}
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()
	return cookieHeaderForCookies(c.client.Jar.Cookies(base))
}

func (s *nativeTaskService) fetchChallenges(ctx context.Context, client *taskHTTPClient, section sectionConfig) ([]fetchedChallenge, error) {
	indexResp, err := client.get(ctx, section.IndexPage)
	if err != nil {
		return nil, err
	}
	indexBody, err := ioReadAllAndClose(indexResp)
	if err != nil {
		return nil, err
	}
	if looksLikeLoginPage(indexResp.Request.URL.Path, indexBody) {
		return nil, fmt.Errorf("%s 页面需要登录", section.Name)
	}

	listResp, err := client.get(ctx, section.ListEndpoint)
	if err == nil {
		listBody, bodyErr := ioReadAllAndClose(listResp)
		if bodyErr == nil {
			var payload any
			if json.Unmarshal([]byte(listBody), &payload) == nil {
				items := normalizeChallengeList(payload, section, client)
				if len(items) > 0 {
					return items, nil
				}
			}
		}
	}
	items := extractChallengesFromHTML(indexBody, indexResp.Request.URL.String(), section, client)
	if len(items) > 0 {
		return items, nil
	}
	return nil, fmt.Errorf("没能从 %s 页面解析出题目列表", section.Name)
}

func normalizeChallengeList(payload any, section sectionConfig, client *taskHTTPClient) []fetchedChallenge {
	candidates := make([]any, 0)
	switch current := payload.(type) {
	case []any:
		candidates = current
	case map[string]any:
		for _, key := range []string{"game", "data", "challenges", "results", "items"} {
			if list, ok := current[key].([]any); ok {
				candidates = list
				break
			}
		}
		if len(candidates) == 0 {
			allMaps := true
			for _, value := range current {
				if _, ok := value.(map[string]any); !ok {
					allMaps = false
					break
				}
			}
			if allMaps {
				for _, value := range current {
					candidates = append(candidates, value)
				}
			}
		}
	}

	results := make([]fetchedChallenge, 0, len(candidates))
	for _, raw := range candidates {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		challengeID := firstString(item["id"], item["chalid"], item["chal_id"], item["value"])
		if challengeID == "" {
			continue
		}
		title := firstString(item["name"], item["title"], item["chal"], item["challenge"])
		if title == "" {
			title = "challenge_" + challengeID
		}
		category := firstString(item["category"], item["type"], item["group"], item["field"])
		tags := normalizeTags(item["tags"])
		tags = append(tags, normalizeTags(item["tag"])...)
		kind := detectChallengeKind(section.Name, title, category, tags)
		expectsAttachments, expectsRemote := challengeExpectations(kind)
		results = append(results, fetchedChallenge{
			ChallengeID:        challengeID,
			Section:            section.Name,
			Title:              title,
			Category:           category,
			Tags:               dedupeStrings(tags),
			ChallengeKind:      kind,
			ExpectsAttachments: expectsAttachments,
			ExpectsRemote:      expectsRemote,
			DetailURL:          client.absoluteURL(strings.ReplaceAll(section.DetailEndpointTemplate, "{id}", challengeID)),
		})
	}
	return results
}

func extractChallengesFromHTML(htmlText string, currentURL string, section sectionConfig, client *taskHTTPClient) []fetchedChallenge {
	root, err := xhtml.Parse(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	found := map[string]fetchedChallenge{}
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "a" {
			href := attrValue(node, "href")
			fullURL := resolveURL(currentURL, href)
			match := reChallengeLink.FindStringSubmatch(mustURLPath(fullURL))
			if len(match) == 3 {
				challengeID := strings.TrimSpace(match[2])
				title := collapseText(textContent(node))
				if title == "" {
					title = "challenge_" + challengeID
				}
				kind := detectChallengeKind(section.Name, title, "", nil)
				expectsAttachments, expectsRemote := challengeExpectations(kind)
				found[challengeID] = fetchedChallenge{
					ChallengeID:        challengeID,
					Section:            section.Name,
					Title:              title,
					ChallengeKind:      kind,
					ExpectsAttachments: expectsAttachments,
					ExpectsRemote:      expectsRemote,
					DetailURL:          client.absoluteURL(strings.ReplaceAll(section.DetailEndpointTemplate, "{id}", challengeID)),
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)

	if len(found) == 0 {
		for _, match := range regexp.MustCompile(`data-id=["']?(\d+)["']?`).FindAllStringSubmatch(htmlText, -1) {
			challengeID := strings.TrimSpace(match[1])
			kind := detectChallengeKind(section.Name, section.Name+"_"+challengeID, "", nil)
			expectsAttachments, expectsRemote := challengeExpectations(kind)
			found[challengeID] = fetchedChallenge{
				ChallengeID:        challengeID,
				Section:            section.Name,
				Title:              section.Name + "_" + challengeID,
				ChallengeKind:      kind,
				ExpectsAttachments: expectsAttachments,
				ExpectsRemote:      expectsRemote,
				DetailURL:          client.absoluteURL(strings.ReplaceAll(section.DetailEndpointTemplate, "{id}", challengeID)),
			}
		}
	}

	keys := make([]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool { return numericPrefix(keys[i]) < numericPrefix(keys[j]) })
	results := make([]fetchedChallenge, 0, len(keys))
	for _, key := range keys {
		results = append(results, found[key])
	}
	return results
}

func (s *nativeTaskService) fetchChallengeDetail(ctx context.Context, client *taskHTTPClient, section sectionConfig, challenge fetchedChallenge) (fetchedChallenge, error) {
	detailAPI := strings.ReplaceAll(section.DetailAPIEndpoint, "{id}", challenge.ChallengeID)
	resp, err := client.get(ctx, detailAPI)
	if err == nil {
		body, readErr := ioReadAllAndClose(resp)
		if readErr == nil {
			var payload any
			if json.Unmarshal([]byte(body), &payload) == nil {
				enrichChallengeFromPayload(&challenge, payload, client)
				return finalizeChallenge(challenge), nil
			}
		}
	}

	form := url.Values{}
	resp, err = client.postForm(ctx, strings.ReplaceAll(section.DetailEndpointTemplate, "{id}", challenge.ChallengeID), form)
	if err != nil {
		return challenge, err
	}
	body, err := ioReadAllAndClose(resp)
	if err != nil {
		return challenge, err
	}
	if strings.TrimSpace(body) == "-1" {
		return challenge, errors.New("题目详情拉取失败：当前会话未授权")
	}
	var payload any
	if json.Unmarshal([]byte(body), &payload) == nil {
		enrichChallengeFromPayload(&challenge, payload, client)
		return finalizeChallenge(challenge), nil
	}
	challenge.DescriptionHTML = body
	challenge.DescriptionText = stripHTML(body)
	challenge.Attachments = extractAttachmentsFromHTML(body, resp.Request.URL.String(), client)
	challenge.RemoteTargets = extractRemoteTargetsFromHTML(body, resp.Request.URL.String())
	return finalizeChallenge(challenge), nil
}

func finalizeChallenge(challenge fetchedChallenge) fetchedChallenge {
	if challenge.DescriptionText == "" && challenge.DescriptionHTML != "" {
		challenge.DescriptionText = stripHTML(challenge.DescriptionHTML)
	}
	challenge.ChallengeKind = detectChallengeKind(challenge.Section, challenge.Title, challenge.Category, challenge.Tags)
	challenge.ExpectsAttachments, challenge.ExpectsRemote = challengeExpectations(challenge.ChallengeKind)
	if len(challenge.RemoteTargets) == 0 {
		challenge.RemoteTargets = extractRemoteTargetsFromText(challenge.DescriptionText, "description")
	}
	challenge.RemoteTargets = dedupeRemoteTargets(challenge.RemoteTargets)
	challenge.RemoteSHA256 = stableJSONHash(normalizeRemoteJSON(challenge.RemoteTargets))
	challenge.DescriptionSHA256 = sha256Text(firstNonEmpty(challenge.DescriptionText, challenge.DescriptionHTML))
	challenge.AssetWarnings = challengeAssetWarnings(challenge)
	return challenge
}

func enrichChallengeFromPayload(challenge *fetchedChallenge, payload any, client *taskHTTPClient) {
	switch current := payload.(type) {
	case map[string]any:
		if title := firstString(current["name"], current["title"], current["chal"], current["challenge"]); title != "" {
			challenge.Title = title
		}
		if category := firstString(current["category"], current["type"], current["group"], current["field"]); category != "" {
			challenge.Category = category
		}
		tags := append(normalizeTags(current["tags"]), normalizeTags(current["tag"])...)
		challenge.Tags = dedupeStrings(append(challenge.Tags, tags...))
		description := firstString(current["description"], current["desc"], current["html"], current["content"], current["message"])
		if description != "" {
			if strings.Contains(description, "<") && strings.Contains(description, ">") {
				challenge.DescriptionHTML = description
				challenge.DescriptionText = stripHTML(description)
			} else {
				challenge.DescriptionText = strings.TrimSpace(description)
			}
		}
		challenge.Attachments = extractAttachmentsFromPayload(current, client)
		challenge.RemoteTargets = extractRemoteTargetsFromPayload(current)
		if len(challenge.Attachments) == 0 && challenge.DescriptionHTML != "" {
			challenge.Attachments = extractAttachmentsFromHTML(challenge.DescriptionHTML, challenge.DetailURL, client)
		}
		if len(challenge.RemoteTargets) == 0 && challenge.DescriptionHTML != "" {
			challenge.RemoteTargets = extractRemoteTargetsFromHTML(challenge.DescriptionHTML, challenge.DetailURL)
		}
	case []any:
		challenge.DescriptionText = mustJSON(payload)
		challenge.Attachments = extractAttachmentsFromPayload(payload, client)
		challenge.RemoteTargets = extractRemoteTargetsFromPayload(payload)
	default:
		challenge.DescriptionText = strings.TrimSpace(fmt.Sprint(payload))
		challenge.RemoteTargets = extractRemoteTargetsFromText(challenge.DescriptionText, "payload")
	}
}

func (s *nativeTaskService) materializeChallenge(ctx context.Context, client *taskHTTPClient, challenge fetchedChallenge, forceDownload bool) (challengeRecord, error) {
	dirName, dirPath, err := s.detectLocalChallengeDir(challenge)
	if err != nil {
		return challengeRecord{}, err
	}
	attachmentsDir := filepath.Join(dirPath, "attachments")
	if err := os.MkdirAll(attachmentsDir, 0o755); err != nil {
		return challengeRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(dirPath, "description.md"), []byte(strings.TrimSpace(challenge.DescriptionText)+"\n"), 0o644); err != nil {
		return challengeRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(dirPath, "remote.txt"), []byte(buildRemoteSummary(challenge.RemoteTargets)), 0o644); err != nil {
		return challengeRecord{}, err
	}
	if err := os.WriteFile(filepath.Join(dirPath, "challenge.meta.json"), []byte(buildLocalChallengeMeta(challenge, dirName, dirPath)), 0o644); err != nil {
		return challengeRecord{}, err
	}
	challenge.DescriptionPath = filepath.Join(dirPath, "description.md")
	challenge.RemoteSummaryPath = filepath.Join(dirPath, "remote.txt")
	challenge.DirName = dirName
	challenge.DirPath = dirPath

	prev, _ := s.loadChallengeRecord(context.Background(), s.challengeRepo.DB(), challenge.Section, challenge.ChallengeID)
	prevAttachments := map[string]attachmentRecord{}
	for _, item := range prev.Attachments {
		prevAttachments[firstNonEmpty(item.URL, item.Name)] = item
	}
	currentPaths := map[string]bool{}
	for index := range challenge.Attachments {
		if strings.TrimSpace(challenge.Attachments[index].URL) == "" {
			continue
		}
		previous := prevAttachments[firstNonEmpty(challenge.Attachments[index].URL, challenge.Attachments[index].Name)]
		item, syncErr := s.syncAttachment(ctx, client, challenge, challenge.Attachments[index], attachmentsDir, previous, forceDownload, isRealtimeCandidate(challenge.ChallengeKind, challenge.Category, challenge.Title, challenge.Tags, nativeSections[challenge.Section].RealtimeKeywords))
		if syncErr != nil {
			challenge.Attachments[index].Changed = true
			challenge.AssetWarnings = append(challenge.AssetWarnings, "attachment_download_failed:"+firstNonEmpty(challenge.Attachments[index].Name, challenge.Attachments[index].URL))
			continue
		}
		challenge.Attachments[index] = item
		currentPaths[item.LocalPath] = true
	}
	entries, _ := os.ReadDir(attachmentsDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(attachmentsDir, entry.Name())
		if !currentPaths[fullPath] {
			_ = os.Remove(fullPath)
		}
	}

	challenge.Fingerprint = stableJSONHash(map[string]any{
		"challenge_id":       challenge.ChallengeID,
		"title":              challenge.Title,
		"challenge_kind":     challenge.ChallengeKind,
		"category":           challenge.Category,
		"description_sha256": challenge.DescriptionSHA256,
		"remote_sha256":      challenge.RemoteSHA256,
		"remote_targets":     normalizeRemoteJSON(challenge.RemoteTargets),
		"attachments":        normalizeAttachmentJSON(challenge.Attachments),
	})
	challenge.Changed = challenge.Fingerprint != prev.Fingerprint || attachmentsChanged(challenge.Attachments)

	return challengeRecord{
		Key:                challenge.Section + ":" + challenge.ChallengeID,
		ChallengeID:        challenge.ChallengeID,
		Section:            challenge.Section,
		Title:              challenge.Title,
		Category:           challenge.Category,
		ChallengeKind:      challenge.ChallengeKind,
		ExpectsAttachments: challenge.ExpectsAttachments,
		ExpectsRemote:      challenge.ExpectsRemote,
		AssetWarnings:      challenge.AssetWarnings,
		DirName:            dirName,
		DirPath:            dirPath,
		DetailURL:          challenge.DetailURL,
		DescriptionPath:    challenge.DescriptionPath,
		DescriptionSHA256:  challenge.DescriptionSHA256,
		RemoteSummaryPath:  challenge.RemoteSummaryPath,
		RemoteSHA256:       challenge.RemoteSHA256,
		RemoteTargets:      challenge.RemoteTargets,
		Fingerprint:        challenge.Fingerprint,
		Changed:            challenge.Changed,
		Attachments:        challenge.Attachments,
		UpdatedAt:          nowTS(),
	}, nil
}

func (s *nativeTaskService) detectLocalChallengeDir(challenge fetchedChallenge) (string, string, error) {
	if err := os.MkdirAll(s.layout.ChallengesRoot, 0o755); err != nil {
		return "", "", err
	}
	canonical := buildCanonicalChallengeDirName(challenge)
	target := filepath.Join(s.layout.ChallengesRoot, canonical)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return "", "", err
	}
	pattern := filepath.Join(s.layout.ChallengesRoot, challenge.ChallengeID+"_*")
	matches, _ := filepath.Glob(pattern)
	for _, candidate := range matches {
		candidate = filepath.Clean(candidate)
		if candidate == target {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = mergeDirInto(candidate, target)
	}
	return canonical, target, nil
}

func buildCanonicalChallengeDirName(challenge fetchedChallenge) string {
	title := strings.TrimSpace(challenge.Title)
	if isPlaceholderChallengeTitle(title, challenge.ChallengeID) {
		title = "challenge_" + strings.TrimSpace(challenge.ChallengeID)
	}
	return sanitizePathComponent(strings.TrimSpace(challenge.ChallengeID) + "_" + title)
}

func buildLocalChallengeMeta(challenge fetchedChallenge, dirName string, dirPath string) string {
	payload := map[string]any{
		"challenge_id":        strings.TrimSpace(challenge.ChallengeID),
		"section":             strings.TrimSpace(challenge.Section),
		"title":               strings.TrimSpace(challenge.Title),
		"category":            strings.TrimSpace(challenge.Category),
		"challenge_kind":      strings.TrimSpace(challenge.ChallengeKind),
		"expects_attachments": challenge.ExpectsAttachments,
		"expects_remote":      challenge.ExpectsRemote,
		"detail_url":          strings.TrimSpace(challenge.DetailURL),
		"dir_name":            strings.TrimSpace(dirName),
		"dir_path":            strings.TrimSpace(dirPath),
		"updated_at":          nowTS(),
	}
	return mustJSON(payload)
}

func mergeDirInto(source string, target string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(source, entry.Name())
		dst := filepath.Join(target, entry.Name())
		if entry.IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			if err := mergeDirInto(src, dst); err != nil {
				return err
			}
			_ = os.Remove(src)
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			if copyErr := copyFileRaw(src, dst); copyErr != nil {
				return copyErr
			}
			_ = os.Remove(src)
		}
	}
	_ = os.Remove(source)
	return nil
}

func (s *nativeTaskService) syncAttachment(ctx context.Context, client *taskHTTPClient, challenge fetchedChallenge, attachment attachmentRecord, attachmentsDir string, previous attachmentRecord, forceDownload bool, alwaysRefresh bool) (attachmentRecord, error) {
	previousPath := strings.TrimSpace(previous.LocalPath)
	shouldFetch := forceDownload || alwaysRefresh || previousPath == ""
	if !shouldFetch && previousPath != "" {
		if _, err := os.Stat(previousPath); err != nil {
			shouldFetch = true
		}
	}

	var payload []byte
	var fileHash string
	var fileSize int64
	var changed bool
	var sharedPath string

	if shouldFetch {
		reqCtx, cancel := context.WithTimeout(ctx, 2*nativeDefaultTimeout)
		defer cancel()
		resp, err := client.get(reqCtx, attachment.URL)
		if err != nil {
			return attachment, err
		}
		payload, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return attachment, err
		}
		if resp.StatusCode >= 400 {
			return attachment, fmt.Errorf("下载附件失败：%s 返回 HTTP %d", attachment.URL, resp.StatusCode)
		}
		fileHash = sha256Bytes(payload)
		fileSize = int64(len(payload))
		changed = fileHash != strings.TrimSpace(previous.SHA256)
		sharedPath, err = ensureSharedAttachmentBytes(s.layout, payload, fileHash)
		if err != nil {
			return attachment, err
		}
	} else {
		fileHash = fileSHA256(previousPath)
		info, err := os.Stat(previousPath)
		if err != nil {
			return attachment, err
		}
		fileSize = info.Size()
		changed = fileHash != strings.TrimSpace(previous.SHA256)
		var errShared error
		sharedPath, errShared = ensureSharedAttachmentFile(s.layout, previousPath, fileHash)
		if errShared != nil {
			return attachment, errShared
		}
	}

	finalName := buildAttachmentFilename(challenge, attachment.Name, fileHash)
	finalPath := filepath.Join(attachmentsDir, finalName)
	storageMode, err := materializeAttachmentLink(sharedPath, finalPath)
	if err != nil {
		return attachment, err
	}
	if previousPath != "" && previousPath != finalPath {
		_ = os.Remove(previousPath)
	}
	attachment.StoredName = finalName
	attachment.LocalPath = finalPath
	attachment.SharedPath = sharedPath
	attachment.StorageMode = storageMode
	attachment.SHA256 = fileHash
	attachment.Size = fileSize
	attachment.Changed = changed
	return attachment, nil
}

func ensureSharedAttachmentBytes(layout runtime.Layout, payload []byte, sha256Value string) (string, error) {
	target := sharedAttachmentPath(layout, sha256Value)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return "", err
	}
	return target, nil
}

func ensureSharedAttachmentFile(layout runtime.Layout, source string, sha256Value string) (string, error) {
	target := sharedAttachmentPath(layout, sha256Value)
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.Link(source, target); err == nil {
		return target, nil
	}
	if err := copyFileRaw(source, target); err != nil {
		return "", err
	}
	return target, nil
}

func sharedAttachmentPath(layout runtime.Layout, sha256Value string) string {
	return filepath.Join(layout.AppRuntimeRoot, "shared", "attachments", sha256Value[:2], sha256Value+".blob")
}

func materializeAttachmentLink(sharedPath string, target string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if info, err := os.Lstat(target); err == nil && info != nil {
		resolvedCurrent, errCurrent := filepath.EvalSymlinks(target)
		resolvedShared, errShared := filepath.EvalSymlinks(sharedPath)
		if errCurrent == nil && errShared == nil && resolvedCurrent == resolvedShared {
			return "existing", nil
		}
		_ = os.Remove(target)
	}
	if err := os.Link(sharedPath, target); err == nil {
		return "hardlink", nil
	}
	if err := os.Symlink(sharedPath, target); err == nil {
		return "symlink", nil
	}
	if err := copyFileRaw(sharedPath, target); err != nil {
		return "", err
	}
	return "copy", nil
}

func buildAttachmentFilename(challenge fetchedChallenge, attachmentName string, fileHash string) string {
	original := sanitizePathComponent(firstNonEmpty(attachmentName, "attachment.bin"))
	suffix := path.Ext(original)
	stem := strings.TrimSuffix(original, suffix)
	challengeStub := sanitizePathComponent(buildCanonicalChallengeDirName(challenge))
	hashStub := firstN(fileHash, 12)
	return sanitizePathComponent(fmt.Sprintf("%s__%s__%s%s", challengeStub, stem, hashStub, suffix))
}

func (s *nativeTaskService) refreshPlatformSubmissions(ctx context.Context, client *taskHTTPClient, section sectionConfig) (map[string]remoteSolveInfo, error) {
	resp, err := client.get(ctx, section.SolvesEndpoint)
	if err != nil {
		return nil, err
	}
	body, err := ioReadAllAndClose(resp)
	if err != nil {
		return nil, err
	}
	if looksLikeLoginPage(resp.Request.URL.Path, body) || resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("拉取当前账号已提交题目失败：%s 需要重新登录", section.SolvesEndpoint)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("拉取当前账号已提交题目失败：%s 返回的不是 JSON", section.SolvesEndpoint)
	}
	solved := map[string]remoteSolveInfo{}
	rawList, _ := payload["solves"].([]any)
	for _, raw := range rawList {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		challengeID := firstString(item["chalid"], item["challenge_id"], item["id"])
		if challengeID == "" {
			continue
		}
		solved[challengeID] = remoteSolveInfo{
			ChallengeID: challengeID,
			Title:       firstString(item["chal"]),
			Category:    firstString(item["category"]),
			SubmittedAt: formatUnixTS(item["time"]),
			Value:       item["value"],
			SolveID:     item["id"],
			TeamID:      item["team"],
		}
	}
	return solved, nil
}

func (s *nativeTaskService) runChallengeSolver(ctx context.Context, client *taskHTTPClient, account accountdomain.Account, challenge challengeRecord) (string, solverResult, error) {
	solverPath := firstExisting(challenge.DirPath, "solve.py", "solver.py", "exp.py", "exploit.py", "poc.py")
	if strings.TrimSpace(solverPath) == "" {
		result := solverResult{Status: "skipped", Reason: "solver script not found"}
		return "", result, nil
	}
	if s.runner == nil {
		result := solverResult{Status: "error", Reason: "python runner unavailable"}
		return "", result, errors.New(result.Reason)
	}
	candidatePython := dedupeStrings([]string{
		s.pythonBinary,
		filepath.Join(s.layout.AppDataRoot, "python", "bin", "python3"),
		filepath.Join(s.layout.AppDataRoot, "python", "Scripts", "python.exe"),
		"python3",
		"python",
	})
	tried := make([]string, 0, len(candidatePython))
	var lastRun pythonrunner.RunResult
	var lastErr error
	solverArgs := buildSolverArgs(challenge)
	primaryAttachment := firstExistingAttachment(challenge)
	for _, pythonBinary := range candidatePython {
		runReq := pythonrunner.RunRequest{
			Code: solverBootstrap,
			CopyPaths: []pythonrunner.CopyPath{
				{Source: challenge.DirPath, Target: "challenge"},
			},
			Script:         "bootstrap.py",
			Args:           append([]string{"challenge/" + filepath.Base(solverPath)}, solverArgs...),
			PythonBinary:   pythonBinary,
			TimeoutSeconds: 600,
			Profile:        "local-isolated",
			Env: map[string]string{
				"AUTO_ACCOUNT_NAME":            account.Name,
				"AUTO_ACCOUNT_USERNAME":        account.Username,
				"AUTO_CHALLENGE_ID":            challenge.ChallengeID,
				"AUTO_CHALLENGE_KEY":           challenge.Key,
				"AUTO_CHALLENGE_TITLE":         challenge.Title,
				"AUTO_CHALLENGE_CATEGORY":      challenge.Category,
				"AUTO_CHALLENGE_DIR":           "{{SANDBOX_ROOT}}/challenge",
				"AUTO_CHALLENGE_FINGERPRINT":   challenge.Fingerprint,
				"AUTO_CHALLENGES_DB":           s.layout.AppDatabasePath,
				"AUTO_ATTACHMENTS_DIR":         "{{SANDBOX_ROOT}}/challenge/attachments",
				"AUTO_SHARED_ATTACHMENTS_ROOT": filepath.Join(s.layout.AppRuntimeRoot, "shared", "attachments"),
				"AUTO_REMOTE_FILE":             "{{SANDBOX_ROOT}}/challenge/remote.txt",
				"AUTO_PRIMARY_ATTACHMENT":      primaryAttachment,
				"AUTO_SESSION_TOKEN":           sessionToken(client),
				"AUTO_BASE_URL":                nativeBaseURL,
			},
		}
		tried = append(tried, pythonBinary)
		lastRun, lastErr = s.runner.Run(ctx, runReq)
		if lastErr != nil {
			continue
		}
		if lastRun.ExitCode == 0 || !shouldRetrySolverInterpreter(lastRun) {
			break
		}
	}

	result := solverResult{
		Status:            "error",
		ReturnCode:        lastRun.ExitCode,
		Entrypoint:        "submit_flag_or_main",
		SolverFile:        solverPath,
		Python:            firstNonEmpty(lastRun.Command...),
		TriedInterpreters: tried,
		Fingerprint:       challenge.Fingerprint,
		RanAt:             nowTS(),
		StdoutTail:        tailText(lastRun.Stdout, 2000),
		StderrTail:        tailText(lastRun.Stderr, 2000),
	}
	if lastErr != nil {
		result.Reason = lastErr.Error()
		return "", result, lastErr
	}
	flagValue := extractSolverResultFlag(lastRun.Stdout)
	if flagValue == "" {
		flagValue = extractFlag(lastRun.Stdout)
	}
	result.Flag = flagValue
	if lastRun.ExitCode == 0 {
		result.Status = "ok"
	} else {
		result.Status = "error"
		result.Reason = firstNonEmpty(extractShortSolverFailure(lastRun.Stderr), extractShortSolverFailure(lastRun.Stdout), "solver exited with non-zero status")
	}
	return flagValue, result, nil
}

func shouldRetrySolverInterpreter(result pythonrunner.RunResult) bool {
	text := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	return strings.Contains(text, "modulenotfounderror") ||
		strings.Contains(text, "no module named") ||
		strings.Contains(text, "importerror")
}

func extractSolverResultFlag(stdout string) string {
	lines := strings.Split(stdout, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "__AUTO_RESULT__=") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "__AUTO_RESULT__="))
		if payload == "" {
			return ""
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			return ""
		}
		return strings.TrimSpace(firstString(data["flag"]))
	}
	return ""
}

func extractFlag(text string) string {
	match := reFlag.FindString(text)
	return strings.TrimSpace(match)
}

func (s *nativeTaskService) submitFlagForChallenge(ctx context.Context, client *taskHTTPClient, account accountdomain.Account, challenge challengeRecord, flagValue string, manual bool) (map[string]any, error) {
	db := s.challengeRepo.DB()
	entry, err := s.loadChallengeAccountState(ctx, db, challenge.Key, account.Name)
	if err != nil {
		return nil, err
	}
	allowed, reason := shouldSubmit(entry, challenge, flagValue, manual, nativeSections[challenge.Section].RealtimeKeywords)
	if !allowed {
		result := map[string]any{
			"accepted": false,
			"skipped":  true,
			"message":  reason,
		}
		entry = recordSubmitSkipState(entry, challenge, flagValue, result)
		err = s.upsertChallengeAccounts(ctx, db, []challengeAccountRecord{{
			ChallengeKey: challenge.Key,
			AccountName:  account.Name,
			Data:         entry,
			UpdatedAt:    nowTS(),
		}})
		return result, err
	}

	fields := []string{"key", "flag", "answer", "submission"}
	section := nativeSections[challenge.Section]
	submitPath := strings.ReplaceAll(section.SubmitEndpointTemplate, "{id}", challenge.ChallengeID)
	tried := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		payload := url.Values{}
		payload.Set(field, flagValue)
		nonce, _ := fetchNonce(ctx, client, section)
		if nonce != "" {
			payload.Set("nonce", nonce)
		}
		resp, err := client.postForm(ctx, submitPath, payload)
		if err != nil {
			return nil, err
		}
		body, err := ioReadAllAndClose(resp)
		if err != nil {
			return nil, err
		}
		result := interpretSubmitResponse(resp.StatusCode, resp.Request.URL.String(), body)
		result["field"] = field
		result["fingerprint"] = challenge.Fingerprint
		tried = append(tried, result)
		if boolValue(result["accepted"]) {
			result["attempts"] = tried
			entry = recordSubmitState(entry, challenge, flagValue, result)
			if err := s.upsertChallengeAccounts(ctx, db, []challengeAccountRecord{{
				ChallengeKey: challenge.Key,
				AccountName:  account.Name,
				Data:         entry,
				UpdatedAt:    nowTS(),
			}}); err != nil {
				return nil, err
			}
			return result, nil
		}
		if boolValue(result["auth_error"]) {
			return nil, errors.New("提交 flag 时会话失效，请重新登录")
		}
	}
	final := map[string]any{
		"accepted":    false,
		"message":     "提交失败，所有候选字段都未通过。",
		"attempts":    tried,
		"fingerprint": challenge.Fingerprint,
	}
	entry = recordSubmitState(entry, challenge, flagValue, final)
	if err := s.upsertChallengeAccounts(ctx, db, []challengeAccountRecord{{
		ChallengeKey: challenge.Key,
		AccountName:  account.Name,
		Data:         entry,
		UpdatedAt:    nowTS(),
	}}); err != nil {
		return nil, err
	}
	return final, nil
}

func (s *nativeTaskService) submitFlagWithRetry(ctx context.Context, client *taskHTTPClient, account accountdomain.Account, challenge challengeRecord, flagValue string, manual bool, logger taskLogger) (*taskHTTPClient, map[string]any, error) {
	result, err := s.submitFlagForChallenge(ctx, client, account, challenge, flagValue, manual)
	if err != nil && isSessionExpiredError(err) {
		logger.Printf("%s %s #%s session expired, relogin and retry", account.Name, challenge.Section, challenge.ChallengeID)
		refreshed, loginErr := s.reloginAccount(ctx, account)
		if loginErr != nil {
			return client, nil, loginErr
		}
		client = refreshed
		result, err = s.submitFlagForChallenge(ctx, client, account, challenge, flagValue, manual)
	}
	return client, result, err
}

func fetchNonce(ctx context.Context, client *taskHTTPClient, section sectionConfig) (string, error) {
	if client != nil {
		if nonce, ok := client.cachedNonce(section.Name); ok {
			return nonce, nil
		}
	}
	resp, err := client.get(ctx, section.IndexPage)
	if err != nil {
		return "", err
	}
	body, err := ioReadAllAndClose(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 || looksLikeLoginPage(resp.Request.URL.Path, body) {
		return "", errors.New("会话失效，无法获取 nonce")
	}
	match := reNonceInput.FindStringSubmatch(body)
	if len(match) == 2 {
		nonce := strings.TrimSpace(match[1])
		client.storeNonce(section.Name, nonce)
		return nonce, nil
	}
	root, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return "", nil
	}
	var nonce string
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if nonce != "" {
			return
		}
		if node.Type == xhtml.ElementNode && node.Data == "input" {
			name := attrValue(node, "name")
			id := attrValue(node, "id")
			if name == "nonce" || id == "nonce" {
				nonce = strings.TrimSpace(attrValue(node, "value"))
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	client.storeNonce(section.Name, nonce)
	return nonce, nil
}

func isSessionExpiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "会话失效") ||
		strings.Contains(text, "重新登录") ||
		strings.Contains(text, "session") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "forbidden")
}

func interpretSubmitResponse(statusCode int, finalURL string, body string) map[string]any {
	text := strings.TrimSpace(body)
	result := map[string]any{
		"status":       statusCode,
		"url":          finalURL,
		"body":         tailText(text, 2000),
		"accepted":     false,
		"auth_error":   false,
		"message":      firstString(text[:minInt(len(text), 200)]),
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
	var payload any
	if json.Unmarshal([]byte(text), &payload) == nil {
		blob := mustJSON(payload)
		lowered := strings.ToLower(blob)
		if strings.Contains(lowered, `"success"`) || strings.Contains(lowered, `"correct"`) || strings.Contains(lowered, "accepted") || strings.Contains(lowered, "right") || strings.Contains(lowered, "passed") {
			result["accepted"] = true
		}
		result["message"] = tailText(blob, 200)
		return result
	}
	lowered := strings.ToLower(text)
	if strings.Contains(lowered, "correct") || strings.Contains(lowered, "success") || strings.Contains(lowered, "accepted") || strings.Contains(text, "答对") || strings.Contains(text, "正确") || strings.Contains(text, "通过") {
		result["accepted"] = true
	}
	return result
}

func (s *nativeTaskService) loadChallengeRecord(ctx context.Context, db *sql.DB, section string, challengeID string) (challengeRecord, error) {
	row := db.QueryRowContext(ctx, `
SELECT challenge_key, challenge_id, section_name, title, category, challenge_kind,
       expects_attachments, expects_remote, asset_warnings_json, dir_name, dir_path, detail_url,
       description_path, description_sha256, remote_summary_path, remote_sha256, remote_targets_json,
       fingerprint, changed, attachments_json, updated_at
FROM challenges
WHERE section_name = ? AND challenge_id = ?
`, section, challengeID)
	var record challengeRecord
	var warningsJSON string
	var remoteJSON string
	var attachmentsJSON string
	var expectsAttachments int
	var expectsRemote int
	var changed int
	err := row.Scan(
		&record.Key,
		&record.ChallengeID,
		&record.Section,
		&record.Title,
		&record.Category,
		&record.ChallengeKind,
		&expectsAttachments,
		&expectsRemote,
		&warningsJSON,
		&record.DirName,
		&record.DirPath,
		&record.DetailURL,
		&record.DescriptionPath,
		&record.DescriptionSHA256,
		&record.RemoteSummaryPath,
		&record.RemoteSHA256,
		&remoteJSON,
		&record.Fingerprint,
		&changed,
		&attachmentsJSON,
		&record.UpdatedAt,
	)
	if err != nil {
		return challengeRecord{}, err
	}
	record.ExpectsAttachments = expectsAttachments == 1
	record.ExpectsRemote = expectsRemote == 1
	record.Changed = changed == 1
	_ = json.Unmarshal([]byte(warningsJSON), &record.AssetWarnings)
	_ = json.Unmarshal([]byte(remoteJSON), &record.RemoteTargets)
	_ = json.Unmarshal([]byte(attachmentsJSON), &record.Attachments)
	return record, nil
}

func (s *nativeTaskService) loadSectionChallenges(ctx context.Context, db *sql.DB, section string, ids map[string]bool) ([]challengeRecord, error) {
	rows, err := db.QueryContext(ctx, `
SELECT challenge_key, challenge_id, section_name, title, category, challenge_kind,
       expects_attachments, expects_remote, asset_warnings_json, dir_name, dir_path, detail_url,
       description_path, description_sha256, remote_summary_path, remote_sha256, remote_targets_json,
       fingerprint, changed, attachments_json, updated_at
FROM challenges
WHERE section_name = ?
ORDER BY challenge_id + 0 ASC, challenge_key ASC
`, section)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]challengeRecord, 0, 64)
	for rows.Next() {
		var record challengeRecord
		var warningsJSON string
		var remoteJSON string
		var attachmentsJSON string
		var expectsAttachments int
		var expectsRemote int
		var changed int
		if err := rows.Scan(
			&record.Key,
			&record.ChallengeID,
			&record.Section,
			&record.Title,
			&record.Category,
			&record.ChallengeKind,
			&expectsAttachments,
			&expectsRemote,
			&warningsJSON,
			&record.DirName,
			&record.DirPath,
			&record.DetailURL,
			&record.DescriptionPath,
			&record.DescriptionSHA256,
			&record.RemoteSummaryPath,
			&record.RemoteSHA256,
			&remoteJSON,
			&record.Fingerprint,
			&changed,
			&attachmentsJSON,
			&record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if len(ids) > 0 && !ids[record.ChallengeID] {
			continue
		}
		record.ExpectsAttachments = expectsAttachments == 1
		record.ExpectsRemote = expectsRemote == 1
		record.Changed = changed == 1
		_ = json.Unmarshal([]byte(warningsJSON), &record.AssetWarnings)
		_ = json.Unmarshal([]byte(remoteJSON), &record.RemoteTargets)
		_ = json.Unmarshal([]byte(attachmentsJSON), &record.Attachments)
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *nativeTaskService) loadChallengeAccountState(ctx context.Context, db *sql.DB, challengeKey string, accountName string) (map[string]any, error) {
	var raw string
	err := db.QueryRowContext(ctx, `
SELECT data_json
FROM challenge_accounts
WHERE challenge_key = ? AND account_name = ?
`, challengeKey, accountName).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	data := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return data, nil
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *nativeTaskService) upsertChallenges(ctx context.Context, db *sql.DB, items []challengeRecord) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO challenges (
  challenge_key, challenge_id, section_name, title, category, challenge_kind,
  expects_attachments, expects_remote, asset_warnings_json, dir_name, dir_path, detail_url,
  description_path, description_sha256, remote_summary_path, remote_sha256, remote_targets_json,
  fingerprint, changed, attachments_json, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(challenge_key) DO UPDATE SET
  challenge_id = excluded.challenge_id,
  section_name = excluded.section_name,
  title = excluded.title,
  category = excluded.category,
  challenge_kind = excluded.challenge_kind,
  expects_attachments = excluded.expects_attachments,
  expects_remote = excluded.expects_remote,
  asset_warnings_json = excluded.asset_warnings_json,
  dir_name = excluded.dir_name,
  dir_path = excluded.dir_path,
  detail_url = excluded.detail_url,
  description_path = excluded.description_path,
  description_sha256 = excluded.description_sha256,
  remote_summary_path = excluded.remote_summary_path,
  remote_sha256 = excluded.remote_sha256,
  remote_targets_json = excluded.remote_targets_json,
  fingerprint = excluded.fingerprint,
  changed = excluded.changed,
  attachments_json = excluded.attachments_json,
  updated_at = excluded.updated_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		warningsJSON := mustJSON(item.AssetWarnings)
		remoteJSON := mustJSON(item.RemoteTargets)
		attachmentsJSON := mustJSON(item.Attachments)
		if _, err = stmt.ExecContext(ctx,
			item.Key, item.ChallengeID, item.Section, item.Title, item.Category, item.ChallengeKind,
			boolInt(item.ExpectsAttachments), boolInt(item.ExpectsRemote), warningsJSON, item.DirName, item.DirPath, item.DetailURL,
			item.DescriptionPath, item.DescriptionSHA256, item.RemoteSummaryPath, item.RemoteSHA256, remoteJSON,
			item.Fingerprint, boolInt(item.Changed), attachmentsJSON, firstNonEmpty(item.UpdatedAt, nowTS()),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *nativeTaskService) upsertChallengeAccounts(ctx context.Context, db *sql.DB, items []challengeAccountRecord) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO challenge_accounts (challenge_key, account_name, data_json, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(challenge_key, account_name) DO UPDATE SET
  data_json = excluded.data_json,
  updated_at = excluded.updated_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, item := range items {
		if _, err = stmt.ExecContext(ctx, item.ChallengeKey, item.AccountName, mustJSON(item.Data), firstNonEmpty(item.UpdatedAt, nowTS())); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *nativeTaskService) updateRuntimeState(ctx context.Context, account accountdomain.Account, loginStatus string, lastError string, success bool) error {
	db := s.challengeRepo.DB()
	if db == nil {
		return nil
	}
	now := nowTS()
	sessionFiles := s.accountSessionFiles(account)
	rawJSON := mustJSON(map[string]any{
		"cookie_file": sessionFiles.CookiePath,
		"token_file":  sessionFiles.TokenPath,
	})
	_, err := db.ExecContext(ctx, `
	INSERT INTO account_runtime (
	  account_name, cycle_status, login_status, last_error, last_login_at,
	  last_cycle_started_at, last_cycle_finished_at, processed_challenges, processed_sections,
	  remote_submission_count, last_remote_submissions_sync_at, session_token_file,
	  session_token_exists, source, raw_json, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, 0, '', ?, ?, 'native-go', ?, ?)
	ON CONFLICT(account_name) DO UPDATE SET
	  cycle_status = excluded.cycle_status,
	  login_status = excluded.login_status,
	  last_error = excluded.last_error,
	  last_login_at = CASE WHEN excluded.login_status = 'ok' THEN excluded.last_login_at ELSE account_runtime.last_login_at END,
	  last_cycle_started_at = excluded.last_cycle_started_at,
	  session_token_file = excluded.session_token_file,
	  session_token_exists = excluded.session_token_exists,
	  raw_json = excluded.raw_json,
	  source = excluded.source,
	  updated_at = excluded.updated_at
	`, account.Name, "running", loginStatus, lastError, now, now, "",
		sessionFiles.TokenPath, boolInt(success && fileExists(sessionFiles.TokenPath)), rawJSON, now)
	return err
}

func (s *nativeTaskService) updateRemoteSubmissionState(ctx context.Context, account accountdomain.Account, count int) error {
	db := s.challengeRepo.DB()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
	UPDATE account_runtime
	SET remote_submission_count = ?,
	    last_remote_submissions_sync_at = ?,
	    updated_at = ?
	WHERE account_name = ?
	`, count, nowTS(), nowTS(), account.Name)
	return err
}

func (s *nativeTaskService) saveProcessedCounters(ctx context.Context, account accountdomain.Account, processedChallenges int, processedSections int) error {
	db := s.challengeRepo.DB()
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `
UPDATE account_runtime
SET cycle_status = 'idle',
    processed_challenges = ?,
    processed_sections = ?,
    last_cycle_finished_at = ?,
    updated_at = ?
WHERE account_name = ?
`, processedChallenges, processedSections, nowTS(), nowTS(), account.Name)
	return err
}

func accountStatusCounts(ctx context.Context, db *sql.DB, accountName string, section string, ids map[string]bool) (int, int, error) {
	rows, err := db.QueryContext(ctx, `
SELECT challenge_key, data_json
FROM challenge_accounts
WHERE account_name = ? AND challenge_key LIKE ?
ORDER BY challenge_key ASC
`, accountName, section+":%")
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	pending := 0
	solved := 0
	for rows.Next() {
		var challengeKey string
		var raw string
		if err := rows.Scan(&challengeKey, &raw); err != nil {
			return 0, 0, err
		}
		data := map[string]any{}
		_ = json.Unmarshal([]byte(raw), &data)
		challengeID := firstString(data["challenge_id"], strings.TrimPrefix(challengeKey, section+":"))
		if challengeID == "" {
			continue
		}
		if len(ids) > 0 && !ids[challengeID] {
			continue
		}
		if submissionConfirmed(data) {
			solved++
			continue
		}
		if strings.TrimSpace(firstString(data["last_flag"])) != "" {
			pending++
		}
	}
	return pending, solved, rows.Err()
}

func mergeChallengeState(existing map[string]any, manifest challengeRecord) map[string]any {
	if existing == nil {
		existing = map[string]any{}
	}
	existing["challenge_id"] = manifest.ChallengeID
	existing["section"] = manifest.Section
	existing["title"] = manifest.Title
	existing["category"] = manifest.Category
	existing["challenge_kind"] = manifest.ChallengeKind
	existing["expects_attachments"] = manifest.ExpectsAttachments
	existing["expects_remote"] = manifest.ExpectsRemote
	existing["asset_warnings"] = manifest.AssetWarnings
	existing["attachments"] = manifest.Attachments
	existing["remote_targets"] = manifest.RemoteTargets
	existing["description_sha256"] = manifest.DescriptionSHA256
	existing["remote_sha256"] = manifest.RemoteSHA256
	existing["fingerprint"] = manifest.Fingerprint
	existing["changed"] = manifest.Changed
	existing["last_seen_at"] = nowTS()
	return existing
}

func reconcileChallengeState(existing map[string]any, manifest challengeRecord) (map[string]any, bool, string, string) {
	if existing == nil {
		existing = map[string]any{}
	}
	resetNeeded, previousSignature, currentSignature := shouldResetForAttachmentMismatch(existing, manifest)
	if resetNeeded {
		existing = resetChallengeStateForCurrentAssets(existing, manifest, "attachment_signature_mismatch", previousSignature)
	}
	existing = mergeChallengeState(existing, manifest)
	existing["attachment_signature"] = currentSignature
	return existing, resetNeeded, previousSignature, currentSignature
}

func updateSolverState(existing map[string]any, challenge challengeRecord, solver solverResult, flagValue string) map[string]any {
	existing = mergeChallengeState(existing, challenge)
	existing["solver"] = solver
	existing["last_solver_fingerprint"] = challenge.Fingerprint
	existing["attachment_signature"] = attachmentSignature(challenge.Attachments)
	if flagValue != "" {
		existing["last_flag"] = flagValue
	}
	return existing
}

func recordSubmitSkipState(existing map[string]any, challenge challengeRecord, flagValue string, result map[string]any) map[string]any {
	existing = mergeChallengeState(existing, challenge)
	existing["submission"] = result
	existing["last_submit_skipped_at"] = nowTS()
	if flagValue != "" {
		existing["last_submitted_flag"] = flagValue
	}
	message := firstString(result["message"])
	if message == "already_submitted" || message == "duplicate_submit" {
		existing["last_submit_ok"] = true
		existing["platform_solved"] = true
		existing["platform_submission"] = map[string]any{
			"challenge_id": challenge.ChallengeID,
			"title":        challenge.Title,
			"category":     challenge.Category,
			"source":       "submit_skip",
			"message":      message,
		}
		delete(existing, "force_reprocess")
		delete(existing, "force_reprocess_reason")
		delete(existing, "force_reprocess_at")
	}
	return existing
}

func recordSubmitState(existing map[string]any, challenge challengeRecord, flagValue string, result map[string]any) map[string]any {
	existing = mergeChallengeState(existing, challenge)
	message := firstString(result["message"])
	submitConfirmed := boolValue(result["accepted"]) || message == "already_submitted" || message == "duplicate_submit"
	existing["submission"] = result
	existing["last_submit_ok"] = submitConfirmed
	existing["last_submitted_at"] = nowTS()
	if flagValue != "" {
		existing["last_submitted_flag"] = flagValue
	}
	existing["last_submitted_fingerprint"] = challenge.Fingerprint
	if submitConfirmed {
		existing["platform_solved"] = true
		existing["platform_solved_at"] = firstString(existing["last_submitted_at"])
		existing["platform_submission"] = map[string]any{
			"challenge_id": challenge.ChallengeID,
			"title":        challenge.Title,
			"category":     challenge.Category,
			"submitted_at": firstString(existing["last_submitted_at"]),
			"source":       "submit_response",
			"message":      firstNonEmpty(message, "success"),
		}
		delete(existing, "force_reprocess")
		delete(existing, "force_reprocess_reason")
		delete(existing, "force_reprocess_at")
	}
	return existing
}

func shouldSolve(state map[string]any, fingerprint string, challengeKind string, realtimeKeywords []string) bool {
	if boolValue(state["force_reprocess"]) {
		return true
	}
	if isDynamicFlagChallenge(challengeKind, realtimeKeywords) {
		return !submissionConfirmed(state)
	}
	if firstString(state["last_solver_fingerprint"]) != fingerprint {
		return true
	}
	if firstString(state["last_flag"]) == "" {
		return true
	}
	solver, _ := state["solver"].(map[string]any)
	status := strings.ToLower(strings.TrimSpace(firstString(solver["status"])))
	return status != "ok" && status != "reused"
}

func solveSkipReason(state map[string]any, challenge challengeRecord, realtimeKeywords []string) string {
	if boolValue(state["force_reprocess"]) {
		return "force_reprocess"
	}
	if isDynamicFlagChallenge(challenge.ChallengeKind, realtimeKeywords) && submissionConfirmed(state) {
		return "dynamic_already_submitted"
	}
	if firstString(state["last_solver_fingerprint"]) != challenge.Fingerprint {
		return "stale_fingerprint"
	}
	if firstString(state["last_flag"]) == "" {
		return "missing_flag"
	}
	solver, _ := state["solver"].(map[string]any)
	status := strings.ToLower(strings.TrimSpace(firstString(solver["status"])))
	if status == "ok" || status == "reused" {
		return "cached_solver_result"
	}
	return firstNonEmpty(status, "unknown")
}

func challengeNeedsResync(challenge challengeRecord) bool {
	if strings.TrimSpace(challenge.DirPath) == "" {
		return true
	}
	if _, err := os.Stat(challenge.DirPath); err != nil {
		return true
	}
	if firstExisting(challenge.DirPath, "solve.py", "solver.py", "exp.py", "exploit.py", "poc.py") != "" {
		return false
	}
	if len(challenge.Attachments) > 0 || challenge.ExpectsAttachments {
		return true
	}
	return false
}

func buildSolverArgs(challenge challengeRecord) []string {
	attachment := firstExistingAttachment(challenge)
	if attachment != "" {
		return []string{attachment}
	}
	if fileExists(filepath.Join(challenge.DirPath, "remote.txt")) {
		return []string{filepath.Join("challenge", "remote.txt")}
	}
	return nil
}

func firstExistingAttachment(challenge challengeRecord) string {
	if strings.TrimSpace(challenge.DirPath) == "" {
		return ""
	}
	attachmentsDir := filepath.Join(challenge.DirPath, "attachments")
	if !fileExists(attachmentsDir) {
		return ""
	}
	for _, item := range challenge.Attachments {
		name := filepath.Base(strings.TrimSpace(item.LocalPath))
		if name == "" {
			name = filepath.Base(strings.TrimSpace(item.StoredName))
		}
		if name == "" {
			name = filepath.Base(strings.TrimSpace(item.Name))
		}
		if name == "" {
			continue
		}
		if fileExists(filepath.Join(attachmentsDir, name)) {
			return filepath.Join("challenge", "attachments", name)
		}
	}
	entries, err := os.ReadDir(attachmentsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		return filepath.Join("challenge", "attachments", entry.Name())
	}
	return ""
}

func extractShortSolverFailure(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "traceback") {
			continue
		}
		return tailText(line, 300)
	}
	return ""
}

func stateAttachmentSignature(state map[string]any) string {
	if state == nil {
		return ""
	}
	signature := strings.TrimSpace(firstString(state["attachment_signature"]))
	if signature != "" {
		return signature
	}
	switch current := state["attachments"].(type) {
	case []attachmentRecord:
		return attachmentSignature(current)
	case []any:
		items := make([]attachmentRecord, 0, len(current))
		for _, raw := range current {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			items = append(items, attachmentRecord{SHA256: firstString(item["sha256"])})
		}
		return attachmentSignature(items)
	default:
		return ""
	}
}

func hasReprocessableChallengeState(state map[string]any) bool {
	if state == nil {
		return false
	}
	keys := []string{
		"last_flag",
		"solver",
		"submission",
		"platform_submission",
		"last_submit_ok",
		"platform_solved",
		"last_submitted_flag",
		"last_submitted_at",
		"last_submitted_fingerprint",
		"last_solver_fingerprint",
	}
	for _, key := range keys {
		value, ok := state[key]
		if !ok || value == nil {
			continue
		}
		switch current := value.(type) {
		case string:
			if strings.TrimSpace(current) != "" {
				return true
			}
		case bool:
			if current {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func shouldResetForAttachmentMismatch(state map[string]any, challenge challengeRecord) (bool, string, string) {
	currentSignature := attachmentSignature(challenge.Attachments)
	if currentSignature == "" {
		return false, "", currentSignature
	}
	previousSignature := stateAttachmentSignature(state)
	if previousSignature == "" || previousSignature == currentSignature {
		return false, previousSignature, currentSignature
	}
	if !hasReprocessableChallengeState(state) {
		return false, previousSignature, currentSignature
	}
	return true, previousSignature, currentSignature
}

func resetChallengeStateForCurrentAssets(state map[string]any, challenge challengeRecord, reason string, previousSignature string) map[string]any {
	if state == nil {
		state = map[string]any{}
	}
	previousFingerprint := firstString(state["last_solver_fingerprint"], state["fingerprint"])
	for _, key := range []string{
		"last_flag",
		"solver",
		"submission",
		"submission_message",
		"last_submit_ok",
		"last_submit_skipped_at",
		"last_submitted_flag",
		"last_submitted_at",
		"last_submitted_fingerprint",
		"platform_solved",
		"platform_solved_at",
		"platform_submission",
	} {
		delete(state, key)
	}
	state["force_reprocess"] = true
	state["force_reprocess_reason"] = reason
	state["force_reprocess_at"] = nowTS()
	state["force_reprocess_previous_attachment_signature"] = previousSignature
	state["force_reprocess_previous_fingerprint"] = previousFingerprint
	state["challenge_id"] = challenge.ChallengeID
	state["section"] = challenge.Section
	state["title"] = challenge.Title
	return state
}

func shouldSubmit(state map[string]any, challenge challengeRecord, flagValue string, manual bool, realtimeKeywords []string) (bool, string) {
	if boolValue(state["force_reprocess"]) {
		return true, "ready_after_reset"
	}
	if submissionConfirmed(state) {
		return false, "already_submitted"
	}
	if manual {
		return true, "ready"
	}
	if firstString(state["last_solver_fingerprint"]) != challenge.Fingerprint {
		return false, "stale_fingerprint"
	}
	if boolValue(state["last_submit_ok"]) && firstString(state["last_submitted_fingerprint"]) == challenge.Fingerprint && firstString(state["last_submitted_flag"]) == flagValue {
		return false, "duplicate_submit"
	}
	return true, "ready"
}

func submissionConfirmed(entry map[string]any) bool {
	if boolValue(entry["platform_solved"]) || boolValue(entry["last_submit_ok"]) {
		return true
	}
	submission, _ := entry["submission"].(map[string]any)
	message := firstString(submission["message"], entry["submission_message"])
	return message == "success" || message == "already_submitted" || message == "duplicate_submit"
}

func (s *nativeTaskService) findSharedSolution(ctx context.Context, db *sql.DB, accountName string, challenge challengeRecord, realtimeKeywords []string) (sharedSolution, error) {
	if db == nil || isDynamicFlagChallenge(challenge.ChallengeKind, realtimeKeywords) || challengeHasPerAccountAssets(challenge) {
		return sharedSolution{}, nil
	}
	targetSignature := attachmentSignature(challenge.Attachments)
	if targetSignature == "" {
		return sharedSolution{}, nil
	}
	rows, err := db.QueryContext(ctx, `
	SELECT account_name, data_json
	FROM challenge_accounts
	WHERE challenge_key = ? AND account_name <> ?
	ORDER BY account_name ASC
	`, challenge.Key, accountName)
	if err != nil {
		return sharedSolution{}, err
	}
	defer rows.Close()

	best := sharedSolution{}
	bestPriority := -1
	for rows.Next() {
		var sourceAccount string
		var raw string
		if err := rows.Scan(&sourceAccount, &raw); err != nil {
			return sharedSolution{}, err
		}
		state := map[string]any{}
		if strings.TrimSpace(raw) != "" {
			if err := json.Unmarshal([]byte(raw), &state); err != nil {
				continue
			}
		}
		if stateAttachmentSignature(state) != targetSignature {
			continue
		}
		flagValue := strings.TrimSpace(firstString(state["last_flag"]))
		if flagValue == "" {
			continue
		}
		solver, _ := state["solver"].(map[string]any)
		solverStatus := strings.ToLower(strings.TrimSpace(firstString(solver["status"])))
		priority := 0
		if submissionConfirmed(state) {
			priority += 4
		}
		if solverStatus == "ok" {
			priority += 2
		}
		if solverStatus == "reused" {
			priority++
		}
		if priority <= 0 {
			continue
		}
		if priority > bestPriority || (priority == bestPriority && strings.ToLower(sourceAccount) < strings.ToLower(best.SourceAccount)) {
			bestPriority = priority
			best = sharedSolution{
				Flag:          flagValue,
				SourceAccount: sourceAccount,
				SourceState:   state,
			}
		}
	}
	return best, rows.Err()
}

func adoptSharedSolutionState(state map[string]any, challenge challengeRecord, solution sharedSolution) (map[string]any, bool) {
	if state == nil {
		state = map[string]any{}
	}
	if strings.TrimSpace(solution.Flag) == "" {
		return state, false
	}
	targetSignature := attachmentSignature(challenge.Attachments)
	currentSolver, _ := state["solver"].(map[string]any)
	if strings.TrimSpace(firstString(state["last_flag"])) == strings.TrimSpace(solution.Flag) &&
		strings.TrimSpace(firstString(state["attachment_signature"])) == targetSignature &&
		strings.ToLower(strings.TrimSpace(firstString(currentSolver["status"]))) == "reused" &&
		strings.TrimSpace(firstString(currentSolver["source_account"])) == strings.TrimSpace(solution.SourceAccount) &&
		strings.TrimSpace(firstString(state["last_solver_fingerprint"])) == strings.TrimSpace(challenge.Fingerprint) {
		return state, false
	}
	state = mergeChallengeState(state, challenge)
	state["attachment_signature"] = targetSignature
	state["last_flag"] = strings.TrimSpace(solution.Flag)
	state["last_solver_fingerprint"] = challenge.Fingerprint
	sourceSolver, _ := solution.SourceState["solver"].(map[string]any)
	state["solver"] = map[string]any{
		"status":               "reused",
		"flag":                 strings.TrimSpace(solution.Flag),
		"reused_at":            nowTS(),
		"reason":               "same_attachment_hash",
		"source_account":       strings.TrimSpace(solution.SourceAccount),
		"source_solver_status": firstString(sourceSolver["status"]),
		"source_fingerprint":   firstString(solution.SourceState["fingerprint"]),
	}
	return state, true
}

func attachmentSignature(items []attachmentRecord) string {
	hashes := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.SHA256) != "" {
			hashes = append(hashes, strings.TrimSpace(item.SHA256))
		}
	}
	sort.Strings(hashes)
	if len(hashes) == 0 {
		return ""
	}
	return stableJSONHash(hashes)
}

func challengeHasPerAccountAssets(challenge challengeRecord) bool {
	return challenge.ExpectsAttachments || len(challenge.Attachments) > 0
}

func attachmentsChanged(items []attachmentRecord) bool {
	for _, item := range items {
		if item.Changed {
			return true
		}
	}
	return false
}

func selectedSections(raw string) []sectionConfig {
	names := parseIDList(raw)
	if len(names) == 0 {
		return []sectionConfig{nativeSections["challenges"], nativeSections["arena"]}
	}
	results := make([]sectionConfig, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		cfg, ok := nativeSections[name]
		if !ok || seen[name] {
			continue
		}
		seen[name] = true
		results = append(results, cfg)
	}
	return results
}

func filterAccounts(accounts []accountdomain.Account, raw string) []accountdomain.Account {
	allowed := parseNameSet(raw)
	result := make([]accountdomain.Account, 0, len(accounts))
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		if len(allowed) > 0 && !allowed[account.Name] && !allowed[account.Username] {
			continue
		}
		result = append(result, account)
	}
	return result
}

func parseNameSet(raw string) map[string]bool {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]bool, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		result[value] = true
	}
	return result
}

func parseIDSet(raw string) map[string]bool {
	items := parseIDList(raw)
	if len(items) == 0 {
		return nil
	}
	result := make(map[string]bool, len(items))
	for _, item := range items {
		result[item] = true
	}
	return result
}

func parseIDList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	result := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizeTags(value any) []string {
	switch current := value.(type) {
	case []any:
		result := make([]string, 0, len(current))
		for _, item := range current {
			text := collapseText(fmt.Sprint(item))
			if text != "" {
				result = append(result, text)
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(current))
		for _, item := range current {
			text := collapseText(item)
			if text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		text := strings.TrimSpace(current)
		if text == "" {
			return nil
		}
		parts := regexp.MustCompile(`[,/| ]+`).Split(text, -1)
		result := make([]string, 0, len(parts))
		for _, item := range parts {
			item = strings.TrimSpace(item)
			if item != "" {
				result = append(result, item)
			}
		}
		return result
	default:
		return nil
	}
}

func detectChallengeKind(section string, title string, category string, tags []string) string {
	normalizedCategory := strings.ToLower(collapseText(category))
	normalizedSection := strings.ToLower(collapseText(section))
	for _, kind := range []string{"mobile", "re", "pwn", "web", "misc"} {
		if normalizedCategory == kind || normalizedSection == kind {
			return kind
		}
		if kind == "re" && (normalizedCategory == "reverse" || normalizedCategory == "reverse engineering") {
			return "re"
		}
	}
	haystack := " " + strings.ToLower(strings.Join(append([]string{section, title, category}, tags...), " ")) + " "
	rules := []struct {
		Kind   string
		Tokens []string
	}{
		{"mobile", []string{" mobile", "android", "ios", "apk", "ipa"}},
		{"re", []string{" reverse", "re ", "rev", "逆向"}},
		{"pwn", []string{"pwn", "heap", "rop", "fmt", "ret2"}},
		{"web", []string{"web", "flask", "php", "node", "http"}},
		{"misc", []string{"misc"}},
	}
	for _, rule := range rules {
		for _, token := range rule.Tokens {
			if strings.Contains(haystack, token) {
				return rule.Kind
			}
		}
	}
	if normalizedCategory != "" {
		return normalizedCategory
	}
	if normalizedSection != "" {
		return normalizedSection
	}
	return "unknown"
}

func challengeExpectations(kind string) (bool, bool) {
	lowered := strings.ToLower(strings.TrimSpace(kind))
	expectsAttachments := lowered == "re" || lowered == "reverse" || lowered == "rev" || lowered == "mobile"
	expectsRemote := lowered == "web" || lowered == "pwn"
	return expectsAttachments, expectsRemote
}

func challengeAssetWarnings(challenge fetchedChallenge) []string {
	result := make([]string, 0, 2)
	if challenge.ExpectsAttachments && len(challenge.Attachments) == 0 {
		result = append(result, "expected_attachments_missing")
	}
	if challenge.ExpectsRemote && len(challenge.RemoteTargets) == 0 {
		result = append(result, "expected_remote_missing")
	}
	return result
}

func extractAttachmentsFromPayload(payload any, client *taskHTTPClient) []attachmentRecord {
	found := make([]attachmentRecord, 0, 4)
	seen := map[string]bool{}
	var walk func(any)
	add := func(name string, rawURL string) {
		if strings.TrimSpace(rawURL) == "" {
			return
		}
		absolute := client.absoluteURL(rawURL)
		if !looksLikeFileURL(absolute) {
			return
		}
		if seen[absolute] {
			return
		}
		seen[absolute] = true
		pathName := path.Base(mustURLPath(absolute))
		rawName := strings.TrimSpace(collapseText(name))
		if rawName == "" || rawName == rawURL || rawName == absolute || strings.HasPrefix(rawName, "http://") || strings.HasPrefix(rawName, "https://") || strings.HasPrefix(rawName, "/") {
			rawName = pathName
		}
		found = append(found, attachmentRecord{
			Name: sanitizePathComponent(firstNonEmpty(rawName, pathName, "attachment.bin")),
			URL:  absolute,
		})
	}
	walk = func(value any) {
		switch current := value.(type) {
		case map[string]any:
			var directURL string
			for _, key := range []string{"url", "href", "file", "src", "path", "location", "download", "attachment"} {
				if candidate := directAttachmentURL(current[key]); candidate != "" {
					directURL = candidate
					break
				}
			}
			directName := firstString(current["name"], current["filename"], current["title"], current["file_name"], current["label"])
			if directURL != "" && looksLikeFileURL(directURL) {
				add(directName, directURL)
			}
			for key, nested := range current {
				lowered := strings.ToLower(strings.TrimSpace(key))
				if lowered == "files" || lowered == "attachments" || lowered == "attachment" || lowered == "downloads" {
					walk(nested)
					continue
				}
				if (lowered == "description" || lowered == "desc" || lowered == "html" || lowered == "content" || lowered == "message") && strings.TrimSpace(firstString(nested)) != "" {
					for _, item := range extractAttachmentsFromHTML(firstString(nested), nativeBaseURL, client) {
						add(item.Name, item.URL)
					}
					continue
				}
				walk(nested)
			}
		case []any:
			for _, item := range current {
				walk(item)
			}
		case string:
			for _, candidate := range extractAttachmentRefsFromText(current) {
				add(candidate, candidate)
			}
			for _, item := range extractAttachmentsFromHTML(current, nativeBaseURL, client) {
				add(item.Name, item.URL)
			}
		}
	}
	walk(payload)
	return found
}

func extractAttachmentsFromHTML(htmlText string, currentURL string, client *taskHTTPClient) []attachmentRecord {
	root, err := xhtml.Parse(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	results := make([]attachmentRecord, 0, 4)
	seen := map[string]bool{}
	var walk func(*xhtml.Node)
	add := func(name string, rawURL string) {
		href := resolveURL(currentURL, rawURL)
		if looksLikeFileURL(href) && !seen[href] {
			seen[href] = true
			results = append(results, attachmentRecord{
				Name: sanitizePathComponent(firstNonEmpty(name, path.Base(mustURLPath(href)), "attachment.bin")),
				URL:  href,
			})
		}
	}
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			textName := collapseText(textContent(node))
			for _, attrName := range []string{"href", "src", "data-url", "data-href", "download"} {
				raw := attrValue(node, attrName)
				if strings.TrimSpace(raw) == "" {
					continue
				}
				add(textName, raw)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	for _, candidate := range extractAttachmentRefsFromText(htmlText) {
		add(candidate, candidate)
	}
	return results
}

func extractRemoteTargetsFromPayload(payload any) []remoteTarget {
	found := make([]remoteTarget, 0, 4)
	var walk func(any, string)
	addValue := func(value string, source string) {
		if target, ok := remoteTargetFromValue(value, source); ok {
			found = append(found, target)
		}
		found = append(found, extractRemoteTargetsFromText(value, source)...)
	}
	walk = func(value any, source string) {
		switch current := value.(type) {
		case map[string]any:
			for key, nested := range current {
				keyName := strings.ToLower(strings.TrimSpace(key))
				if text := firstString(nested); text != "" {
					switch keyName {
					case "remote", "target", "instance", "endpoint", "url", "host", "hostname", "addr", "address", "nc":
						addValue(text, keyName)
					case "port":
						host := firstString(current["host"], current["hostname"], current["addr"], current["address"])
						if host != "" {
							addValue(host+":"+text, "host-port")
						}
					case "description", "desc", "html", "content", "message":
						found = append(found, extractRemoteTargetsFromHTML(text, nativeBaseURL)...)
					default:
						found = append(found, extractRemoteTargetsFromText(text, keyName)...)
					}
				} else {
					walk(nested, keyName)
				}
			}
		case []any:
			for _, item := range current {
				walk(item, source)
			}
		case string:
			found = append(found, extractRemoteTargetsFromText(current, source)...)
		}
	}
	walk(payload, "payload")
	return dedupeRemoteTargets(found)
}

func extractRemoteTargetsFromHTML(htmlText string, currentURL string) []remoteTarget {
	root, err := xhtml.Parse(strings.NewReader(htmlText))
	if err != nil {
		return nil
	}
	found := make([]remoteTarget, 0, 4)
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "a" {
			href := resolveURL(currentURL, attrValue(node, "href"))
			if !looksLikeFileURL(href) {
				if target, ok := remoteTargetFromValue(href, "html-link"); ok {
					found = append(found, target)
				}
			}
			found = append(found, extractRemoteTargetsFromText(collapseText(textContent(node)), "html-link-text")...)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	found = append(found, extractRemoteTargetsFromText(stripHTML(htmlText), "html-text")...)
	return dedupeRemoteTargets(found)
}

func extractRemoteTargetsFromText(text string, source string) []remoteTarget {
	results := make([]remoteTarget, 0, 8)
	seen := map[string]bool{}
	patterns := []*regexp.Regexp{reRemoteURL, reRemoteAltURL, reRemoteHostPort, reRemoteDomainPort}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllString(text, -1) {
			if target, ok := remoteTargetFromValue(match, source); ok {
				key := target.Kind + "|" + target.Value
				if !seen[key] {
					seen[key] = true
					results = append(results, target)
				}
			}
		}
	}
	return results
}

func remoteTargetFromValue(value string, source string) (remoteTarget, bool) {
	normalized := normalizeRemoteValue(value)
	if !looksLikeRemoteValue(normalized) {
		return remoteTarget{}, false
	}
	lowered := strings.ToLower(normalized)
	if strings.HasPrefix(lowered, "http://") || strings.HasPrefix(lowered, "https://") || strings.HasPrefix(lowered, "ws://") || strings.HasPrefix(lowered, "wss://") || strings.HasPrefix(lowered, "tcp://") || strings.HasPrefix(lowered, "nc://") {
		parsed, err := url.Parse(normalized)
		if err != nil {
			return remoteTarget{}, false
		}
		port := 0
		if parsed.Port() != "" {
			port, _ = strconv.Atoi(parsed.Port())
		}
		return remoteTarget{
			Value:  normalized,
			Kind:   firstNonEmpty(parsed.Scheme, "url"),
			Source: source,
			Host:   parsed.Hostname(),
			Port:   port,
		}, true
	}
	match := reRemoteHostPort.FindString(normalized)
	if match == "" {
		match = reRemoteDomainPort.FindString(normalized)
	}
	if match == "" {
		return remoteTarget{}, false
	}
	host, portText, err := net.SplitHostPort(match)
	if err != nil {
		return remoteTarget{}, false
	}
	port, _ := strconv.Atoi(portText)
	return remoteTarget{
		Value:  host + ":" + strconv.Itoa(port),
		Kind:   "tcp",
		Source: source,
		Host:   host,
		Port:   port,
	}, true
}

func dedupeRemoteTargets(items []remoteTarget) []remoteTarget {
	seen := map[string]bool{}
	result := make([]remoteTarget, 0, len(items))
	for _, item := range items {
		key := item.Kind + "|" + item.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func looksLikeRemoteValue(value string) bool {
	lowered := strings.ToLower(normalizeRemoteValue(value))
	if lowered == "" {
		return false
	}
	if looksLikeFileURL(lowered) {
		return false
	}
	return strings.HasPrefix(lowered, "http://") ||
		strings.HasPrefix(lowered, "https://") ||
		strings.HasPrefix(lowered, "ws://") ||
		strings.HasPrefix(lowered, "wss://") ||
		strings.HasPrefix(lowered, "tcp://") ||
		strings.HasPrefix(lowered, "nc://") ||
		reRemoteHostPort.MatchString(lowered) ||
		reRemoteDomainPort.MatchString(lowered)
}

func normalizeRemoteValue(value string) string {
	return strings.TrimRight(collapseText(value), "/")
}

func normalizeRemoteJSON(items []remoteTarget) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"value": item.Value,
			"kind":  item.Kind,
			"host":  item.Host,
			"port":  item.Port,
		})
	}
	return result
}

func normalizeAttachmentJSON(items []attachmentRecord) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"url":    item.URL,
			"sha256": item.SHA256,
			"size":   item.Size,
		})
	}
	return result
}

func extractAttachmentRefsFromText(text string) []string {
	matches := reFileRef.FindAllString(text, -1)
	results := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		match = strings.TrimSpace(strings.Trim(match, `"'`))
		if match == "" || seen[match] || !looksLikeFileURL(match) {
			continue
		}
		seen[match] = true
		results = append(results, match)
	}
	return results
}

func directAttachmentURL(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" || !looksLikeFileURL(text) {
		return ""
	}
	return text
}

func looksLikeFileURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	lowered := strings.ToLower(parsed.Path)
	if strings.Contains(lowered, "/static/uploads/") || strings.Contains(lowered, "/uploads/") || strings.Contains(lowered, "/file") || strings.Contains(lowered, "/files") || strings.Contains(lowered, "/download") || strings.Contains(lowered, "/attachment") || strings.Contains(lowered, "/attachments") {
		return true
	}
	return reFileURLSuffix.MatchString(lowered)
}

func normalizedWorkers(value int) int {
	if value <= 0 {
		return 2
	}
	if value > 2 {
		return 2
	}
	return value
}

func remoteSyncRecentlyFresh(account accountdomain.Account, section sectionConfig) bool {
	if account.Runtime == nil {
		return false
	}
	lastSync := strings.TrimSpace(account.Runtime.LastRemoteSubmissionsSync)
	if lastSync == "" {
		return false
	}
	lastAt, err := time.ParseInLocation("2006-01-02 15:04:05", lastSync, time.Local)
	if err != nil {
		return false
	}
	if time.Since(lastAt) > nativeCacheFreshTTL {
		return false
	}
	return section.Name == "challenges" || section.Name == "arena"
}

func waitAccountCooldown(ctx context.Context, logger taskLogger, accountName string) error {
	timer := time.NewTimer(nativeAccountCooldown)
	defer timer.Stop()
	if logger != nil {
		logger.Printf("%s cooldown wait=%s", accountName, nativeAccountCooldown)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func challengeRecordLess(left string, right string, leftFallback string, rightFallback string) bool {
	leftNumber := numericPrefix(left)
	rightNumber := numericPrefix(right)
	if leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return strings.ToLower(leftFallback) < strings.ToLower(rightFallback)
}

func isPlaceholderChallengeTitle(title string, challengeID string) bool {
	normalized := strings.ToLower(strings.TrimSpace(collapseText(title)))
	if normalized == "" {
		return true
	}
	if challengeID != "" && normalized == strings.ToLower(strings.TrimSpace(challengeID)) {
		return true
	}
	if reMeaningfulTitle.MatchString(normalized) {
		return true
	}
	cid := regexp.QuoteMeta(strings.ToLower(strings.TrimSpace(challengeID)))
	if cid != "" {
		for _, pattern := range []string{
			`^(?:challenge|challenges|chal|task|problem)[ _-]*` + cid + `$`,
			`^题目[ _-]*` + cid + `$`,
		} {
			if regexp.MustCompile(pattern).MatchString(normalized) {
				return true
			}
		}
	}
	return false
}

func isRealtimeCandidate(kind string, category string, title string, tags []string, keywords []string) bool {
	if len(keywords) == 0 {
		keywords = []string{"web", "pwn"}
	}
	haystack := strings.ToLower(strings.Join(append([]string{kind, category, title}, tags...), " "))
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(haystack, keyword) {
			return true
		}
	}
	return false
}

func isDynamicFlagChallenge(kind string, realtimeKeywords []string) bool {
	return isRealtimeCandidate(kind, "", "", nil, realtimeKeywords) || strings.ToLower(strings.TrimSpace(kind)) == "web" || strings.ToLower(strings.TrimSpace(kind)) == "pwn"
}

func stripHTML(value string) string {
	text := regexp.MustCompile(`(?is)<script.*?>.*?</script>`).ReplaceAllString(value, " ")
	text = regexp.MustCompile(`(?is)<style.*?>.*?</style>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`(?is)</p>`).ReplaceAllString(text, "\n\n")
	text = reHTMLTag.ReplaceAllString(text, " ")
	text = stdhtml.UnescapeString(text)
	text = regexp.MustCompile(`[ \t]+\n`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func collapseText(value string) string {
	value = stdhtml.UnescapeString(value)
	value = reWhitespace.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func sanitizePathComponent(value string) string {
	value = collapseText(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	replacer := strings.NewReplacer(":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, ". ")
	if value == "" {
		return "untitled"
	}
	return value
}

func resolveURL(base string, ref string) string {
	baseURL, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return strings.TrimSpace(ref)
	}
	refURL, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return strings.TrimSpace(ref)
	}
	return baseURL.ResolveReference(refURL).String()
}

func mustURLPath(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	return parsed.Path
}

func ioReadAllAndClose(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func attrValue(node *xhtml.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func textContent(node *xhtml.Node) string {
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func looksLikeLoginPage(urlPath string, body string) bool {
	urlPath = strings.TrimSpace(urlPath)
	if strings.TrimRight(urlPath, "/") == "/login" {
		return true
	}
	return strings.Contains(body, "<h1>登录</h1>") && strings.Contains(body, "password")
}

func stableJSONHash(value any) string {
	data, _ := json.Marshal(value)
	return sha256Bytes(data)
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256Text(value string) string {
	return sha256Bytes([]byte(value))
}

func fileSHA256(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()
	hasher := sha256.New()
	_, _ = io.Copy(hasher, file)
	return hex.EncodeToString(hasher.Sum(nil))
}

func buildRemoteSummary(items []remoteTarget) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, item.Value)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func formatUnixTS(value any) string {
	switch current := value.(type) {
	case nil:
		return ""
	case float64:
		return time.Unix(int64(current), 0).Format("2006-01-02 15:04:05")
	case int64:
		return time.Unix(current, 0).Format("2006-01-02 15:04:05")
	case int:
		return time.Unix(int64(current), 0).Format("2006-01-02 15:04:05")
	case string:
		text := strings.TrimSpace(current)
		if text == "" {
			return ""
		}
		if iv, err := strconv.ParseInt(text, 10, 64); err == nil {
			return time.Unix(iv, 0).Format("2006-01-02 15:04:05")
		}
	}
	return ""
}

func copyFileRaw(source string, target string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	defer targetFile.Close()
	_, err = io.Copy(targetFile, sourceFile)
	return err
}

func newAccountTransport(proxySettings NetworkProxySettings, timeout time.Duration) (*http.Transport, error) {
	proxyFunc := http.ProxyFromEnvironment
	if proxySettings.Enabled {
		proxyURL, err := buildProxyURL(proxySettings)
		if err != nil {
			return nil, err
		}
		proxyFunc = http.ProxyURL(proxyURL)
	}
	transport := &http.Transport{
		Proxy:                 proxyFunc,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   10 * time.Second,
		DisableKeepAlives:     false,
	}
	return transport, nil
}

func shouldRetryHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		521, 522, 523, 524:
		return true
	default:
		return false
	}
}

func waitRetry(ctx context.Context, attempt int, retryAfter string) error {
	delay := retryDelay(attempt, retryAfter)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > 30*time.Second {
			return 30 * time.Second
		}
		return delay
	}
	if retryAfter != "" {
		if at, err := http.ParseTime(strings.TrimSpace(retryAfter)); err == nil {
			delay := time.Until(at)
			if delay > 0 {
				if delay > 30*time.Second {
					return 30 * time.Second
				}
				return delay
			}
		}
	}
	delay := time.Second << maxInt(0, attempt-1)
	if delay > 8*time.Second {
		return 8 * time.Second
	}
	return delay
}

func buildProxyURL(settings NetworkProxySettings) (*url.URL, error) {
	settings, err := normalizeNetworkProxy(settings)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, nil
	}
	host := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	proxyURL := &url.URL{
		Scheme: settings.Type,
		Host:   host,
	}
	if settings.Username != "" || settings.Password != "" {
		proxyURL.User = url.UserPassword(settings.Username, settings.Password)
	}
	return proxyURL, nil
}

func firstString(values ...any) string {
	for _, value := range values {
		switch current := value.(type) {
		case nil:
		case string:
			if strings.TrimSpace(current) != "" {
				return strings.TrimSpace(current)
			}
		case float64:
			return strconv.FormatInt(int64(current), 10)
		case int:
			return strconv.Itoa(current)
		case int64:
			return strconv.FormatInt(current, 10)
		default:
			text := strings.TrimSpace(fmt.Sprint(current))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func boolValue(value any) bool {
	switch current := value.(type) {
	case bool:
		return current
	case string:
		text := strings.ToLower(strings.TrimSpace(current))
		return text == "1" || text == "true"
	case float64:
		return current != 0
	case int:
		return current != 0
	default:
		return false
	}
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func tailText(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}

func displayFlag(flagValue string) string {
	flagValue = strings.TrimSpace(flagValue)
	if flagValue == "" {
		return "-"
	}
	if len(flagValue) <= 16 {
		return flagValue
	}
	return flagValue[:8] + "..." + flagValue[len(flagValue)-4:]
}

func dedupeStrings(items []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func firstN(value string, n int) string {
	if n <= 0 || len(value) <= n {
		return value
	}
	return value[:n]
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func numericPrefix(value string) int {
	var result int
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		result = result*10 + int(r-'0')
	}
	if result == 0 {
		return 999999
	}
	return result
}

func firstExisting(dir string, names ...string) string {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

type persistedCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path,omitempty"`
	Domain   string `json:"domain,omitempty"`
	Expires  string `json:"expires,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
}

func loadPersistedCookies(cookiePath string) []*http.Cookie {
	cookiePath = strings.TrimSpace(cookiePath)
	if cookiePath == "" {
		return nil
	}
	data, err := os.ReadFile(cookiePath)
	if err != nil || len(data) == 0 {
		return nil
	}
	var payload []persistedCookie
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	result := make([]*http.Cookie, 0, len(payload))
	for _, item := range payload {
		cookie := &http.Cookie{
			Name:     strings.TrimSpace(item.Name),
			Value:    item.Value,
			Path:     firstNonEmpty(strings.TrimSpace(item.Path), "/"),
			Domain:   strings.TrimSpace(item.Domain),
			Secure:   item.Secure,
			HttpOnly: item.HttpOnly,
		}
		if item.Expires != "" {
			if expiresAt, err := time.Parse(time.RFC3339, item.Expires); err == nil {
				cookie.Expires = expiresAt
			}
		}
		if cookie.Name != "" {
			result = append(result, cookie)
		}
	}
	return result
}

func savePersistedCookies(cookiePath string, cookies []*http.Cookie) error {
	if err := os.MkdirAll(filepath.Dir(cookiePath), 0o755); err != nil {
		return err
	}
	payload := make([]persistedCookie, 0, len(cookies))
	for _, item := range cookies {
		if item == nil || strings.TrimSpace(item.Name) == "" {
			continue
		}
		entry := persistedCookie{
			Name:     strings.TrimSpace(item.Name),
			Value:    item.Value,
			Path:     item.Path,
			Domain:   item.Domain,
			Secure:   item.Secure,
			HttpOnly: item.HttpOnly,
		}
		if !item.Expires.IsZero() {
			entry.Expires = item.Expires.UTC().Format(time.RFC3339)
		}
		payload = append(payload, entry)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(cookiePath, data, 0o644)
}

func saveTokenFile(tokenPath string, token string) error {
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(tokenPath, []byte(strings.TrimSpace(token)+"\n"), 0o644)
}

func loadTokenFile(tokenPath string) string {
	tokenPath = strings.TrimSpace(tokenPath)
	if tokenPath == "" {
		return ""
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func parseCookieHeader(header string) []*http.Cookie {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ";")
	result := make([]*http.Cookie, 0, len(parts))
	for _, part := range parts {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		result = append(result, &http.Cookie{Name: name, Value: strings.TrimSpace(value), Path: "/"})
	}
	return result
}

func cookieHeaderForCookies(cookies []*http.Cookie) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, item := range cookies {
		if item == nil || strings.TrimSpace(item.Name) == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(item.Name)+"="+item.Value)
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func sessionToken(client *taskHTTPClient) string {
	if client == nil {
		return ""
	}
	return strings.TrimSpace(client.sessionToken())
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
