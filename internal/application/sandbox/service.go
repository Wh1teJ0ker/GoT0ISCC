package sandbox

import (
	"context"
	"strings"

	pythonrunner "got0iscc/desktop/internal/platform/sandbox/python"
)

type Service struct {
	runner       pythonrunner.Runner
	pythonBinary func(context.Context) (string, error)
}

type RunRequest struct {
	Code           string            `json:"code"`
	Files          map[string]string `json:"files"`
	Args           []string          `json:"args"`
	PythonBinary   string            `json:"python_binary"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Profile        string            `json:"profile"`
}

type RunResponse struct {
	OK         bool     `json:"ok"`
	ExitCode   int      `json:"exit_code"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	DurationMS int64    `json:"duration_ms"`
	Workdir    string   `json:"workdir"`
	Command    []string `json:"command"`
}

func NewService(runner pythonrunner.Runner, pythonBinary func(context.Context) (string, error)) *Service {
	return &Service{
		runner:       runner,
		pythonBinary: pythonBinary,
	}
}

func (s *Service) Profiles() []pythonrunner.Profile {
	return s.runner.Profiles()
}

func (s *Service) Run(ctx context.Context, req RunRequest) (RunResponse, error) {
	pythonBinary := strings.TrimSpace(req.PythonBinary)
	if pythonBinary == "" && s.pythonBinary != nil {
		detected, err := s.pythonBinary(ctx)
		if err == nil {
			pythonBinary = strings.TrimSpace(detected)
		}
	}
	runReq := pythonrunner.RunRequest{
		Code:           strings.TrimSpace(req.Code),
		Files:          req.Files,
		Args:           req.Args,
		PythonBinary:   pythonBinary,
		TimeoutSeconds: req.TimeoutSeconds,
		Profile:        strings.TrimSpace(req.Profile),
	}

	result, err := s.runner.Run(ctx, runReq)
	if err != nil {
		return RunResponse{}, err
	}

	return RunResponse{
		OK:         result.ExitCode == 0,
		ExitCode:   result.ExitCode,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		DurationMS: result.DurationMS,
		Workdir:    result.Workdir,
		Command:    result.Command,
	}, nil
}
