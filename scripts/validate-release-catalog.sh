#!/bin/sh
# Shared, offline gate for newly collected and reused catalogue bytes.
set -eu
catalog_binary=${1:?usage: validate-release-catalog.sh <odeduck-binary> <catalog.json.gz>}
catalog_snapshot=${2:?catalogue snapshot required}

# Bound decompression and validate structure before jq reads the archive.
"$catalog_binary" catalog install-snapshot "$catalog_snapshot" --check-only --format json
coverage=$(gzip -dc "$catalog_snapshot" | jq '{
  source, type, syncedAt, total: (.entries | length),
  api: ([.entries[] | select((.dataTypes // []) | index("API"))] | length),
  file: ([.entries[] | select((.dataTypes // []) | index("FILE"))] | length),
  dual: ([.entries[] | select(((.dataTypes // []) | index("API")) and ((.dataTypes // []) | index("FILE")))] | length),
  rest: ([.entries[] | select(.svcType == "REST")] | length),
  link: ([.entries[] | select(.svcType == "LINK")] | length),
  unknown: ([.entries[] | select((.svcType // "") == "")] | length),
  officialContracts: ([.entries[] | select(.officialApi != null)] | length)
}')
printf '%s\n' "$coverage"
if ! printf '%s\n' "$coverage" | jq -e '
  .source == "official-file+web" and .type == "ALL" and
  .total > 90000 and .api > 11000 and .file > 80000 and .dual > 50000 and
  .rest > 6000 and .link > 3500 and .unknown < 500 and .officialContracts > 11000
' >/dev/null; then
  echo 'error: catalogue coverage regression' >&2
  exit 1
fi
"$catalog_binary" catalog validate-release --snapshot "$catalog_snapshot" --format json
