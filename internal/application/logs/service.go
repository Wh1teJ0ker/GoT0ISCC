package logs

import (
	"context"
	"errors"
	"os"
	"strings"

	logdomain "got0iscc/desktop/internal/domain/logs"
	"got0iscc/desktop/internal/platform/runtime"
)

const maxLogBytes = 128 * 1024

type Repository interface {
	ListJobs(ctx context.Context, limit int) ([]logdomain.Job, error)
	ListServiceLogs(ctx context.Context) ([]logdomain.ServiceLog, error)
	JobsSummary(ctx context.Context) (logdomain.Summary, error)
	JobByID(ctx context.Context, id string) (logdomain.Job, error)
	ServiceLogByID(ctx context.Context, id string) (logdomain.ServiceLog, error)
}

type Service struct {
	repo   Repository
	layout runtime.Layout
}

type ContentRequest struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

func NewService(repo Repository, layout runtime.Layout) *Service {
	return &Service{repo: repo, layout: layout}
}

func (s *Service) List(ctx context.Context) (logdomain.Payload, error) {
	summary, err := s.repo.JobsSummary(ctx)
	if err != nil {
		return logdomain.Payload{}, err
	}
	jobs, err := s.repo.ListJobs(ctx, 200)
	if err != nil {
		return logdomain.Payload{}, err
	}
	serviceLogs, err := s.repo.ListServiceLogs(ctx)
	if err != nil {
		return logdomain.Payload{}, err
	}
	return logdomain.Payload{
		Summary:     summary,
		Jobs:        jobs,
		ServiceLogs: serviceLogs,
	}, nil
}

func (s *Service) Content(ctx context.Context, req ContentRequest) (logdomain.Content, error) {
	id := strings.TrimSpace(req.ID)
	kind := strings.TrimSpace(req.Kind)
	switch kind {
	case "job":
		job, err := s.repo.JobByID(ctx, id)
		if err != nil {
			return logdomain.Content{}, err
		}
		return readLogContent(id, job.Title, job.Source, job.LogPath, job.Tail, "")
	case "service":
		item, err := s.repo.ServiceLogByID(ctx, id)
		if err != nil {
			return logdomain.Content{}, err
		}
		return readLogContent(id, item.Name, item.Source, item.FilePath, "", item.Description)
	default:
		return logdomain.Content{}, errors.New("未知日志类型")
	}
}

func readLogContent(id string, title string, source string, path string, fallback string, description string) (logdomain.Content, error) {
	content := logdomain.Content{
		ID:          id,
		Title:       title,
		Source:      source,
		Path:        path,
		Description: description,
	}
	info, statErr := os.Stat(path)
	if statErr == nil {
		content.SizeBytes = info.Size()
		content.ModifiedAt = info.ModTime().Format("2006-01-02 15:04:05")
	}

	if strings.TrimSpace(path) == "" {
		content.Content = fallback
		content.SizeBytes = int64(len(fallback))
		return content, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if fallback != "" {
			content.Content = fallback
			content.SizeBytes = int64(len(fallback))
			return content, nil
		}
		return logdomain.Content{}, err
	}
	if len(data) > maxLogBytes {
		content.Truncated = true
		data = data[len(data)-maxLogBytes:]
	}
	content.Content = string(data)
	content.SizeBytes = int64(len(data))
	return content, nil
}
