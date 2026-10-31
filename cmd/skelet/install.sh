#!/bin/sh
# Installs the skelet CLI from a GitHub release:
#   curl -fsSL https://github.com/skeletg/skelet/releases/latest/download/install.sh | sh
# SKELET_VERSION picks a release (default: the latest), SKELET_INSTALL_DIR the directory (default: ~/.local/bin).
# --force replaces a skelet that is a symlink - usually a checkout's ./skelet linked onto the PATH by hand
set -eu

die() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

force=0
for arg in "$@"; do
    case "$arg" in
        --force) force=1 ;;
        *) die "unknown option $arg - the only one is --force" ;;
    esac
done

version="${SKELET_VERSION:-latest}"
dir="${SKELET_INSTALL_DIR:-$HOME/.local/bin}"
if [ -n "${SKELET_DOWNLOAD_URL:-}" ]; then
    base="$SKELET_DOWNLOAD_URL"
elif [ "$version" = latest ]; then
    base=https://github.com/skeletg/skelet/releases/latest/download
else
    base="https://github.com/skeletg/skelet/releases/download/$version"
fi

case "$(uname -s)" in
    Linux)  os=linux ;;
    Darwin) os=darwin ;;
    *)      die "no build for $(uname -s) - skelet runs on macOS, Linux and WSL" ;;
esac
case "$(uname -m)" in
    x86_64|amd64)  arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *)             die "no build for $(uname -m) - there are amd64 and arm64 ones" ;;
esac
asset="skelet-$os-$arch"

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -q -O "$2" "$1"; }
else
    die "needs curl or wget to download"
fi
if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
    die "needs sha256sum or shasum to check the download"
fi

target="$dir/skelet"
if [ -L "$target" ] && [ "$force" -ne 1 ]; then
    die "$target is a symlink to $(readlink "$target") - remove it, or replace it with: curl -fsSL $base/install.sh | sh -s -- --force"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp" "$dir/.skelet.$$"' EXIT
trap 'exit 130' INT TERM

echo "==> downloading $asset ($version)"
fetch "$base/$asset" "$tmp/$asset" || die "could not download $base/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "could not download $base/checksums.txt"
want="$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || die "checksums.txt has no line for $asset"
got="$(sha256 "$tmp/$asset")"
[ "$got" = "$want" ] || die "$asset does not match its checksum (want $want, got $got) - nothing was installed"

mkdir -p "$dir"
# Copied next to the target and renamed over it, so a skelet that is running right now keeps its old file
cp "$tmp/$asset" "$dir/.skelet.$$"
chmod 0755 "$dir/.skelet.$$"
mv -f "$dir/.skelet.$$" "$target"
echo "==> installed $("$target" version) at $target"

case ":$PATH:" in
    *":$dir:"*) ;;
    *) echo "    $dir isn't on your PATH - add it in your shell's profile: export PATH=\"$dir:\$PATH\"" ;;
esac
echo "    start a project with: skelet new my-shop"
