#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT
FIXTURES="$TEST_ROOT/fixtures"
FAKE_BIN="$TEST_ROOT/fake-bin"
mkdir -p "$FIXTURES/payload" "$FAKE_BIN"

cat > "$FIXTURES/payload/oddsock" <<'EOF'
#!/bin/sh
case "$*" in
    version)
        echo "oddsock 9.9.9 (commit test, built test)"
        ;;
    "catalog install-snapshot"*)
        exit 0
        ;;
    "catalog info"*)
        echo '{"entries":96683,"svcTypes":{"REST":7000,"FILE":84000}}'
        ;;
    "catalog search"*)
        echo '{"total":4,"hits":[{"pk":"fixture"}]}'
        ;;
    *)
        echo "unexpected fake oddsock arguments: $*" >&2
        exit 1
        ;;
esac
EOF
chmod +x "$FIXTURES/payload/oddsock"
tar -czf "$FIXTURES/oddsock_9.9.9_linux_amd64.tar.gz" -C "$FIXTURES/payload" oddsock
cp "$ROOT/install.sh" "$FIXTURES/install.sh"
cp "$ROOT/install.ps1" "$FIXTURES/install.ps1"
printf 'catalog fixture\n' | gzip > "$FIXTURES/oddsock-catalog.json.gz"

(
    cd "$FIXTURES"
    : > checksums.txt
    for asset in install.sh install.ps1 oddsock-catalog.json.gz oddsock_9.9.9_linux_amd64.tar.gz; do
        if command -v shasum >/dev/null 2>&1; then
            shasum -a 256 "$asset"
        else
            sha256sum "$asset"
        fi
    done >> checksums.txt
)

cat > "$FAKE_BIN/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
    -s) echo Linux ;;
    -m) echo x86_64 ;;
    *) echo Linux ;;
esac
EOF
chmod +x "$FAKE_BIN/uname"

cat > "$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
destination=""
url=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o) destination=$2; shift 2 ;;
        -w) shift 2 ;;
        -*) shift ;;
        *) url=$1; shift ;;
    esac
done
[ -n "$destination" ] || exit 2
asset=${url##*/}
case "$asset" in
    oddsock | v9.9.9)
        : > "$destination"
        ;;
    *)
        cp "$FIXTURE_DIR/$asset" "$destination"
        ;;
esac
EOF
chmod +x "$FAKE_BIN/curl"

FIXTURE_DIR="$FIXTURES" PATH="$FAKE_BIN:$PATH" \
    ODDSOCK_WEB_BASE="https://example.invalid/oddsock" \
    "$ROOT/scripts/verify-public-release.sh" v9.9.9 \
    > "$TEST_ROOT/success.log"
grep -q '^public release smoke passed:' "$TEST_ROOT/success.log"

printf 'tampered\n' >> "$FIXTURES/oddsock_9.9.9_linux_amd64.tar.gz"
if FIXTURE_DIR="$FIXTURES" PATH="$FAKE_BIN:$PATH" \
    ODDSOCK_WEB_BASE="https://example.invalid/oddsock" \
    "$ROOT/scripts/verify-public-release.sh" v9.9.9 \
    > "$TEST_ROOT/mismatch.log" 2>&1; then
    echo "public release smoke accepted a checksum mismatch" >&2
    exit 1
fi
grep -q 'checksum mismatch' "$TEST_ROOT/mismatch.log"

echo "public release smoke tests passed"
