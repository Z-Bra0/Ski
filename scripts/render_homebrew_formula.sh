#!/bin/sh

set -eu

REPO_OWNER="${REPO_OWNER:-Z-Bra0}"
REPO_NAME="${REPO_NAME:-Ski}"
FORMULA_NAME="${FORMULA_NAME:-skicli}"
FORMULA_CLASS="${FORMULA_CLASS:-Skicli}"
DESCRIPTION="${DESCRIPTION:-Package manager for AI agent skills}"
HOMEPAGE="${HOMEPAGE:-https://github.com/Z-Bra0/Ski}"
LICENSE_ID="${LICENSE_ID:-GPL-3.0-only}"
TEMPLATE_FILE="${TEMPLATE_FILE:-templates/skicli.tmpl.rb}"

VERSION=""
CHECKSUMS_FILE=""
OUTPUT=""

usage() {
	cat <<'EOF'
Usage: render_homebrew_formula.sh --version v0.1.1 [--checksums dist/ski_0.1.1_checksums.txt] [--output /tmp/skicli.rb]

Options:
  --version    Release version to render, with or without a leading v.
  --checksums  Path to the checksum file. Defaults to dist/ski_<version>_checksums.txt.
  --output     Output path. Defaults to stdout.
  -h, --help   Show this help text.
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
		--checksums)
			if [ "$#" -lt 2 ]; then
				echo "missing value for --checksums" >&2
				exit 1
			fi
			CHECKSUMS_FILE="$2"
			shift 2
			;;
		--output)
			if [ "$#" -lt 2 ]; then
				echo "missing value for --output" >&2
				exit 1
			fi
			OUTPUT="$2"
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

if [ -z "$VERSION" ]; then
	echo "--version is required" >&2
	usage >&2
	exit 1
fi

case "$VERSION" in
	v*)
		VERSION_TAG="$VERSION"
		VERSION_NUMBER="${VERSION#v}"
		;;
	*)
		VERSION_TAG="v$VERSION"
		VERSION_NUMBER="$VERSION"
		;;
esac

if [ -z "$CHECKSUMS_FILE" ]; then
	CHECKSUMS_FILE="dist/ski_${VERSION_NUMBER}_checksums.txt"
fi

if [ ! -f "$CHECKSUMS_FILE" ]; then
	echo "checksum file not found: $CHECKSUMS_FILE" >&2
	exit 1
fi

if [ ! -f "$TEMPLATE_FILE" ]; then
	echo "template file not found: $TEMPLATE_FILE" >&2
	exit 1
fi

need_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "required command not found: $1" >&2
		exit 1
	fi
}

need_cmd awk
need_cmd cat
need_cmd mkdir
need_cmd sed

checksum_for() {
	artifact="$1"
	awk -v target="$artifact" '
		$2 == target || $2 == ("dist/" target) { print $1; exit }
	' "$CHECKSUMS_FILE"
}

DARWIN_ARM64_SHA="$(checksum_for "ski_${VERSION_NUMBER}_darwin_arm64.tar.gz")"
DARWIN_AMD64_SHA="$(checksum_for "ski_${VERSION_NUMBER}_darwin_amd64.tar.gz")"
LINUX_AMD64_SHA="$(checksum_for "ski_${VERSION_NUMBER}_linux_amd64.tar.gz")"
LINUX_ARM64_SHA="$(checksum_for "ski_${VERSION_NUMBER}_linux_arm64.tar.gz")"

for value in \
	"$DARWIN_ARM64_SHA" \
	"$DARWIN_AMD64_SHA" \
	"$LINUX_AMD64_SHA" \
	"$LINUX_ARM64_SHA"
do
	if [ -z "$value" ]; then
		echo "missing expected checksum in $CHECKSUMS_FILE" >&2
		exit 1
	fi
done

escape_replacement() {
	printf '%s' "$1" | sed 's/[&|]/\\&/g'
}

render_formula() {
	template_content="$(cat "$TEMPLATE_FILE")"

	printf '%s' "$template_content" | sed \
		-e "s|{{FORMULA_CLASS}}|$(escape_replacement "$FORMULA_CLASS")|g" \
		-e "s|{{DESCRIPTION}}|$(escape_replacement "$DESCRIPTION")|g" \
		-e "s|{{HOMEPAGE}}|$(escape_replacement "$HOMEPAGE")|g" \
		-e "s|{{LICENSE_ID}}|$(escape_replacement "$LICENSE_ID")|g" \
		-e "s|{{REPO_OWNER}}|$(escape_replacement "$REPO_OWNER")|g" \
		-e "s|{{REPO_NAME}}|$(escape_replacement "$REPO_NAME")|g" \
		-e "s|{{FORMULA_NAME}}|$(escape_replacement "$FORMULA_NAME")|g" \
		-e "s|{{VERSION_TAG}}|$(escape_replacement "$VERSION_TAG")|g" \
		-e "s|{{VERSION_NUMBER}}|$(escape_replacement "$VERSION_NUMBER")|g" \
		-e "s|{{DARWIN_ARM64_SHA}}|$(escape_replacement "$DARWIN_ARM64_SHA")|g" \
		-e "s|{{DARWIN_AMD64_SHA}}|$(escape_replacement "$DARWIN_AMD64_SHA")|g" \
		-e "s|{{LINUX_AMD64_SHA}}|$(escape_replacement "$LINUX_AMD64_SHA")|g" \
		-e "s|{{LINUX_ARM64_SHA}}|$(escape_replacement "$LINUX_ARM64_SHA")|g"
}

if [ -n "$OUTPUT" ]; then
	mkdir -p "$(dirname "$OUTPUT")"
	render_formula > "$OUTPUT"
else
	render_formula
fi
