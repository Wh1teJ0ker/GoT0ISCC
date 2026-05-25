package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Layout struct {
	AppRoot          string
	WorkspaceRoot    string
	ChallengesRoot   string
	TheoryBankPath   string
	TheoryBankDBPath string
	InitSeedSQLPath  string
	InitConfigPath   string
	AppDataRoot      string
	AppRuntimeRoot   string
	AppDatabasePath  string
}

func DetectLayout() (Layout, error) {
	appRoot := detectRuntimeAppRoot()
	appDataRoot := filepath.Join(appRoot, "data")

	workspaceRoot := filepath.Dir(appRoot)
	if workspaceRoot == "" {
		workspaceRoot = appRoot
	}

	legacyRuntimeRoot := filepath.Join(appRoot, "runtime")
	theoryBankPath := resolveTheoryBankPath(appDataRoot, appRoot)
	mainDBPath := filepath.Join(appDataRoot, "got0iscc.db")
	layout := Layout{
		AppRoot:          appRoot,
		WorkspaceRoot:    workspaceRoot,
		ChallengesRoot:   filepath.Join(appDataRoot, "challenges"),
		TheoryBankPath:   theoryBankPath,
		TheoryBankDBPath: mainDBPath,
		InitSeedSQLPath:  filepath.Join(appDataRoot, "got0iscc.init.sql"),
		InitConfigPath:   filepath.Join(appDataRoot, "got0iscc.init.example.yaml"),
		AppDataRoot:      appDataRoot,
		AppRuntimeRoot:   filepath.Join(appDataRoot, "runtime"),
		AppDatabasePath:  mainDBPath,
	}

	if err := os.MkdirAll(layout.AppDataRoot, 0o755); err != nil {
		return Layout{}, fmt.Errorf("create app data root: %w", err)
	}
	if err := os.MkdirAll(layout.AppRuntimeRoot, 0o755); err != nil {
		return Layout{}, fmt.Errorf("create app runtime root: %w", err)
	}
	if err := os.MkdirAll(layout.ChallengesRoot, 0o755); err != nil {
		return Layout{}, fmt.Errorf("create challenges root: %w", err)
	}
	if err := migrateLegacyLocalData(layout, legacyRuntimeRoot); err != nil {
		return Layout{}, err
	}

	return layout, nil
}

func detectCurrentAppRoot() string {
	if workingDir, err := os.Getwd(); err == nil && workingDir != "" {
		return filepath.Clean(workingDir)
	}
	if absoluteDot, err := filepath.Abs("."); err == nil && absoluteDot != "" {
		return filepath.Clean(absoluteDot)
	}
	return "."
}

func detectRuntimeAppRoot() string {
	workingRoot := detectCurrentAppRoot()
	if looksLikeAppRoot(workingRoot) {
		return workingRoot
	}
	if execRoot := detectAppRoot(executableStartDirs()...); execRoot != "" && execRoot != "." {
		return execRoot
	}
	return workingRoot
}

func migrateLegacyLocalData(layout Layout, legacyRuntimeRoot string) error {
	if err := migrateSQLiteIfMissing(
		filepath.Join(legacyRuntimeRoot, "got0iscc.db"),
		layout.AppDatabasePath,
	); err != nil {
		return fmt.Errorf("migrate local database: %w", err)
	}
	if err := moveSQLiteIfMissing(
		filepath.Join(layout.AppRoot, "theory-bank.sqlite"),
		layout.TheoryBankDBPath,
	); err != nil {
		return fmt.Errorf("migrate theory bank database: %w", err)
	}
	if err := migrateDirIfMissing(filepath.Join(layout.WorkspaceRoot, "challenges"), layout.ChallengesRoot); err != nil {
		return fmt.Errorf("migrate challenges dir: %w", err)
	}
	legacyFilePairs := [][2]string{
		{filepath.Join(layout.AppRoot, "got0iscc.init.sql"), layout.InitSeedSQLPath},
		{filepath.Join(layout.AppRoot, "got0iscc.init.example.yaml"), layout.InitConfigPath},
	}
	for _, pair := range legacyFilePairs {
		if err := moveFileIfMissing(pair[0], pair[1]); err != nil {
			return fmt.Errorf("migrate %s: %w", filepath.Base(pair[0]), err)
		}
	}
	return nil
}

func migrateSQLiteIfMissing(sourcePath string, targetPath string) error {
	if pathExists(targetPath) || !pathExists(sourcePath) {
		return nil
	}
	basePairs := [][2]string{
		{sourcePath, targetPath},
		{sourcePath + "-wal", targetPath + "-wal"},
		{sourcePath + "-shm", targetPath + "-shm"},
	}
	for _, pair := range basePairs {
		if err := copyFileIfMissing(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func moveSQLiteIfMissing(sourcePath string, targetPath string) error {
	if pathExists(targetPath) || !pathExists(sourcePath) {
		return nil
	}
	basePairs := [][2]string{
		{sourcePath, targetPath},
		{sourcePath + "-wal", targetPath + "-wal"},
		{sourcePath + "-shm", targetPath + "-shm"},
	}
	for _, pair := range basePairs {
		if err := moveFileIfMissing(pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func copyFileIfMissing(sourcePath string, targetPath string) error {
	if !pathExists(sourcePath) || pathExists(targetPath) {
		return nil
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return err
	}
	return nil
}

func moveFileIfMissing(sourcePath string, targetPath string) error {
	if !pathExists(sourcePath) || pathExists(targetPath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(sourcePath, targetPath); err == nil {
		return nil
	}
	if err := copyFileIfMissing(sourcePath, targetPath); err != nil {
		return err
	}
	return os.Remove(sourcePath)
}

func migrateDirIfMissing(sourcePath string, targetPath string) error {
	if !pathExists(sourcePath) || pathExists(targetPath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.Rename(sourcePath, targetPath)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func executableStartDirs() []string {
	candidates := make([]string, 0, 2)
	add := func(path string) {
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		for _, existing := range candidates {
			if existing == cleaned {
				return
			}
		}
		candidates = append(candidates, cleaned)
	}
	if executablePath, err := os.Executable(); err == nil {
		add(filepath.Dir(executablePath))
		if resolvedPath, err := filepath.EvalSymlinks(executablePath); err == nil {
			add(filepath.Dir(resolvedPath))
		}
	}
	return candidates
}

func detectAppRoot(startDirs ...string) string {
	for _, startDir := range startDirs {
		current := filepath.Clean(startDir)
		for {
			if looksLikeAppRoot(current) {
				return current
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	if len(startDirs) > 0 && startDirs[0] != "" {
		return filepath.Clean(startDirs[0])
	}

	return "."
}

func looksLikeAppRoot(path string) bool {
	if pathExists(filepath.Join(path, "wails.json")) && pathExists(filepath.Join(path, "go.mod")) {
		return true
	}

	dataDir := filepath.Join(path, "data")
	markers := []string{
		filepath.Join(dataDir, "got0iscc.db"),
		filepath.Join(dataDir, "got0iscc.init.sql"),
		filepath.Join(dataDir, "got0iscc.init.example.yaml"),
	}
	for _, marker := range markers {
		if pathExists(marker) {
			return true
		}
	}

	return false
}

func resolveTheoryBankPath(dataRoot string, appRoot string) string {
	baseDirs := []string{dataRoot, appRoot}
	for _, baseDir := range baseDirs {
		standardizedPath := filepath.Join(baseDir, "清洗后的题库.standardized.json")
		standardizedMetaPath := filepath.Join(baseDir, "清洗后的题库.standardized.meta.json")
		rawPath := filepath.Join(baseDir, "清洗后的题库.json")
		if standardizedBankReady(standardizedPath, standardizedMetaPath, rawPath) {
			return standardizedPath
		}
	}
	for _, baseDir := range baseDirs {
		candidates := []string{
			filepath.Join(baseDir, "清洗后的题库.json"),
			filepath.Join(baseDir, "清洗后的题库.normalized.json"),
		}
		for _, candidate := range candidates {
			if pathExists(candidate) {
				return candidate
			}
		}
	}
	return filepath.Join(dataRoot, "清洗后的题库.normalized.json")
}

func standardizedBankReady(bankPath string, metaPath string, rawPath string) bool {
	if !pathExists(bankPath) || !pathExists(metaPath) {
		return false
	}

	var meta struct {
		TotalInput  int `json:"total_input"`
		TotalOutput int `json:"total_output"`
	}
	data, err := os.ReadFile(metaPath)
	if err != nil || json.Unmarshal(data, &meta) != nil {
		return false
	}
	if meta.TotalOutput <= 0 || meta.TotalOutput < meta.TotalInput {
		return false
	}

	minExpected := rawTheoryBankCount(rawPath)
	if minExpected > 0 && meta.TotalOutput < minExpected {
		return false
	}
	return true
}

func rawTheoryBankCount(path string) int {
	if !pathExists(path) {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return 0
	}
	return len(items)
}
