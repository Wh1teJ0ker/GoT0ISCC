package pythonenv

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) initializeFromFallback(ctx context.Context, detectErr error) (CommandResult, error) {
	config, err := s.loadFallbackConfig()
	if err != nil {
		return CommandResult{}, err
	}
	if !config.Enabled {
		return CommandResult{}, fmt.Errorf("%w；且 Python fallback 未启用，请先编辑 %s", detectErr, s.fallbackConfigPath())
	}
	spec, ok := config.platformSpec()
	if !ok || !config.isConfiguredForCurrentPlatform() {
		return CommandResult{}, fmt.Errorf("%w；且当前平台 %s 未配置 Python fallback，请检查 %s", detectErr, config.platformKey(), s.fallbackConfigPath())
	}

	archiveDir := filepath.Join(s.layout.AppDataRoot, "downloads")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return CommandResult{}, err
	}

	archivePath, downloadResult, err := downloadArchive(ctx, archiveDir, spec)
	if err != nil {
		return downloadResult, err
	}

	if err := os.RemoveAll(s.fallbackRuntimeRoot()); err != nil {
		return downloadResult, err
	}
	if err := os.MkdirAll(s.fallbackRuntimeRoot(), 0o755); err != nil {
		return downloadResult, err
	}
	if err := extractArchive(archivePath, s.fallbackRuntimeRoot(), spec.ArchiveType); err != nil {
		return downloadResult, fmt.Errorf("解包 Python fallback 失败: %w", err)
	}

	binaryPath := filepath.Join(s.fallbackRuntimeRoot(), filepath.FromSlash(strings.TrimSpace(spec.BinaryPath)))
	if !pathExists(binaryPath) {
		return downloadResult, fmt.Errorf("fallback Python 已下载，但解释器不存在: %s", binaryPath)
	}
	if err := ensureExecutable(binaryPath); err != nil {
		return downloadResult, err
	}

	pipResult, pipErr := s.upgradePip(ctx, binaryPath)
	saveErr := s.saveManagedPython(ctx, s.fallbackRuntimeRoot(), binaryPath)
	combined := combineCommandResults(
		"下载并启用 fallback Python 运行时",
		[]string{"pythonenv", "fallback", "initialize"},
		downloadResult,
		pipResult,
	)
	if pipErr != nil {
		return combined, pipErr
	}
	if saveErr != nil {
		return combined, saveErr
	}
	return combined, nil
}

func downloadArchive(ctx context.Context, archiveDir string, spec fallbackPlatformSpec) (string, CommandResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	parsedURL, err := url.Parse(strings.TrimSpace(spec.URL))
	if err != nil {
		return "", CommandResult{}, fmt.Errorf("Python fallback URL 无效: %w", err)
	}

	filename := filepath.Base(parsedURL.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "python-runtime.archive"
	}
	archivePath := filepath.Join(archiveDir, filename)

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return "", CommandResult{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", CommandResult{}, fmt.Errorf("下载 Python fallback 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", CommandResult{}, fmt.Errorf("下载 Python fallback 失败: HTTP %d", resp.StatusCode)
	}

	targetFile, err := os.Create(archivePath)
	if err != nil {
		return "", CommandResult{}, err
	}
	defer targetFile.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(targetFile, hasher), resp.Body)
	if err != nil {
		return "", CommandResult{}, fmt.Errorf("写入 Python fallback 压缩包失败: %w", err)
	}

	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	expectedSHA := strings.TrimSpace(strings.ToLower(spec.SHA256))
	if expectedSHA != "" && actualSHA != expectedSHA {
		return "", CommandResult{
			OK:         false,
			Command:    []string{"download", spec.URL},
			Stdout:     fmt.Sprintf("archive=%s\nbytes=%d\nsha256=%s", archivePath, written, actualSHA),
			Stderr:     fmt.Sprintf("SHA256 mismatch: expected=%s actual=%s", expectedSHA, actualSHA),
			ExitCode:   1,
			DurationMS: 0,
		}, fmt.Errorf("Python fallback SHA256 校验失败")
	}

	return archivePath, CommandResult{
		OK:         true,
		Command:    []string{"download", spec.URL},
		Stdout:     fmt.Sprintf("archive=%s\nbytes=%d\nsha256=%s", archivePath, written, actualSHA),
		Stderr:     "",
		ExitCode:   0,
		DurationMS: 0,
	}, nil
}

func extractArchive(archivePath string, targetRoot string, archiveType string) error {
	switch strings.ToLower(strings.TrimSpace(archiveType)) {
	case "zip":
		return extractZIP(archivePath, targetRoot)
	case "tar.gz", "tgz":
		return extractTarGZ(archivePath, targetRoot)
	default:
		return fmt.Errorf("暂不支持的 archive_type: %s，仅支持 zip/tar.gz", archiveType)
	}
}

func extractZIP(archivePath string, targetRoot string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		targetPath, err := safeArchivePath(targetRoot, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := errors.Join(in.Close(), out.Close())
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractTarGZ(archivePath string, targetRoot string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		targetPath, err := safeArchivePath(targetRoot, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func safeArchivePath(root string, name string) (string, error) {
	cleanName := filepath.Clean(strings.ReplaceAll(name, "\\", "/"))
	targetPath := filepath.Join(root, cleanName)
	rel, err := filepath.Rel(root, targetPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive path escapes target root: %s", name)
	}
	return targetPath, nil
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := info.Mode()
	if mode&0o111 != 0 {
		return nil
	}
	return os.Chmod(path, mode|0o755)
}

func combineCommandResults(message string, command []string, results ...CommandResult) CommandResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	totalDuration := int64(0)
	ok := true
	exitCode := 0

	for index, result := range results {
		if result.Command == nil && result.Stdout == "" && result.Stderr == "" && result.DurationMS == 0 && result.ExitCode == 0 && !result.OK {
			continue
		}
		if stdout.Len() > 0 {
			stdout.WriteString("\n\n")
		}
		stdout.WriteString(fmt.Sprintf("[step %d] %s\n", index+1, strings.Join(result.Command, " ")))
		if strings.TrimSpace(result.Stdout) != "" {
			stdout.WriteString(strings.TrimSpace(result.Stdout))
			stdout.WriteString("\n")
		}
		if strings.TrimSpace(result.Stderr) != "" {
			if stderr.Len() > 0 {
				stderr.WriteString("\n\n")
			}
			stderr.WriteString(fmt.Sprintf("[step %d] %s\n%s", index+1, strings.Join(result.Command, " "), strings.TrimSpace(result.Stderr)))
		}
		totalDuration += result.DurationMS
		if !result.OK {
			ok = false
			if result.ExitCode != 0 {
				exitCode = result.ExitCode
			}
		}
	}

	if ok {
		exitCode = 0
	}
	if strings.TrimSpace(message) != "" {
		stdout.WriteString("\n")
		stdout.WriteString(message)
	}

	return CommandResult{
		OK:         ok,
		Command:    command,
		Stdout:     strings.TrimSpace(stdout.String()),
		Stderr:     strings.TrimSpace(stderr.String()),
		ExitCode:   exitCode,
		DurationMS: totalDuration,
	}
}
