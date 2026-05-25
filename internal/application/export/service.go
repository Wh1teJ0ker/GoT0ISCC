package export

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	runtimeplatform "got0iscc/desktop/internal/platform/runtime"
)

type Service struct {
	layout runtimeplatform.Layout
}

type ExportResult struct {
	ArchivePath string   `json:"archive_path"`
	SizeBytes   int64    `json:"size_bytes"`
	Files       []string `json:"files"`
	SHA256      string   `json:"sha256"`
	CreatedAt   string   `json:"created_at"`
}

func NewService(layout runtimeplatform.Layout) *Service {
	return &Service{layout: layout}
}

func (s *Service) CreateMigrationBundle(ctx context.Context) (ExportResult, error) {
	_ = ctx
	exportRoot := filepath.Join(s.layout.AppDataRoot, "exports")
	if err := os.MkdirAll(exportRoot, 0o755); err != nil {
		return ExportResult{}, err
	}
	stamp := time.Now().Format("20060102-150405")
	targetPath := filepath.Join(exportRoot, fmt.Sprintf("got0iscc-migration-%s.zip", stamp))

	files := s.collectFiles()
	if len(files) == 0 {
		return ExportResult{}, fmt.Errorf("no files available for export")
	}
	if err := createZip(targetPath, files); err != nil {
		return ExportResult{}, err
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return ExportResult{}, err
	}
	hash, err := fileSHA256(targetPath)
	if err != nil {
		return ExportResult{}, err
	}
	manifest := make([]string, 0, len(files))
	for _, item := range files {
		manifest = append(manifest, item.archivePath)
	}
	return ExportResult{
		ArchivePath: targetPath,
		SizeBytes:   info.Size(),
		Files:       manifest,
		SHA256:      hash,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}, nil
}

type exportFile struct {
	sourcePath  string
	archivePath string
}

func (s *Service) collectFiles() []exportFile {
	paths := []string{
		s.layout.AppDatabasePath,
		s.layout.AppDatabasePath + "-wal",
		s.layout.AppDatabasePath + "-shm",
		s.layout.InitSeedSQLPath,
		s.layout.InitConfigPath,
	}
	files := make([]exportFile, 0, len(paths)+8)
	for _, path := range paths {
		if fileExists(path) {
			files = append(files, exportFile{
				sourcePath:  path,
				archivePath: filepath.ToSlash(strings.TrimPrefix(path, s.layout.AppRoot+string(filepath.Separator))),
			})
		}
	}

	addTree := func(root string, prefix string) {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) == 0 {
			return
		}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return nil
			}
			files = append(files, exportFile{
				sourcePath:  path,
				archivePath: filepath.ToSlash(filepath.Join(prefix, rel)),
			})
			return nil
		})
	}

	addTree(s.layout.AppDataRoot, "data")
	addTree(filepath.Join(s.layout.AppRoot, "runtime"), "runtime-legacy")

	manifestPayload := map[string]any{
		"created_at":     time.Now().Format(time.RFC3339),
		"app_root":       s.layout.AppRoot,
		"workspace_root": s.layout.WorkspaceRoot,
		"database":       s.layout.AppDatabasePath,
		"file_count":     len(files),
	}
	manifestBytes, _ := json.MarshalIndent(manifestPayload, "", "  ")
	manifestPath := filepath.Join(s.layout.AppDataRoot, "exports", ".manifest.tmp.json")
	_ = os.MkdirAll(filepath.Dir(manifestPath), 0o755)
	_ = os.WriteFile(manifestPath, manifestBytes, 0o644)
	files = append(files, exportFile{
		sourcePath:  manifestPath,
		archivePath: "manifest.json",
	})
	return files
}

func createZip(targetPath string, files []exportFile) error {
	targetFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	writer := zip.NewWriter(targetFile)
	defer writer.Close()

	for _, item := range files {
		info, err := os.Stat(item.sourcePath)
		if err != nil || info.IsDir() {
			continue
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = item.archivePath
		header.Method = zip.Deflate
		entryWriter, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		sourceFile, err := os.Open(item.sourcePath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(entryWriter, sourceFile); err != nil {
			sourceFile.Close()
			return err
		}
		sourceFile.Close()
	}
	return writer.Close()
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
