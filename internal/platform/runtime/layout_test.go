package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectCurrentAppRootUsesWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})

	got, err := filepath.EvalSymlinks(detectCurrentAppRoot())
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("detectCurrentAppRoot() = %q, want %q", got, root)
	}
}

func TestDetectRuntimeAppRootUsesWorkingDirectoryWhenItHasData(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "got0iscc.init.sql"), []byte("-- seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})

	got, err := filepath.EvalSymlinks(detectRuntimeAppRoot())
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("detectRuntimeAppRoot() = %q, want %q", got, root)
	}
}

func TestDetectAppRootFromPackagedExecutablePath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module got0iscc/desktop\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	execDir := filepath.Join(root, "build", "releases", "macos", "GoT0ISCC.app", "Contents", "MacOS")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := detectAppRoot(execDir); got != root {
		t.Fatalf("detectAppRoot() = %q, want %q", got, root)
	}
}

func TestDetectAppRootFromDataMarkers(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "got0iscc.init.sql"), []byte("-- seed"), 0o644); err != nil {
		t.Fatal(err)
	}

	execDir := filepath.Join(root, "GoT0ISCC.app", "Contents", "MacOS")
	if err := os.MkdirAll(execDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := detectAppRoot(execDir); got != root {
		t.Fatalf("detectAppRoot() = %q, want %q", got, root)
	}
}
