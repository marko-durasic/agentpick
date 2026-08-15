#!/usr/bin/env bash
# install.sh — build agentpick into ~/.local/bin and install short aliases
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${AGENTPICK_BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$BIN_DIR"

if ! command -v go >/dev/null 2>&1; then
  echo "install.sh: go not found on PATH" >&2
  exit 1
fi

echo "Building agentpick..."
go build -o "$BIN_DIR/agentpick" "$ROOT/cmd/agentpick"

# Thin alias scripts → agentpick <provider> (muscle-memory from h*)
install_alias() {
  local name="$1"
  local target="$2"
  local path="$BIN_DIR/$name"
  if [[ -e "$path" && ! -L "$path" ]]; then
    # Preserve previous non-symlink scripts once
    if [[ ! -e "${path}.pre-agentpick" ]]; then
      cp -a "$path" "${path}.pre-agentpick"
      echo "  backed up $path → ${path}.pre-agentpick"
    fi
  fi
  cat >"$path" <<EOF
#!/usr/bin/env bash
exec "$BIN_DIR/agentpick" $target "\$@"
EOF
  chmod +x "$path"
  echo "  installed $name → agentpick $target"
}

install_alias hclaude  "claude"
install_alias hopus    "claude"
install_alias hcodex   "codex"
install_alias hcursor  "cursor"
install_alias hgrok    "grok"
install_alias hcopilot "copilot"
install_alias hagy     "agy"

cat >"$BIN_DIR/hagents" <<EOF
#!/usr/bin/env bash
exec "$BIN_DIR/agentpick" list "\$@"
EOF
chmod +x "$BIN_DIR/hagents"
echo "  installed hagents → agentpick list"

echo
echo "Done. Ensure $BIN_DIR is on PATH, then try:"
echo "  agentpick --help"
echo "  agentpick list"
echo "  agentpick          # interactive picker"
echo "  hclaude            # alias"
