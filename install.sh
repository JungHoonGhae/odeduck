#!/bin/sh
# oddsock 설치 스크립트 (macOS/Linux)
#
#   curl -fsSL https://raw.githubusercontent.com/JungHoonGhae/oddsock/main/install.sh | sh
#
# 환경변수:
#   INSTALL_DIR     설치 위치 (기본 /usr/local/bin)
#   ODDSOCK_VERSION 특정 버전 고정 (예: v0.4.0, 기본 latest)
#   OPENDATACTL_VERSION / GONGCTL_VERSION 이전 변수명(호환용)
set -e

REPO="JungHoonGhae/oddsock"
BINARY="oddsock"
FORMER_BINARY="opendatactl"
LEGACY_BINARY="gongctl"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

main() {
    os=$(detect_os)
    arch=$(detect_arch)

    if [ "$os" = "windows" ]; then
        echo "Error: this script does not support Windows. Use PowerShell instead:"
        echo '  (& gh api -H "Accept: application/vnd.github.raw+json" repos/'"$REPO"'/contents/install.ps1) | Out-String | Invoke-Expression'
        exit 1
    fi

    version="${ODDSOCK_VERSION:-${OPENDATACTL_VERSION:-${GONGCTL_VERSION:-$(latest_version)}}}"
    [ -n "$version" ] || { echo "Error: could not resolve latest version."; exit 1; }
    ver_no_v="${version#v}"

    asset="${BINARY}_${ver_no_v}_${os}_${arch}.tar.gz"
    echo "Detected: ${os}/${arch}"
    echo "Installing ${BINARY} ${version}..."

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    if ! download_asset "$version" "$asset" "${tmpdir}/${asset}"; then
        asset="${FORMER_BINARY}_${ver_no_v}_${os}_${arch}.tar.gz"
        if ! download_asset "$version" "$asset" "${tmpdir}/${asset}"; then
            # v0.8 and earlier only shipped gongctl_* archives.
            asset="${LEGACY_BINARY}_${ver_no_v}_${os}_${arch}.tar.gz"
            download_asset "$version" "$asset" "${tmpdir}/${asset}"
        fi
    fi
    catalog_asset="oddsock-catalog.json.gz"
    catalog_available=0
    if download_asset "$version" "$catalog_asset" "${tmpdir}/${catalog_asset}"; then
        catalog_available=1
    else
        catalog_asset="opendatactl-catalog.json.gz"
        if download_asset "$version" "$catalog_asset" "${tmpdir}/${catalog_asset}"; then
            catalog_available=1
        else
            echo "Prebuilt catalogue is not available for ${version}; install will continue without it."
        fi
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
    if [ ! -f "${tmpdir}/${BINARY}" ]; then
        if [ -f "${tmpdir}/${FORMER_BINARY}" ]; then
            cp "${tmpdir}/${FORMER_BINARY}" "${tmpdir}/${BINARY}"
        elif [ -f "${tmpdir}/${LEGACY_BINARY}" ]; then
            cp "${tmpdir}/${LEGACY_BINARY}" "${tmpdir}/${BINARY}"
        fi
    fi
    if [ ! -f "${tmpdir}/${FORMER_BINARY}" ]; then
        cp "${tmpdir}/${BINARY}" "${tmpdir}/${FORMER_BINARY}"
    fi
    if [ ! -f "${tmpdir}/${LEGACY_BINARY}" ]; then
        cp "${tmpdir}/${BINARY}" "${tmpdir}/${LEGACY_BINARY}"
    fi

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
    $SUDO mv "${tmpdir}/${FORMER_BINARY}" "${INSTALL_DIR}/${FORMER_BINARY}"
    $SUDO chmod +x "${INSTALL_DIR}/${FORMER_BINARY}"
    $SUDO mv "${tmpdir}/${LEGACY_BINARY}" "${INSTALL_DIR}/${LEGACY_BINARY}"
    $SUDO chmod +x "${INSTALL_DIR}/${LEGACY_BINARY}"

    if [ "$catalog_available" -eq 1 ]; then
        "${INSTALL_DIR}/${BINARY}" catalog install-snapshot "${tmpdir}/${catalog_asset}" -f table
    fi

    echo ""
    echo "Installed: $("${INSTALL_DIR}/${BINARY}" version 2>/dev/null || echo "$BINARY")"
    echo "Compatibility aliases: ${FORMER_BINARY}, ${LEGACY_BINARY}"
    echo ""
    echo "Next steps:"
    echo "  oddsock login                                 # 브라우저 1회 로그인"
    echo "  oddsock search 대기오염 --type api -f table    # 데이터셋 검색"
    echo "  oddsock apply <pk> --purpose ... --category research"
}

latest_version() {
    if gh_authenticated; then
        gh release view --repo "$REPO" --json tagName --jq .tagName
        return
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
