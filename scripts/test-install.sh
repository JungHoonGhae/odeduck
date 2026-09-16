#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
TEST_ROOT=$(mktemp -d)
trap 'rm -rf "$TEST_ROOT"' EXIT

release_base="https://github.com/JungHoonGhae/odeduck/releases/latest/download"
grep -Fq "$release_base/install.sh" "$ROOT/README.md"
grep -Fq "$release_base/install.ps1" "$ROOT/README.md"
grep -Fq "$release_base/install.sh" "$ROOT/docs/promo/launch-kit.md"
grep -Fq "$release_base/install.sh" "$ROOT/install.sh"
grep -Fq "$release_base/install.ps1" "$ROOT/install.ps1"
grep -Fq "$release_base/install.sh" "$ROOT/skills/odeduck/references/setup.md"
grep -Fq "$release_base/install.ps1" "$ROOT/skills/odeduck/references/setup.md"

make_binary() {
    destination="$1"
    name="$2"
    mkdir -p "$destination/payload"
    cat > "$destination/payload/$name" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "version" ]; then
    echo "odeduck test"
fi
exit 0
EOF
    chmod +x "$destination/payload/$name"
    tar -czf "$destination/$name.tar.gz" -C "$destination/payload" "$name"
    rm -rf "$destination/payload"
}

write_checksums() {
    fixture_dir="$1"
    shift
    : > "$fixture_dir/checksums.txt"
    for asset in "$@"; do
        (cd "$fixture_dir" && sha256sum "$asset") >> "$fixture_dir/checksums.txt"
    done
}

make_fake_uname() {
    bin_dir="$1"
    cat > "$bin_dir/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
    -s) echo Linux ;;
    -m) echo x86_64 ;;
    *) echo Linux ;;
esac
EOF
    chmod +x "$bin_dir/uname"
}

make_fake_curl() {
    bin_dir="$1"
    cat > "$bin_dir/curl" <<'EOF'
#!/bin/sh
destination=""
url=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o)
            destination="$2"
            shift 2
            ;;
        -* ) shift ;;
        *)
            url="$1"
            shift
            ;;
    esac
done
asset=${url##*/}
printf 'curl %s\n' "$asset" >> "$TEST_LOG"
if [ "$asset" = "latest" ]; then
    printf 'https://github.com/JungHoonGhae/odeduck/releases/tag/v9.9.9'
    exit 0
fi
[ -n "$destination" ] || exit 1
cp "$FIXTURE_DIR/$asset" "$destination"
EOF
    chmod +x "$bin_dir/curl"
}

make_fake_gh() {
    bin_dir="$1"
    auth_result="$2"
    download_result="$3"
    cat > "$bin_dir/gh" <<EOF
#!/bin/sh
set -e
if [ "\${1:-}" = "auth" ] && [ "\${2:-}" = "status" ]; then
    exit $auth_result
fi
if [ "\${1:-}" = "release" ] && [ "\${2:-}" = "download" ]; then
    asset=""
    destination=""
    while [ "\$#" -gt 0 ]; do
        case "\$1" in
            --pattern) asset="\$2"; shift 2 ;;
            --dir) destination="\$2"; shift 2 ;;
            *) shift ;;
        esac
    done
    printf 'gh %s\n' "\$asset" >> "\$TEST_LOG"
    [ $download_result -eq 0 ] || exit $download_result
    cp "\$FIXTURE_DIR/\$asset" "\$destination/\$asset"
    exit 0
fi
exit 1
EOF
    chmod +x "$bin_dir/gh"
}

run_installer() {
    case_name="$1"
    fixture_dir="$2"
    fake_bin="$3"
    pinned_version="${4-v9.9.9}"
    install_dir="$TEST_ROOT/$case_name/install"
    log_file="$TEST_ROOT/$case_name/downloads.log"
    mkdir -p "$install_dir"
    : > "$log_file"
    FIXTURE_DIR="$fixture_dir" TEST_LOG="$log_file" INSTALL_DIR="$install_dir" \
        ODEDUCK_VERSION="$pinned_version" PATH="$fake_bin:/usr/bin:/bin" \
    sh "$ROOT/install.sh" > "$TEST_ROOT/$case_name/output.log" 2>&1
    "$install_dir/odeduck" version | grep -q '^odeduck test$'
}

setup_case() {
    case_name="$1"
    case_root="$TEST_ROOT/$case_name"
    mkdir -p "$case_root/fixtures" "$case_root/bin"
    make_fake_uname "$case_root/bin"
    make_fake_curl "$case_root/bin"
    printf '%s\n' "$case_root"
}

public_root=$(setup_case public_https)
make_fake_gh "$public_root/bin" 1 1
make_binary "$public_root/fixtures" odeduck
mv "$public_root/fixtures/odeduck.tar.gz" "$public_root/fixtures/odeduck_9.9.9_linux_amd64.tar.gz"
write_checksums "$public_root/fixtures" odeduck_9.9.9_linux_amd64.tar.gz
run_installer public_https "$public_root/fixtures" "$public_root/bin" ""
grep -q '^curl latest$' "$public_root/downloads.log"
grep -q '^curl odeduck_9.9.9_linux_amd64.tar.gz$' "$public_root/downloads.log"

gh_root=$(setup_case authenticated_gh)
make_fake_gh "$gh_root/bin" 0 0
make_binary "$gh_root/fixtures" odeduck
mv "$gh_root/fixtures/odeduck.tar.gz" "$gh_root/fixtures/odeduck_9.9.9_linux_amd64.tar.gz"
printf 'catalog fixture\n' | gzip > "$gh_root/fixtures/odeduck-catalog.json.gz"
write_checksums "$gh_root/fixtures" odeduck_9.9.9_linux_amd64.tar.gz odeduck-catalog.json.gz
run_installer authenticated_gh "$gh_root/fixtures" "$gh_root/bin"
grep -q '^gh odeduck_9.9.9_linux_amd64.tar.gz$' "$gh_root/downloads.log"
if grep -q '^curl ' "$gh_root/downloads.log"; then
    echo "authenticated gh case unexpectedly used curl" >&2
    exit 1
fi

fallback_root=$(setup_case gh_https_fallback)
make_fake_gh "$fallback_root/bin" 0 1
make_binary "$fallback_root/fixtures" odeduck
mv "$fallback_root/fixtures/odeduck.tar.gz" "$fallback_root/fixtures/odeduck_9.9.9_linux_amd64.tar.gz"
write_checksums "$fallback_root/fixtures" odeduck_9.9.9_linux_amd64.tar.gz
run_installer gh_https_fallback "$fallback_root/fixtures" "$fallback_root/bin"
grep -q '^gh odeduck_9.9.9_linux_amd64.tar.gz$' "$fallback_root/downloads.log"
grep -q '^curl odeduck_9.9.9_linux_amd64.tar.gz$' "$fallback_root/downloads.log"

latest_fallback_root=$(setup_case gh_latest_https_fallback)
make_fake_gh "$latest_fallback_root/bin" 0 0
make_binary "$latest_fallback_root/fixtures" odeduck
mv "$latest_fallback_root/fixtures/odeduck.tar.gz" "$latest_fallback_root/fixtures/odeduck_9.9.9_linux_amd64.tar.gz"
write_checksums "$latest_fallback_root/fixtures" odeduck_9.9.9_linux_amd64.tar.gz
run_installer gh_latest_https_fallback "$latest_fallback_root/fixtures" "$latest_fallback_root/bin" ""
grep -q '^curl latest$' "$latest_fallback_root/downloads.log"
grep -q '^gh odeduck_9.9.9_linux_amd64.tar.gz$' "$latest_fallback_root/downloads.log"

mismatch_root=$(setup_case checksum_mismatch)
make_fake_gh "$mismatch_root/bin" 1 1
make_binary "$mismatch_root/fixtures" odeduck
mv "$mismatch_root/fixtures/odeduck.tar.gz" "$mismatch_root/fixtures/odeduck_9.9.9_linux_amd64.tar.gz"
printf '%064d  odeduck_9.9.9_linux_amd64.tar.gz\n' 0 > "$mismatch_root/fixtures/checksums.txt"
mismatch_install="$mismatch_root/install"
mkdir -p "$mismatch_install"
if FIXTURE_DIR="$mismatch_root/fixtures" TEST_LOG="$mismatch_root/downloads.log" \
    INSTALL_DIR="$mismatch_install" ODEDUCK_VERSION=v9.9.9 \
    PATH="$mismatch_root/bin:/usr/bin:/bin" sh "$ROOT/install.sh" \
    > "$mismatch_root/output.log" 2>&1; then
    echo "checksum mismatch was accepted" >&2
    exit 1
fi
test ! -e "$mismatch_install/odeduck"

echo "shell installer tests passed"
