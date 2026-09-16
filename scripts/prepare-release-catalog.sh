#!/bin/sh
# Reuse a published snapshot. A failed gate never falls back to portal scraping.
set -eu
catalog_binary=${1:?usage: prepare-release-catalog.sh <odeduck-binary> <output-dir> [source-tag]}
catalog_output=${2:?output directory required}
catalog_requested_tag=${3:-}
catalog_repository=JungHoonGhae/odeduck
catalog_asset=odeduck-catalog.json.gz
catalog_script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

for target in "$catalog_output/$catalog_asset" "$catalog_output/catalog-source.json"; do
  if [ -e "$target" ]; then
    echo "error: output already exists: $target" >&2
    exit 1
  fi
done
catalog_stage=$(mktemp -d)
trap 'rm -rf "$catalog_stage"' EXIT

# Include dedicated catalogue prereleases, but never application previews,
# drafts, or the application tag currently being packaged (including retries).
gh api --paginate "repos/$catalog_repository/releases" > "$catalog_stage/releases.json"
catalog_tag=$(jq -sr --arg requested "$catalog_requested_tag" --arg current "${GITHUB_REF_NAME:-}" '
  [.[][] | select(.draft == false and .published_at != null and .tag_name != $current)
   | select((.tag_name | test("^catalog-[0-9]{8}-[0-9]+-[0-9]+$")) or
            (.prerelease == false and (.tag_name | test("^v[0-9]+\\.[0-9]+\\.[0-9]+$"))))
   | select($requested == "" or .tag_name == $requested)]
  | sort_by([(.tag_name | startswith("catalog-")), .published_at]) | last | .tag_name // empty
' "$catalog_stage/releases.json")
if [ -z "$catalog_tag" ]; then
  echo 'error: no published catalogue source; run Catalog refresh or select a published source tag' >&2
  exit 1
fi
echo "Reusing catalogue from $catalog_tag"
gh release download "$catalog_tag" --repo "$catalog_repository" \
  --pattern "$catalog_asset" --pattern checksums.txt --dir "$catalog_stage"

catalog_expected=$(awk -v asset="$catalog_asset" '$2 == asset {print $1}' "$catalog_stage/checksums.txt")
if ! printf '%s\n' "$catalog_expected" | awk '
  NR != 1 || length($0) != 64 || $0 !~ /^[0-9a-f]+$/ {bad=1}
  END {exit (bad || NR != 1)}
'; then
  echo 'error: invalid catalogue checksum (missing, malformed or duplicated)' >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  catalog_actual=$(sha256sum "$catalog_stage/$catalog_asset" | awk '{print $1}')
else
  catalog_actual=$(shasum -a 256 "$catalog_stage/$catalog_asset" | awk '{print $1}')
fi
if [ "$catalog_actual" != "$catalog_expected" ]; then
  echo 'error: catalogue checksum mismatch' >&2
  exit 1
fi
sh "$catalog_script_dir/validate-release-catalog.sh" "$catalog_binary" "$catalog_stage/$catalog_asset"

# Preserve the compressed bytes and their original syncedAt, not a fresh timestamp.
jq -n --arg sourceTag "$catalog_tag" --arg sha256 "$catalog_actual" \
  '{sourceTag: $sourceTag, sha256: $sha256}' > "$catalog_stage/catalog-source.json"
mkdir -p "$catalog_output"
cp "$catalog_stage/$catalog_asset" "$catalog_output/$catalog_asset"
cp "$catalog_stage/catalog-source.json" "$catalog_output/catalog-source.json"
