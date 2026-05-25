package python

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Profile struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	Isolation             string `json:"isolation"`
	NetworkPolicy         string `json:"network_policy"`
	DefaultTimeoutSeconds int    `json:"default_timeout_seconds"`
}

type RunRequest struct {
	Code           string
	Files          map[string]string
	CopyPaths      []CopyPath
	Env            map[string]string
	Script         string
	Args           []string
	PythonBinary   string
	TimeoutSeconds int
	Profile        string
}

type CopyPath struct {
	Source string
	Target string
}

type RunResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMS int64
	Workdir    string
	Command    []string
}

type Runner interface {
	Profiles() []Profile
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

type LocalRunner struct {
	sandboxRoot string
}

func NewLocalRunner(runtimeRoot string) *LocalRunner {
	return &LocalRunner{
		sandboxRoot: filepath.Join(runtimeRoot, "sandbox", "runs"),
	}
}

func (r *LocalRunner) Profiles() []Profile {
	return []Profile{
		{
			ID:                    "local-isolated",
			Name:                  "Local Isolated",
			Description:           "Run python inside a temporary workspace with isolated mode enabled.",
			Isolation:             "process + temporary workspace",
			NetworkPolicy:         "inherits host network",
			DefaultTimeoutSeconds: 15,
		},
	}
}

func (r *LocalRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	profile := strings.TrimSpace(req.Profile)
	if profile == "" {
		profile = "local-isolated"
	}
	if profile != "local-isolated" {
		return RunResult{}, fmt.Errorf("unsupported sandbox profile: %s", profile)
	}

	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 15
	}
	if timeoutSeconds > 300 {
		timeoutSeconds = 300
	}

	if err := os.MkdirAll(r.sandboxRoot, 0o755); err != nil {
		return RunResult{}, err
	}

	workdir, err := os.MkdirTemp(r.sandboxRoot, "run-*")
	if err != nil {
		return RunResult{}, err
	}

	if err := os.MkdirAll(filepath.Join(workdir, "home"), 0o755); err != nil {
		return RunResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(workdir, "tmp"), 0o755); err != nil {
		return RunResult{}, err
	}

	if err := writeInjectedFiles(workdir, req.Files); err != nil {
		return RunResult{}, err
	}

	if err := copyInjectedPaths(workdir, req.CopyPaths); err != nil {
		return RunResult{}, err
	}

	scriptName := firstNonEmpty(strings.TrimSpace(req.Script), "main.py")
	scriptName, err = sanitizeRelativePath(scriptName)
	if err != nil {
		return RunResult{}, err
	}

	mainPath := filepath.Join(workdir, scriptName)
	if strings.TrimSpace(req.Code) != "" {
		if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
			return RunResult{}, err
		}
		if err := os.WriteFile(mainPath, []byte(req.Code), 0o644); err != nil {
			return RunResult{}, err
		}
	}

	if _, err := os.Stat(mainPath); err != nil {
		return RunResult{}, errors.New("sandbox requires code or files['main.py']")
	}

	pythonBinary := firstNonEmpty(strings.TrimSpace(req.PythonBinary), "python3")
	command := append([]string{pythonBinary, "-I", scriptName}, req.Args...)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	cmd.Dir = workdir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"PYTHONNOUSERSITE=1",
		"PYTHONUNBUFFERED=1",
		"HOME=" + filepath.Join(workdir, "home"),
		"TMPDIR=" + filepath.Join(workdir, "tmp"),
	}
	for key, value := range req.Env {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsRune(key, '=') {
			continue
		}
		value = strings.ReplaceAll(value, "{{SANDBOX_ROOT}}", workdir)
		value = strings.ReplaceAll(value, "{{SANDBOX_HOME}}", filepath.Join(workdir, "home"))
		value = strings.ReplaceAll(value, "{{SANDBOX_TMP}}", filepath.Join(workdir, "tmp"))
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	runErr := cmd.Run()
	durationMS := time.Since(startedAt).Milliseconds()

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			exitCode = 124
			stderr.WriteString("\n[sandbox] execution timed out")
		} else {
			return RunResult{}, runErr
		}
	}

	return RunResult{
		ExitCode:   exitCode,
		Stdout:     stdout.String(),
		Stderr:     strings.TrimLeft(stderr.String(), "\n"),
		DurationMS: durationMS,
		Workdir:    workdir,
		Command:    command,
	}, nil
}

func writeInjectedFiles(workdir string, files map[string]string) error {
	for name, content := range files {
		cleanName, err := sanitizeRelativePath(name)
		if err != nil {
			return err
		}
		target := filepath.Join(workdir, cleanName)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyInjectedPaths(workdir string, items []CopyPath) error {
	for _, item := range items {
		source := strings.TrimSpace(item.Source)
		targetName, err := sanitizeRelativePath(item.Target)
		if err != nil {
			return err
		}
		if source == "" {
			return errors.New("sandbox copy path requires source")
		}
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		target := filepath.Join(workdir, targetName)
		if info.IsDir() {
			if err := copyDir(source, target); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyFile(source, target, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(source string, target string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, 0o755)
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return copyFile(path, destination, info.Mode())
	})
}

func copyFile(source string, target string, mode os.FileMode) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	if _, err := targetFile.ReadFrom(sourceFile); err != nil {
		return err
	}
	return nil
}

func sanitizeRelativePath(name string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(name))
	if clean == "." || clean == "" {
		return "", errors.New("invalid sandbox file path")
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path is not allowed: %s", name)
	}
	if strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("path escapes sandbox root: %s", name)
	}
	return clean, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
