#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
APP_NAME="GoT0ISCC"
RELEASE_ROOT="$ROOT_DIR/build/releases/custom"
MAC_RELEASE_DIR="$RELEASE_ROOT/${APP_NAME}-macos-arm64"
WIN_RELEASE_DIR="$RELEASE_ROOT/${APP_NAME}-windows-amd64"
MAC_SOURCE_APP="$ROOT_DIR/build/bin/${APP_NAME}.app"
WIN_SOURCE_EXE="$ROOT_DIR/build/bin/${APP_NAME}.exe"
WIN_SOURCE_ALT="$ROOT_DIR/build/bin/${APP_NAME}"
DATA_DIR="$ROOT_DIR/data"

log() {
  printf '[package] %s\n' "$1"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'missing command: %s\n' "$1" >&2
    exit 1
  }
}

clean_db() {
  if [[ -f "$DATA_DIR/got0iscc.db" ]]; then
    log "checkpoint sqlite"
    sqlite3 "$DATA_DIR/got0iscc.db" "PRAGMA wal_checkpoint(TRUNCATE); VACUUM;"
    rm -f "$DATA_DIR/got0iscc.db-wal" "$DATA_DIR/got0iscc.db-shm"
  fi
}

copy_data_dir() {
  local target_dir="$1"
  local platform="$2"
  mkdir -p "$target_dir"
  rsync -a \
    --delete \
    --exclude '.DS_Store' \
    --exclude 'got0iscc.db-wal' \
    --exclude 'got0iscc.db-shm' \
    --exclude 'runtime/sandbox/runs' \
    "$DATA_DIR/" "$target_dir/data/"

  if [[ "$platform" == "windows" ]]; then
    rm -rf "$target_dir/data/python"
  fi
}

build_mac() {
  require_cmd wails
  require_cmd ditto
  require_cmd zip

  log "build macos app"
  (cd "$ROOT_DIR" && wails build -clean -platform darwin/arm64 -o "$APP_NAME")

  if [[ ! -d "$MAC_SOURCE_APP" ]]; then
    printf 'mac source app not found: %s\n' "$MAC_SOURCE_APP" >&2
    exit 1
  fi

  rm -rf "$MAC_RELEASE_DIR"
  mkdir -p "$MAC_RELEASE_DIR"
  ditto "$MAC_SOURCE_APP" "$MAC_RELEASE_DIR/$APP_NAME.app"
  copy_data_dir "$MAC_RELEASE_DIR" macos

  rm -f "$RELEASE_ROOT/${APP_NAME}-macos-arm64.zip"
  (cd "$RELEASE_ROOT" && ditto -c -k --sequesterRsrc --keepParent "${APP_NAME}-macos-arm64" "${APP_NAME}-macos-arm64.zip")
}

build_windows() {
  require_cmd wails
  require_cmd zip
  require_cmd x86_64-w64-mingw32-gcc

  log "build windows exe"
  (
    cd "$ROOT_DIR" && \
    CC=/opt/homebrew/bin/x86_64-w64-mingw32-gcc \
    CXX=/opt/homebrew/bin/x86_64-w64-mingw32-g++ \
    GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
    wails build -clean -platform windows/amd64 -webview2 download -o "$APP_NAME"
  )

  local win_binary="$WIN_SOURCE_EXE"
  if [[ ! -f "$win_binary" ]]; then
    if [[ -f "$WIN_SOURCE_ALT" ]]; then
      win_binary="$WIN_SOURCE_ALT"
    else
      printf 'windows source exe not found: %s or %s\n' "$WIN_SOURCE_EXE" "$WIN_SOURCE_ALT" >&2
      exit 1
    fi
  fi

  rm -rf "$WIN_RELEASE_DIR"
  mkdir -p "$WIN_RELEASE_DIR"
  cp "$win_binary" "$WIN_RELEASE_DIR/$APP_NAME.exe"
  copy_data_dir "$WIN_RELEASE_DIR" windows

  cat > "$WIN_RELEASE_DIR/README.txt" <<'EOF'
GoT0ISCC Windows 运行说明

1. 首次运行前，请确保系统已安装 Microsoft Edge WebView2 Runtime。
2. 如果双击无反应，优先安装以下运行时后再试：
   https://developer.microsoft.com/microsoft-edge/webview2/
3. 保持 GoT0ISCC.exe 与 data 目录同级，不要单独移动 exe。
EOF

  rm -f "$RELEASE_ROOT/${APP_NAME}-windows-amd64.zip"
  (cd "$RELEASE_ROOT" && zip -qry "${APP_NAME}-windows-amd64.zip" "${APP_NAME}-windows-amd64")
}

main() {
  require_cmd rsync
  require_cmd sqlite3
  mkdir -p "$RELEASE_ROOT"
  clean_db
  build_mac
  build_windows
  log "done"
}

main "$@"
