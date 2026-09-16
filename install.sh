#!/bin/sh
# odeduck 설치 스크립트 (macOS/Linux)
#
#   curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.sh | sh
#
# 환경변수:
#   INSTALL_DIR     설치 위치 (기본 /usr/local/bin)
#   ODEDUCK_VERSION 특정 버전 고정 (예: v0.19.0, 기본 latest)
set -e

REPO="JungHoonGhae/odeduck"
BINARY="odeduck"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

main() {
    os=$(detect_os)
    arch=$(detect_arch)
    version="${ODEDUCK_VERSION:-$(latest_version)}"
    [ -n "$version" ] || { echo "Error: could not resolve latest version."; exit 1; }

    if [ "$os" = "windows" ]; then
        echo "Error: this script does not support Windows. Use PowerShell instead:"
        echo '  irm https://github.com/'"$REPO"'/releases/download/'"$version"'/install.ps1 | iex'
        exit 1
    fi

    ver_no_v="${version#v}"

    asset="${BINARY}_${ver_no_v}_${os}_${arch}.tar.gz"
    echo "Detected: ${os}/${arch}"
    echo "Installing ${BINARY} ${version}..."

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    download_asset "$version" "$asset" "${tmpdir}/${asset}"
    catalog_asset="odeduck-catalog.json.gz"
    catalog_available=0
    if download_asset "$version" "$catalog_asset" "${tmpdir}/${catalog_asset}"; then
        catalog_available=1
    else
        echo "Prebuilt catalogue is not available for ${version}; install will continue without it."
    fi
    download_asset "$version" "checksums.txt" "${tmpdir}/checksums.txt"

    echo "Verifying checksum..."
    (
        cd "$tmpdir"
        grep " ${asset}\$" checksums.txt > asset.sha256
        if command -v shasum >/dev/null 2>&1; then
            shasum -a 256 -c asset.sha256 >/dev/null
        else
            sha256sum -c asset.sha256 >/dev/null
        fi
        if [ "$catalog_available" -eq 1 ]; then
            grep " ${catalog_asset}\$" checksums.txt > catalog.sha256
            if command -v shasum >/dev/null 2>&1; then
                shasum -a 256 -c catalog.sha256 >/dev/null
            else
                sha256sum -c catalog.sha256 >/dev/null
            fi
        fi
    )

    tar -xzf "${tmpdir}/${asset}" -C "$tmpdir"
    [ -f "${tmpdir}/${BINARY}" ] || { echo "Error: ${BINARY} missing from ${asset}."; exit 1; }

    if [ "$catalog_available" -eq 1 ]; then
        "${tmpdir}/${BINARY}" catalog install-snapshot "${tmpdir}/${catalog_asset}" --check-only -f table
    fi

    # 쓰기 가능하면 그대로, 아니면 sudo. /usr/local/bin 이 없는 환경도 있으므로
    # mkdir -p 를 먼저 한다.
    if can_write "$INSTALL_DIR"; then
        SUDO=""
    else
        SUDO="sudo"
        echo "Elevated permissions required to write ${INSTALL_DIR}."
    fi
    $SUDO mkdir -p "$INSTALL_DIR"
    $SUDO mv "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
    $SUDO chmod +x "${INSTALL_DIR}/${BINARY}"

    if [ "$catalog_available" -eq 1 ]; then
        "${INSTALL_DIR}/${BINARY}" catalog install-snapshot "${tmpdir}/${catalog_asset}" -f table
    fi

    echo ""
    echo "Installed: $("${INSTALL_DIR}/${BINARY}" version 2>/dev/null || echo "$BINARY")"
    echo ""
    echo "Next steps:"
    echo "  odeduck goal --help    # 목표 실행과 검토 모델 설정"
    echo "  odeduck mcp --help     # AI 앱에서 같은 목표 실행기 사용"
    echo "  odeduck login          # 인증 API를 쓸 때 브라우저 로그인"
}

latest_version() {
    if gh_authenticated; then
        if resolved_version=$(gh release view --repo "$REPO" --json tagName --jq .tagName 2>/dev/null); then
            if [ -n "$resolved_version" ]; then
                printf '%s\n' "$resolved_version"
                return
            fi
        fi
    fi
    # API 대신 releases/latest 리다이렉트에서 태그 추출 (rate limit 없음)
    curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest" \
        | sed 's#.*/tag/##'
}

download_asset() {
    release_version="$1"
    asset_name="$2"
    destination="$3"
    if gh_authenticated; then
        if gh release download "$release_version" --repo "$REPO" --pattern "$asset_name" --dir "$(dirname "$destination")" --clobber; then
            return
        fi
    fi
    curl -fsSL -o "$destination" "https://github.com/${REPO}/releases/download/${release_version}/${asset_name}"
}

gh_authenticated() {
    command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1
}

detect_os() {
    case "$(uname -s)" in
        Darwin) echo "darwin" ;;
        Linux) echo "linux" ;;
        MINGW* | MSYS* | CYGWIN*) echo "windows" ;;
        *) echo "Error: unsupported OS $(uname -s)" >&2; exit 1 ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo "amd64" ;;
        arm64 | aarch64) echo "arm64" ;;
        *) echo "Error: unsupported architecture $(uname -m)" >&2; exit 1 ;;
    esac
}

# dir 이 아직 없으면 존재하는 최상위 조상에 쓰기 가능한지 본다 (mkdir -p 대비).
can_write() {
    d="$1"
    while [ ! -d "$d" ]; do
        d=$(dirname "$d")
    done
    [ -w "$d" ]
}

main "$@"
