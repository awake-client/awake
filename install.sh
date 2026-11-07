#!/bin/sh
# Awake installer for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/awake-client/awake/main/install.sh | sh
#
# Environment variables:
#   AWAKE_VERSION      Version to install, e.g. v1.2.3 (default: latest release).
#   AWAKE_INSTALL_DIR  Where to install the app (default: see --help).
#   GITHUB_TOKEN       Used when the latest release has to go through the API.
#
# Exit codes: 0 success, 1 usage or install error.

set -eu

REPOSITORY="awake-client/awake"
APP_NAME="Awake"
BINARY_NAME="awake"
VERSION="${AWAKE_VERSION:-}"
INSTALL_DIR="${AWAKE_INSTALL_DIR:-}"
SYSTEM=0
MODIFY_PATH=1
UNINSTALL=0

usage() {
  cat <<EOF
Install Awake, a git-native API client, on macOS or Linux.

Usage:
  curl -fsSL https://raw.githubusercontent.com/awake-client/awake/main/install.sh | sh
  sh install.sh [options]

Options:
  -v, --version <tag>  Install a specific version (default: latest release).
  -d, --dir <path>     Install directory.
                       macOS: the folder that receives Awake.app
                              (default: /Applications, else ~/Applications).
                       Linux: the folder that receives the awake binary
                              (default: /usr/local/bin when writable,
                              else ~/.local/bin).
      --system         Install system-wide. Uses sudo and /usr/local
                       (macOS: /Applications) instead of the user's home.
      --no-path        Do not add the install directory to PATH (Linux).
      --uninstall      Remove a previous install.
  -h, --help           Show this help.

Environment:
  AWAKE_VERSION, AWAKE_INSTALL_DIR, GITHUB_TOKEN

Examples:
  curl -fsSL https://raw.githubusercontent.com/awake-client/awake/main/install.sh | sh
  curl -fsSL .../install.sh | sh -s -- --version v1.2.3
  sh install.sh --uninstall
EOF
}

say() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    -v|--version)
      [ $# -ge 2 ] || die "$1 requires a value"
      VERSION="$2"
      shift 2
      ;;
    --version=*)
      VERSION="${1#*=}"
      shift
      ;;
    -d|--dir)
      [ $# -ge 2 ] || die "$1 requires a value"
      INSTALL_DIR="$2"
      shift 2
      ;;
    --dir=*)
      INSTALL_DIR="${1#*=}"
      shift
      ;;
    --system)
      SYSTEM=1
      shift
      ;;
    --no-path)
      MODIFY_PATH=0
      shift
      ;;
    --uninstall)
      UNINSTALL=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1 (try --help)"
      ;;
  esac
done

# ---------------------------------------------------------------- utilities

download() { # url destination
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$2" "$1"
  else
    die "curl or wget is required to download Awake"
  fi
}

github_api_get() { # url destination
  if command -v curl >/dev/null 2>&1; then
    if [ -n "${GITHUB_TOKEN:-}" ]; then
      curl -fsSL --retry 3 -H "Authorization: Bearer $GITHUB_TOKEN" -o "$2" "$1"
    else
      curl -fsSL --retry 3 -o "$2" "$1"
    fi
  else
    download "$1" "$2"
  fi
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    die "root privileges are required for this install; rerun without --system or as root"
  fi
}

root_do() {
  if [ "$SYSTEM" -eq 1 ]; then
    as_root "$@"
  else
    "$@"
  fi
}

sha256() { # file
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    return 1
  fi
}

# ------------------------------------------------------------------- system

detect_os() {
  case "$(uname -s)" in
    Darwin) OS=darwin ;;
    Linux) OS=linux ;;
    *) die "unsupported operating system: $(uname -s); Awake supports macOS and Linux" ;;
  esac
}

detect_arch() {
  if [ "$OS" = darwin ]; then
    # Report arm64 even when this script runs under Rosetta 2.
    if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = 1 ]; then
      ARCH=arm64
      return
    fi
  fi

  case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *) die "unsupported architecture: $(uname -m)" ;;
  esac
}

resolve_version() {
  if [ -z "$VERSION" ] && command -v curl >/dev/null 2>&1; then
    # releases/latest redirects to releases/tag/vX.Y.Z; no API rate limit.
    latest="$(curl -fsSL -o /dev/null -w '%{url_effective}' \
      "https://github.com/$REPOSITORY/releases/latest" 2>/dev/null || true)"
    case "$latest" in
      */tag/*) VERSION="${latest##*/tag/}" ;;
    esac
  fi

  if [ -z "$VERSION" ]; then
    # The redirect can be blocked; the API is the fallback.
    json="$TMP_DIR/latest.json"
    if github_api_get "https://api.github.com/repos/$REPOSITORY/releases/latest" "$json" 2>/dev/null; then
      VERSION="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$json" | head -n 1)"
    fi
  fi

  [ -n "$VERSION" ] || die "could not determine the latest release; pass --version vX.Y.Z"

  case "$VERSION" in
    v*) ;;
    *) VERSION="v$VERSION" ;;
  esac
}

asset_name() {
  case "$OS" in
    darwin) printf 'awake-macos-%s.zip' "$ARCH" ;;
    linux) printf 'awake-linux-%s.tar.gz' "$ARCH" ;;
  esac
}

verify_checksum() { # archive asset
  sums="$TMP_DIR/SHA256SUMS"
  if ! download "$RELEASE_URL/SHA256SUMS" "$sums" 2>/dev/null; then
    warn "checksum file unavailable; skipping verification"
    return 0
  fi

  expected="$(awk -v name="$2" '{n=$2; sub(/^\*/,"",n); if (n==name) {print $1; exit}}' "$sums")"
  if [ -z "$expected" ]; then
    warn "no checksum published for $2; skipping verification"
    return 0
  fi

  if ! actual="$(sha256 "$1")"; then
    warn "no sha256 utility found; skipping verification"
    return 0
  fi

  [ "$expected" = "$actual" ] || die "checksum mismatch for $2 (expected $expected, got $actual)"
  say "Verified $2"
}

# ------------------------------------------------------------ install paths

resolve_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    printf '%s\n' "${INSTALL_DIR%/}"
    return
  fi

  case "$OS" in
    darwin)
      if [ "$SYSTEM" -eq 1 ] || [ -w /Applications ]; then
        printf '/Applications\n'
      else
        printf '%s/Applications\n' "$HOME"
      fi
      ;;
    linux)
      if [ "$SYSTEM" -eq 1 ]; then
        printf '/usr/local/bin\n'
      elif [ -w /usr/local/bin ] && [ -w /usr/local ]; then
        printf '/usr/local/bin\n'
      else
        printf '%s/.local/bin\n' "$HOME"
      fi
      ;;
  esac
}

data_home() {
  if [ "$SYSTEM" -eq 1 ]; then
    printf '/usr/local/share\n'
  else
    printf '%s\n' "${XDG_DATA_HOME:-$HOME/.local/share}"
  fi
}

ensure_user_dir() { # path
  if ! root_do mkdir -p "$1" 2>/dev/null; then
    if [ "$SYSTEM" -eq 1 ]; then
      die "cannot create $1 as root"
    fi
    die "cannot create $1; rerun with --system or choose another --dir"
  fi
  if [ "$SYSTEM" -eq 0 ] && [ ! -w "$1" ]; then
    die "$1 is not writable; rerun with --system or choose another --dir"
  fi
}

remove_file() { # path
  rm -f "$1" 2>/dev/null || as_root rm -f "$1"
}

remove_tree() { # path
  rm -rf "$1" 2>/dev/null || as_root rm -rf "$1"
}

# --------------------------------------------------------------- linux glue

linux_runtime_hint() {
  if ldconfig -p 2>/dev/null | grep -q 'libwebkitgtk-6\.0'; then
    return
  fi
  for lib in /usr/lib/*/libwebkitgtk-6.0.so* /usr/lib/libwebkitgtk-6.0.so*; do
    if [ -e "$lib" ]; then
      return
    fi
  done

  warn "WebKitGTK 6.0 was not found; Awake needs GTK4 and WebKitGTK 6.0 at runtime"
  if command -v apt-get >/dev/null 2>&1; then
    say "  sudo apt install libgtk-4-1 libwebkitgtk-6.0-4" >&2
  elif command -v dnf >/dev/null 2>&1; then
    say "  sudo dnf install gtk4 webkitgtk6.0" >&2
  elif command -v pacman >/dev/null 2>&1; then
    say "  sudo pacman -S gtk4 webkitgtk-6.0" >&2
  elif command -v zypper >/dev/null 2>&1; then
    say "  sudo zypper install gtk4 libwebkitgtk-6.0-4" >&2
  fi
}

install_desktop_entry() { # binary_dir icon_source
  binary="$1/$BINARY_NAME"
  data="$(data_home)"
  desktop_dir="$data/applications"
  icon_dir="$data/icons/hicolor/1024x1024/apps"

  ensure_user_dir "$desktop_dir"
  ensure_user_dir "$icon_dir"

  icon="$2"
  if [ -n "$icon" ] && [ -f "$icon" ]; then
    root_do cp "$icon" "$icon_dir/$BINARY_NAME.png"
  fi

  entry="$TMP_DIR/$BINARY_NAME.desktop"
  cat > "$entry" <<EOF
[Desktop Entry]
Version=1.0
Name=$APP_NAME
Comment=Git-native API client
Exec="$binary" %u
Terminal=false
Type=Application
Icon=$BINARY_NAME
Categories=Utility;
StartupWMClass=$BINARY_NAME
EOF
  root_do cp "$entry" "$desktop_dir/$BINARY_NAME.desktop"

  if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    root_do gtk-update-icon-cache -q -t -f "$data/icons/hicolor" 2>/dev/null || true
  fi
  if command -v update-desktop-database >/dev/null 2>&1; then
    root_do update-desktop-database -q "$desktop_dir" 2>/dev/null || true
  fi

  say "Installed the application menu entry."
}

add_to_path() { # directory
  dir="$1"
  case ":$PATH:" in
    *":$dir:"*) return ;;
  esac

  if [ "$MODIFY_PATH" -eq 0 ]; then
    say "Add $dir to your PATH to run awake from a terminal."
    return
  fi

  shell_name="$(basename "${SHELL:-sh}")"
  case "$shell_name" in
    zsh)
      rc="$HOME/.zshrc"
      line="export PATH=\"$dir:\$PATH\""
      ;;
    bash)
      if [ "$OS" = darwin ] && [ ! -f "$HOME/.bashrc" ] && [ -f "$HOME/.bash_profile" ]; then
        rc="$HOME/.bash_profile"
      else
        rc="$HOME/.bashrc"
      fi
      line="export PATH=\"$dir:\$PATH\""
      ;;
    fish)
      rc="$HOME/.config/fish/config.fish"
      line="fish_add_path \"$dir\""
      mkdir -p "$(dirname "$rc")"
      ;;
    *)
      say "Add $dir to your PATH to run awake from a terminal."
      return
      ;;
  esac

  if [ -f "$rc" ] && grep -q '>>> awake installer >>>' "$rc"; then
    return
  fi

  {
    printf '\n# >>> awake installer >>>\n'
    printf '%s\n' "$line"
    printf '# <<< awake installer <<<\n'
  } >> "$rc"
  say "Added $dir to PATH in $rc (restart your shell to pick it up)."
}

remove_from_path() {
  for rc in \
    "$HOME/.zshrc" \
    "$HOME/.bashrc" \
    "$HOME/.bash_profile" \
    "$HOME/.config/fish/config.fish"
  do
    [ -f "$rc" ] || continue
    grep -q '>>> awake installer >>>' "$rc" || continue

    tmp="$rc.awake-tmp"
    awk '
      /^# >>> awake installer >>>$/ { skip = 1; next }
      /^# <<< awake installer <<<$/ { skip = 0; next }
      !skip { print }
    ' "$rc" > "$tmp"
    mv "$tmp" "$rc"
  done
}

# -------------------------------------------------------------------- modes

install_macos() { # archive
  unzip -q "$1" -d "$TMP_DIR/extract" || die "could not unpack $1"

  app="$TMP_DIR/extract/$APP_NAME.app"
  [ -d "$app" ] || die "archive did not contain $APP_NAME.app"

  ensure_user_dir "$INSTALL_DIR"
  destination="$INSTALL_DIR/$APP_NAME.app"

  if [ -e "$destination" ]; then
    root_do rm -rf "$destination"
  fi
  root_do ditto "$app" "$destination"

  # Release builds are ad-hoc signed, so the quarantine flag makes Gatekeeper
  # report the app as damaged. Remove it from our own freshly unpacked copy.
  if command -v xattr >/dev/null 2>&1; then
    root_do xattr -dr com.apple.quarantine "$destination" 2>/dev/null || true
  fi

  say "Installed $APP_NAME $VERSION to $destination"
  say "Open it from Launchpad or with: open -a $APP_NAME"
}

install_linux() { # archive
  tar -xzf "$1" -C "$TMP_DIR/extract" || die "could not unpack $1"

  binary="$TMP_DIR/extract/$BINARY_NAME"
  [ -f "$binary" ] || die "archive did not contain the $BINARY_NAME binary"

  ensure_user_dir "$INSTALL_DIR"
  destination="$INSTALL_DIR/$BINARY_NAME"
  root_do cp "$binary" "$destination"
  root_do chmod 755 "$destination"

  icon="$TMP_DIR/extract/$BINARY_NAME.png"
  if [ ! -f "$icon" ]; then
    # Older releases only contain the binary.
    if download "https://raw.githubusercontent.com/$REPOSITORY/$VERSION/build/appicon.png" \
      "$TMP_DIR/appicon.png" 2>/dev/null; then
      icon="$TMP_DIR/appicon.png"
    fi
  fi

  install_desktop_entry "$INSTALL_DIR" "$icon"
  linux_runtime_hint
  add_to_path "$INSTALL_DIR"

  say "Installed $APP_NAME $VERSION to $destination"
  say "Run it with: $destination"
}

uninstall() {
  case "$OS" in
    darwin)
      target="$INSTALL_DIR/$APP_NAME.app"
      if [ -e "$target" ]; then
        remove_tree "$target"
        say "Removed $target"
      else
        say "$APP_NAME is not installed in $INSTALL_DIR"
      fi
      ;;
    linux)
      removed=0
      for dir in "$INSTALL_DIR" "/usr/local/bin" "$HOME/.local/bin"; do
        [ -n "$dir" ] || continue
        target="$dir/$BINARY_NAME"
        [ -e "$target" ] || continue
        remove_file "$target"
        say "Removed $target"
        removed=1
      done

      data="$(data_home)"
      for file in \
        "$data/applications/$BINARY_NAME.desktop" \
        "$data/icons/hicolor/1024x1024/apps/$BINARY_NAME.png"
      do
        [ -e "$file" ] || continue
        remove_file "$file"
      done
      if command -v update-desktop-database >/dev/null 2>&1; then
        root_do update-desktop-database -q "$data/applications" 2>/dev/null || true
      fi
      remove_from_path

      [ "$removed" -eq 1 ] || say "$APP_NAME is not installed"
      ;;
  esac
}

# --------------------------------------------------------------------- main

detect_os
detect_arch
INSTALL_DIR="$(resolve_install_dir)"

if [ "$UNINSTALL" -eq 1 ]; then
  uninstall
  exit 0
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/awake.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

resolve_version

ASSET="$(asset_name)"
RELEASE_URL="https://github.com/$REPOSITORY/releases/download/$VERSION"

say "Installing $APP_NAME $VERSION ($OS/$ARCH)"

download "$RELEASE_URL/$ASSET" "$TMP_DIR/$ASSET"
verify_checksum "$TMP_DIR/$ASSET" "$ASSET"

mkdir -p "$TMP_DIR/extract"

case "$OS" in
  darwin) install_macos "$TMP_DIR/$ASSET" ;;
  linux) install_linux "$TMP_DIR/$ASSET" ;;
esac
