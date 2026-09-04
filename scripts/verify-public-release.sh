#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
VERSION=${1:-v$(tr -d '\r\n' < "$ROOT/VERSION")}
REPOSITORY=${ODDSOCK_REPOSITORY:-JungHoonGhae/oddsock}
WEB_BASE=${ODDSOCK_WEB_BASE:-https://github.com/$REPOSITORY}
RELEASE_BASE=${ODDSOCK_RELEASE_BASE:-$WEB_BASE/releases/download/$VERSION}

printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$' || {
    echo "error: version must look like v0.16.1" >&2
    exit 2
}

for command_name in awk curl grep jq gzip tar; do
    command -v "$command_name" >/dev/null 2>&1 || {
        echo "error: $command_name is required" >&2
        exit 2
    }
done
if ! command -v shasum >/dev/null 2>&1 && ! command -v sha256sum >/dev/null 2>&1; then
    echo "error: shasum or sha256sum is required" >&2
    exit 2
fi

case "$(uname -s)" in
    Darwin) platform=darwin ;;
    Linux) platform=linux ;;
    *) echo "error: public smoke test supports macOS and Linux runners" >&2; exit 2 ;;
esac
case "$(uname -m)" in
    x86_64 | amd64) architecture=amd64 ;;
    arm64 | aarch64) architecture=arm64 ;;
    *) echo "error: unsupported architecture $(uname -m)" >&2; exit 2 ;;
esac

SMOKE_ROOT=$(mktemp -d)
trap 'rm -rf "$SMOKE_ROOT"' EXIT

download() {
    asset_name=$1
    curl -fsSL "$RELEASE_BASE/$asset_name" -o "$SMOKE_ROOT/$asset_name"
}

checksum_of() {
    checksum_file=$1
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$checksum_file" | awk '{print $1}'
    else
        sha256sum "$checksum_file" | awk '{print $1}'
    fi
}

verify_asset() {
    verified_asset=$1
    expected=$(awk -v name="$verified_asset" '$2 == name {print $1; exit}' "$SMOKE_ROOT/checksums.txt")
    [ -n "$expected" ] || {
        echo "error: checksums.txt does not list $verified_asset" >&2
        exit 1
    }
    actual=$(checksum_of "$SMOKE_ROOT/$verified_asset")
    [ "$actual" = "$expected" ] || {
        echo "error: checksum mismatch for $verified_asset" >&2
        exit 1
    }
}

echo "Checking anonymous repository and release access..."
curl -fsSL "$WEB_BASE" -o /dev/null
curl -fsSL "$WEB_BASE/releases/tag/$VERSION" -o /dev/null

archive="oddsock_${VERSION#v}_${platform}_${architecture}.tar.gz"
for asset in checksums.txt install.sh install.ps1 oddsock-catalog.json.gz "$archive"; do
    download "$asset"
done
for asset in install.sh install.ps1 oddsock-catalog.json.gz "$archive"; do
    verify_asset "$asset"
done

echo "Running the pinned installer without GitHub CLI authentication..."
mkdir -p "$SMOKE_ROOT/no-gh" "$SMOKE_ROOT/bin" "$SMOKE_ROOT/home" "$SMOKE_ROOT/config"
printf '#!/bin/sh\nexit 1\n' > "$SMOKE_ROOT/no-gh/gh"
chmod +x "$SMOKE_ROOT/no-gh/gh"
HOME="$SMOKE_ROOT/home" XDG_CONFIG_HOME="$SMOKE_ROOT/config" \
    INSTALL_DIR="$SMOKE_ROOT/bin" ODDSOCK_VERSION="$VERSION" \
    PATH="$SMOKE_ROOT/no-gh:$PATH" sh "$SMOKE_ROOT/install.sh"

version_output=$(HOME="$SMOKE_ROOT/home" XDG_CONFIG_HOME="$SMOKE_ROOT/config" \
    "$SMOKE_ROOT/bin/oddsock" version)
printf '%s\n' "$version_output" | grep -F "${VERSION#v}" >/dev/null

HOME="$SMOKE_ROOT/home" XDG_CONFIG_HOME="$SMOKE_ROOT/config" \
    "$SMOKE_ROOT/bin/oddsock" catalog info -f json > "$SMOKE_ROOT/catalog-info.json"
jq -e '.entries >= 90000 and .svcTypes.REST > 6000 and .svcTypes.FILE > 80000' \
    "$SMOKE_ROOT/catalog-info.json" >/dev/null

HOME="$SMOKE_ROOT/home" XDG_CONFIG_HOME="$SMOKE_ROOT/config" \
    "$SMOKE_ROOT/bin/oddsock" catalog search \
    "서울에서 작은 가게 후보를 좁힐 자료" \
    --concept 생활인구 --concept 추정매출 \
    --concept "상권 점포" --concept "상권 개폐업" \
    --limit 8 --semantic=false -f json > "$SMOKE_ROOT/search.json"
jq -e '.total > 0 and (.hits | length) > 0' "$SMOKE_ROOT/search.json" >/dev/null

echo "public release smoke passed: $REPOSITORY $VERSION ($platform/$architecture)"
