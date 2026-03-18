#!/bin/sh

set -eu

REPO_OWNER="Z-Bra0"
REPO_NAME="Ski"
INSTALL_DIR="${SKI_INSTALL_DIR:-$HOME/.local/bin}"
VERSION=""

usage() {
	cat <<'EOF'
Usage: install.sh [--version v0.1.0] [--dir /path/to/bin]

Options:
  --version   Release tag to install. Defaults to the latest GitHub release.
  --dir       Install directory. Defaults to $SKI_INSTALL_DIR or ~/.local/bin.
  -h, --help  Show this help text.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--version)
			if [ "$#" -lt 2 ]; then
				echo "missing value for --version" >&2
				exit 1
			fi
			VERSION="$2"
			shift 2
			;;
		--dir)
			if [ "$#" -lt 2 ]; then
				echo "missing value for --dir" >&2
				exit 1
			fi
			INSTALL_DIR="$2"
			shift 2
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "unknown argument: $1" >&2
			usage >&2
			exit 1
			;;
	esac
done

need_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "required command not found: $1" >&2
		exit 1
	fi
}

sha256_file() {
	file="$1"

	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | cut -d ' ' -f 1
		return
	fi

	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | cut -d ' ' -f 1
		return
	fi

	if command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$file" | sed 's/^.*= //'
		return
	fi

	echo "sha256sum, shasum, or openssl is required to verify ski" >&2
	exit 1
}

download_to() {
	url="$1"
	dest="$2"

	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$dest"
		return
	fi

	if command -v wget >/dev/null 2>&1; then
		wget -qO "$dest" "$url"
		return
	fi

	echo "curl or wget is required to download ski" >&2
	exit 1
}

fetch_text() {
	url="$1"

	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url"
		return
	fi

	if command -v wget >/dev/null 2>&1; then
		wget -qO- "$url"
		return
	fi

	echo "curl or wget is required to download ski" >&2
	exit 1
}

resolve_version() {
	if [ -n "$VERSION" ]; then
		return
	fi

	api_url="https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/releases/latest"
	VERSION="$(fetch_text "$api_url" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"

	if [ -z "$VERSION" ]; then
		echo "failed to resolve the latest ski release tag" >&2
		exit 1
	fi
}

detect_platform() {
	case "$(uname -s)" in
		Darwin)
			OS="darwin"
			;;
		Linux)
			OS="linux"
			;;
		*)
			echo "unsupported operating system: $(uname -s)" >&2
			exit 1
			;;
	esac

	case "$(uname -m)" in
		x86_64|amd64)
			ARCH="amd64"
			;;
		arm64|aarch64)
			ARCH="arm64"
			;;
		*)
			echo "unsupported architecture: $(uname -m)" >&2
			exit 1
			;;
	esac
}

need_cmd uname
need_cmd mktemp
need_cmd tar
need_cmd mkdir
need_cmd chmod
need_cmd cp
need_cmd find
need_cmd sed
need_cmd grep
need_cmd cut

detect_platform
resolve_version

artifact="ski_${VERSION#v}_${OS}_${ARCH}.tar.gz"
download_url="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$VERSION/$artifact"
checksums_name="ski_${VERSION#v}_checksums.txt"
checksums_url="https://github.com/$REPO_OWNER/$REPO_NAME/releases/download/$VERSION/$checksums_name"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT INT TERM

archive_path="$tmpdir/$artifact"
checksums_path="$tmpdir/$checksums_name"
extract_dir="$tmpdir/extract"

mkdir -p "$extract_dir" "$INSTALL_DIR"

echo "Downloading $download_url"
download_to "$download_url" "$archive_path"

echo "Downloading $checksums_url"
download_to "$checksums_url" "$checksums_path"

expected_sha256="$(grep "  $artifact\$" "$checksums_path" | cut -d ' ' -f 1 | head -n 1)"
if [ -z "$expected_sha256" ]; then
	echo "failed to find a checksum for $artifact" >&2
	exit 1
fi

actual_sha256="$(sha256_file "$archive_path")"
if [ "$actual_sha256" != "$expected_sha256" ]; then
	echo "checksum verification failed for $artifact" >&2
	echo "expected: $expected_sha256" >&2
	echo "actual:   $actual_sha256" >&2
	exit 1
fi

echo "Verified SHA-256 checksum for $artifact"

tar -xzf "$archive_path" -C "$extract_dir"

binary_path="$(find "$extract_dir" -type f -name ski | head -n 1)"
if [ -z "$binary_path" ]; then
	echo "downloaded archive did not contain a ski binary" >&2
	exit 1
fi

install_path="$INSTALL_DIR/ski"
cp "$binary_path" "$install_path"
chmod 755 "$install_path"

echo "Installed ski $VERSION to $install_path"

case ":$PATH:" in
	*":$INSTALL_DIR:"*)
		;;
	*)
		echo "Add $INSTALL_DIR to your PATH to run ski from any shell."
		;;
esac
