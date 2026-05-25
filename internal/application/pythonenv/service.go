package pythonenv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	runtimeplatform "got0iscc/desktop/internal/platform/runtime"
	"got0iscc/desktop/internal/platform/storage/sqlite"
)

const (
	metaManagedPythonBinary = "pythonenv.managed_binary"
	metaManagedPythonRoot   = "pythonenv.managed_root"
	metaManagedPythonReady  = "pythonenv.managed_ready"
)

type Service struct {
	layout runtimeplatform.Layout
	store  *sqlite.Store
}

type Status struct {
	Platform            string   `json:"platform"`
	Architecture        string   `json:"architecture"`
	ManagedRoot         string   `json:"managed_root"`
	ManagedPythonBinary string   `json:"managed_python_binary"`
	ActivePythonBinary  string   `json:"active_python_binary"`
	ActiveSource        string   `json:"active_source"`
	DetectedCandidates  []string `json:"detected_candidates"`
	Strategy            string   `json:"strategy"`
	FallbackConfigPath  string   `json:"fallback_config_path"`
	FallbackEnabled     bool     `json:"fallback_enabled"`
	FallbackConfigured  bool     `json:"fallback_configured"`
	FallbackPlatform    string   `json:"fallback_platform"`
	Ready               bool     `json:"ready"`
	InstalledPackages   []string `json:"installed_packages"`
	LastError           string   `json:"last_error,omitempty"`
}

type InitRequest struct {
	PythonBinary string `json:"python_binary"`
}

type InstallPackagesRequest struct {
	Packages []string `json:"packages"`
}

type CommandResult struct {
	OK         bool     `json:"ok"`
	Command    []string `json:"command"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	ExitCode   int      `json:"exit_code"`
	DurationMS int64    `json:"duration_ms"`
}

func NewService(layout runtimeplatform.Layout, store *sqlite.Store) *Service {
	return &Service{
		layout: layout,
		store:  store,
	}
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	detectedCandidates := detectSystemPythonCandidates()
	strategy := "优先使用已初始化的内部 venv；若不存在，则先探测本地 Python；仅在本地探测失败时才进入发行版 fallback"
	managedRoot, managedBinary := s.currentManagedPaths(ctx)
	activeBinary, activeSource, activeErr := s.resolveActivePythonBinary(ctx)
	packages, pkgErr := s.installedPackages(ctx, activeBinary)
	fallbackConfig, fallbackErr := s.loadFallbackConfig()
	status := Status{
		Platform:            runtime.GOOS,
		Architecture:        runtime.GOARCH,
		ManagedRoot:         managedRoot,
		ManagedPythonBinary: managedBinary,
		ActivePythonBinary:  activeBinary,
		ActiveSource:        activeSource,
		DetectedCandidates:  detectedCandidates,
		Strategy:            strategy,
		FallbackConfigPath:  s.fallbackConfigPath(),
		FallbackEnabled:     fallbackConfig.Enabled,
		FallbackConfigured:  fallbackConfig.isConfiguredForCurrentPlatform(),
		FallbackPlatform:    fallbackConfig.platformKey(),
		Ready:               strings.TrimSpace(activeBinary) != "" && pathExists(activeBinary),
		InstalledPackages:   packages,
	}
	errorsList := make([]string, 0, 3)
	if activeErr != nil {
		errorsList = append(errorsList, activeErr.Error())
	}
	if pkgErr != nil {
		errorsList = append(errorsList, pkgErr.Error())
	}
	if fallbackErr != nil {
		errorsList = append(errorsList, fallbackErr.Error())
	}
	if len(errorsList) > 0 {
		status.LastError = strings.Join(errorsList, " | ")
	}
	return status, nil
}

func (s *Service) ActivePythonBinary(ctx context.Context) (string, error) {
	binary, _, err := s.resolveActivePythonBinary(ctx)
	return binary, err
}

func (s *Service) Initialize(ctx context.Context, req InitRequest) (CommandResult, error) {
	if managedBinary, ok := s.existingManagedPython(ctx); ok {
		result, err := s.upgradePip(ctx, managedBinary)
		combined := combineCommandResults(
			"检测到已有内部 Python 环境，已直接复用并升级 pip",
			[]string{"pythonenv", "reuse", "upgrade-pip"},
			result,
		)
		if err != nil {
			return combined, err
		}
		if err := s.saveManagedPython(ctx, s.currentManagedRoot(ctx), managedBinary); err != nil {
			return combined, err
		}
		return combined, nil
	}

	candidate := strings.TrimSpace(req.PythonBinary)
	if candidate != "" {
		resolved, err := resolveRequestedPython(candidate)
		if err != nil {
			return CommandResult{}, err
		}
		return s.initializeManagedVenv(ctx, resolved)
	}

	resolved, err := detectSystemPython()
	if err == nil {
		return s.initializeManagedVenv(ctx, resolved)
	}
	return s.initializeFromFallback(ctx, err)
}

func (s *Service) InstallPackages(ctx context.Context, req InstallPackagesRequest) (CommandResult, error) {
	pythonBinary, err := s.ActivePythonBinary(ctx)
	if err != nil {
		return CommandResult{}, err
	}
	packages := normalizePackages(req.Packages)
	if len(packages) == 0 {
		return CommandResult{}, errors.New("no packages specified")
	}
	command := append([]string{pythonBinary, "-m", "pip", "install"}, packages...)
	return runCommand(ctx, s.layout.AppRoot, command, nil)
}

func (s *Service) managedRoot() string {
	return filepath.Join(s.layout.AppDataRoot, "python")
}

func (s *Service) managedPythonBinary() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(s.managedRoot(), "Scripts", "python.exe")
	}
	return filepath.Join(s.managedRoot(), "bin", "python3")
}

func (s *Service) installedPackages(ctx context.Context, pythonBinary string) ([]string, error) {
	if strings.TrimSpace(pythonBinary) == "" || !pathExists(pythonBinary) {
		return nil, nil
	}
	result, err := runCommand(ctx, s.layout.AppRoot, []string{pythonBinary, "-m", "pip", "list", "--format=freeze"}, nil)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	packages := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		packages = append(packages, line)
	}
	sort.Strings(packages)
	return packages, nil
}

func normalizePackages(items []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(items))
	for _, item := range items {
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

func detectSystemPython() (string, error) {
	candidates := detectSystemPythonCommands()
	for _, candidate := range candidates {
		binary, err := resolvePythonCandidate(candidate)
		if err == nil {
			return binary, nil
		}
	}
	return "", errors.New("no suitable python interpreter found in local environment")
}

func detectSystemPythonCandidates() []string {
	candidates := detectSystemPythonCommands()
	results := make([]string, 0, len(candidates))
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		binary, err := resolvePythonCandidate(candidate)
		if err == nil {
			if _, ok := seen[binary]; ok {
				continue
			}
			seen[binary] = struct{}{}
			results = append(results, binary)
		}
	}
	return results
}

func detectSystemPythonCommands() [][]string {
	candidates := [][]string{}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			[]string{"python"},
			[]string{"py", "-3"},
		)
	} else {
		candidates = append(candidates,
			[]string{"python3"},
			[]string{"python"},
		)
	}
	return candidates
}

func resolvePythonCandidate(command []string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("empty python candidate")
	}
	path, err := exec.LookPath(command[0])
	if err != nil {
		return "", err
	}
	args := append(append([]string{}, command[1:]...), "-c", "import os, sys; print(os.path.realpath(sys.executable))")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		resolved := strings.TrimSpace(stdout.String())
		if resolved != "" && pathExists(resolved) {
			return resolved, nil
		}
	}
	return path, nil
}

func resolveRequestedPython(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty python path")
	}
	if pathExists(value) {
		return value, nil
	}
	if !strings.ContainsAny(value, " \t") {
		return resolvePythonCandidate([]string{value})
	}
	return resolvePythonCandidate(strings.Fields(value))
}

func runCommand(ctx context.Context, workdir string, command []string, env map[string]string) (CommandResult, error) {
	if len(command) == 0 {
		return CommandResult{}, errors.New("empty command")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	execCmd := exec.CommandContext(timeoutCtx, command[0], command[1:]...)
	execCmd.Dir = workdir
	execCmd.Env = os.Environ()
	for key, value := range env {
		execCmd.Env = append(execCmd.Env, key+"="+value)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	started := time.Now()
	runErr := execCmd.Run()
	durationMS := time.Since(started).Milliseconds()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return CommandResult{}, runErr
		}
	}
	result := CommandResult{
		OK:         exitCode == 0,
		Command:    command,
		Stdout:     stdout.String(),
		Stderr:     strings.TrimSpace(stderr.String()),
		ExitCode:   exitCode,
		DurationMS: durationMS,
	}
	if exitCode != 0 {
		return result, fmt.Errorf("command failed with exit code %d", exitCode)
	}
	return result, nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Service) initializeManagedVenv(ctx context.Context, pythonBinary string) (CommandResult, error) {
	if err := os.RemoveAll(s.managedRoot()); err != nil {
		return CommandResult{}, err
	}
	if err := os.MkdirAll(s.managedRoot(), 0o755); err != nil {
		return CommandResult{}, err
	}

	createResult, createErr := runCommand(
		ctx,
		s.layout.AppRoot,
		[]string{pythonBinary, "-m", "venv", s.managedRoot()},
		nil,
	)
	managedBinary := s.managedPythonBinary()
	combined := combineCommandResults(
		"已创建内部 Python venv",
		[]string{"pythonenv", "initialize"},
		createResult,
	)
	if createErr != nil {
		return combined, createErr
	}
	if !pathExists(managedBinary) {
		return combined, fmt.Errorf("managed python not found after init: %s", managedBinary)
	}

	pipResult, pipErr := s.upgradePip(ctx, managedBinary)
	combined = combineCommandResults(
		"内部 Python 已初始化，并已执行 pip install --upgrade pip",
		[]string{"pythonenv", "initialize"},
		createResult,
		pipResult,
	)
	if pipErr != nil {
		return combined, pipErr
	}
	if err := s.saveManagedPython(ctx, s.managedRoot(), managedBinary); err != nil {
		return combined, err
	}
	return combined, nil
}

func (s *Service) upgradePip(ctx context.Context, pythonBinary string) (CommandResult, error) {
	ensureResult, ensureErr := runCommand(ctx, s.layout.AppRoot, []string{pythonBinary, "-m", "ensurepip", "--upgrade"}, nil)
	upgradeResult, upgradeErr := runCommand(ctx, s.layout.AppRoot, []string{pythonBinary, "-m", "pip", "install", "--upgrade", "pip"}, nil)
	combined := combineCommandResults(
		"已执行 ensurepip 与 pip 升级",
		[]string{pythonBinary, "-m", "pip", "install", "--upgrade", "pip"},
		ensureResult,
		upgradeResult,
	)
	if ensureErr != nil && upgradeErr != nil {
		return combined, upgradeErr
	}
	if upgradeErr != nil {
		return combined, upgradeErr
	}
	return combined, nil
}

func (s *Service) saveManagedPython(ctx context.Context, root string, binary string) error {
	if err := s.store.SetMetaValue(ctx, metaManagedPythonRoot, root); err != nil {
		return err
	}
	if err := s.store.SetMetaValue(ctx, metaManagedPythonBinary, binary); err != nil {
		return err
	}
	if err := s.store.SetMetaValue(ctx, metaManagedPythonReady, "1"); err != nil {
		return err
	}
	return nil
}

func (s *Service) currentManagedRoot(ctx context.Context) string {
	if value, _ := s.store.MetaValue(ctx, metaManagedPythonRoot); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return s.managedRoot()
}

func (s *Service) currentManagedPaths(ctx context.Context) (string, string) {
	root := s.currentManagedRoot(ctx)
	if value, _ := s.store.MetaValue(ctx, metaManagedPythonBinary); strings.TrimSpace(value) != "" {
		return root, strings.TrimSpace(value)
	}
	if pathExists(s.managedPythonBinary()) {
		return s.managedRoot(), s.managedPythonBinary()
	}
	return root, ""
}

func (s *Service) existingManagedPython(ctx context.Context) (string, bool) {
	if value, _ := s.store.MetaValue(ctx, metaManagedPythonBinary); strings.TrimSpace(value) != "" && pathExists(value) {
		return value, true
	}
	if pathExists(s.managedPythonBinary()) {
		return s.managedPythonBinary(), true
	}
	return "", false
}

func (s *Service) resolveActivePythonBinary(ctx context.Context) (string, string, error) {
	if binary, ok := s.existingManagedPython(ctx); ok {
		return binary, "managed", nil
	}
	if binary, err := detectSystemPython(); err == nil {
		return binary, "system", nil
	}
	return "", "", errors.New("未找到可用 Python：当前没有内部 venv，本地 Python 探测也失败")
}
