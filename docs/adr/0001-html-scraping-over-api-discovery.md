---
status: accepted
---

# API-first discovery with deterministic HTML fallback and loud drift detection

oddsock originally read data.go.kr by scraping HTML, which looks fragile enough that
"surely there's a better way" keeps coming up — an adaptive-selector library
(Scrapling), a crawler framework (goscrapy), an AI browser agent (Browser Use
et al), or discovering the JSON APIs beneath the pages (Unbrowse's approach). We
investigated the last one against the live portal. That investigation correctly
found that the browser pages themselves return server-rendered HTML, but it drew
too broad a conclusion: data.go.kr separately publishes an official catalogue
API and standardized dataset metadata. The amended decision is therefore:

1. use a documented official API when it carries the required fact;
2. otherwise use the first-party Schema.org/DCAT metadata representation;
3. parse deterministic first-party HTML or form contracts only for facts that
   the machine interfaces omit, such as a concrete file asset identifier;
4. use a browser only for human SSO and form-owned state changes.

Every fallback is labelled in the returned evidence and protected by a loud
drift signal. A working HTML parser is not described as a public API.

## Correction and revalidation (2026-09-01)

The official `공공 데이터 포털 목록 조회 API` (public data PK `15077093`)
publishes four JSON operations, including `dataset`, `file-data-list` and
`open-data-list`. Its public test key reported 96,519 current logical datasets,
compared with the 95,951 unique nodes collected by the existing HTML sweep, and
returned richer fields such as `page_url`, `meta_url`, formats, update dates and
OpenAPI operation metadata. A normal key must have utilization approval to page
or filter; the documented test key deliberately disables those features.

The official catalogue API is implemented as the richest source, but live
verification found its application restricted to administrative/public-sector
accounts. The portal also publishes a public monthly 목록개방현황 CSV (PK
`15062804`): 96,110 rows streamed in 22 seconds without login or a secret. It
accurately classified 7,176 REST and 4,778 LINK lists, but omitted many
portal-generated API+FILE alternatives. Release packaging therefore joins that
official file with the current web delivery discovery, rejects exports below
90,000 nodes or per-type coverage thresholds, and includes the composite gzip
artifact in the release checksum. `catalog sync --source official` remains a
fail-closed richer option for approved institutional keys; `official-file` is
the fast public source and `official-file+web` is the release-quality source.
Installers validate
and atomically install it while preserving a newer local snapshot. This keeps a
complete no-login path without requiring every end user to apply for the bulk
catalogue operation. The HTML sweep remains an explicit compatibility fallback
for older releases and upstream outages.

The institution-key `official` mode joins the logical `dataset` list with
`open-data-list` operations and `file-data-list` versions by public list ID.
The distributable composite preserves REST/LINK/FILE co-representations,
approval modes and formats; operation details are then loaded from the exact
official Swagger or labelled detail fallback when a user inspects one selected
node. This keeps the global snapshot compact and publicly reproducible.
The release gate checks API/FILE and REST/LINK coverage plus unknown-type and
official-contract counts; total row count alone is not accepted as completeness.
Required/optional flags and sample parameter values are not present in the bulk
API, so the detail-page contract remains a labelled fallback for those facts.

For an individual FILE node, `/catalog/{pk}/fileData.json` supplies official
Schema.org metadata but omits the actual download identifier, historical asset
list and column schema. `inspect_dataset` therefore reads that JSON first and
uses the detail page only to resolve a typed, origin-constrained asset request.

Provider catalogues follow the same rule. For example, Seoul Open Data Plaza's
documented `SearchOpenDataServiceList` API identifies that `OA-15572` is offered
as SHEET, FILE and OPENAPI and gives the canonical URLs. Only the final versioned
file list is read from the HTTPS provider page because the catalogue response
does not include file sequence IDs. Seoul's credentialed data API remains HTTP
only and allows at most 1,000 rows per request, so oddsock does not send a
user key over that transport; the complete HTTPS file archive is the safe path
for full local analysis.

## Evidence (2026-07-25, live authenticated session)

XHR/fetch captured via CDP while loading each page:

| Page | Data-bearing XHR/fetch |
|---|---|
| `selectDataSetList.do` (search) | none — only a `.hbs` template and Google Analytics |
| `openapi.do` (describe) | none — only `selectMberInfo.json` (member info) |
| `selectAcountList.do` (applications) | none — only `selectCommonCodeSelectboxList.json` (select-box codes) |
| apply submit | **`POST /iim/api/saveDevAcountRequest.do`** — a real endpoint |

## Revalidation (2026-08-31, KRDS redesign)

The portal replaced its search and account-list markup with KRDS components, so
we repeated API discovery before changing selectors. CDP captured request
metadata for public search, the authenticated application list, the API-key
page, and the apply form without recording cookies, payloads, or key values.

| Page | Data-bearing XHR/fetch after redesign |
|---|---|
| public search | none — rows are in the document HTML |
| OpenAPI detail | metadata and the initial operation are in the document HTML; remaining legacy operations come from **`POST /tcs/dss/selectApiDetailFunction.do`**, an HTML fragment endpoint |
| application list | none — `selectCommonCodeSelectboxList.json` only supplies common select-box codes |
| API-key page | none — the key field is in the authenticated document HTML |
| apply form | none — only a web-filter `.hbs` template |

The detail fragment endpoint is now used directly. It is a better boundary than
driving the operation selector UI even though its response still requires HTML
parsing. The apply submit endpoint still exists, but its form validation and
payload assembly remain browser-owned. The redesign therefore refines the
decision: use stable read endpoints when found, parse their factual HTML, and
keep submit form-driven.

This also establishes an operating rule: after a major portal redesign, rerun
network discovery and re-check separately published catalogue/metadata APIs.
Page XHR inspection alone cannot establish that no official API exists.

## Revalidation (2026-09-01, combined API+FILE and ODCloud application)

Some generated OpenAPIs no longer have a working `/openapi.do` route. Their
`/fileData.do` page combines FILE assets and an OpenAPI tab and references a
documented `https://infuser.odcloud.kr/oas/docs?namespace={pk}/v1` Swagger
document. oddsock now falls back to that combined first-party page, fetches
only the exact allowlisted Swagger reference for the same PK, and treats the
resulting `api.odcloud.kr` operations as REST evidence. Service keys may be sent
only to `apis.data.go.kr` or `api.odcloud.kr`, always over HTTPS and never to an
arbitrary Swagger host.

The application redirect for these operations now lands on
`multiCloudApiRequestForm.do`, whose selected operation is a hidden
`publicDataDetailPk` rather than a checkbox table. The shared apply/probe script
recognizes both legacy and multi-cloud forms. A live individual-account test
applied for `서울교통공사_월별 승하차인원`, observed immediate approval, and
called the selected ODCloud operation successfully. The catalogue-list APIs
themselves were separately confirmed as institution-restricted and are not
silently described as generally applicable.

## Considered options

- **Use only the browser page's XHR as API discovery** — rejected: search,
  account, and key pages may be server-rendered while a separately published
  official catalogue API still exists. A targeted HTML fragment endpoint is
  used only where the documented interfaces omit the required contract.
- **`POST /iim/api/saveDevAcountRequest.do` directly for apply** — rejected even
  though the endpoint exists. Driving the form's own `fn_save()` makes the portal
  build and validate the payload; hand-rolling the POST means reproducing every
  field (plus session/CSRF preconditions) and silently creating malformed
  applications when a field changes.
- **Adaptive/self-healing selectors (Scrapling)** — rejected: Python (breaks the
  single-binary distribution) and *probabilistic* — relocating an element by
  similarity can silently match the wrong thing. oddsock's contract is
  surface-only: fail loudly, never fabricate.
- **Crawler framework (goscrapy)** — rejected: oddsock hits three known
  endpoints; it is not a crawler, and a framework wouldn't make selectors any
  less brittle.
- **AI browser agents (Browser Use, Stagehand, agent-browser)** — rejected: an
  LLM deciding what to click is probabilistic, and `apply` creates a real
  application on the user's government account. That path must stay
  deterministic.
- **Swapping chromedp for another driver (Rod, Playwright-Go)** — not adopted:
  every Go option is CDP underneath, so this changes the API surface, not the
  substance, at the cost of re-validating hand-verified automation.

## Consequences

- Markup drift is inevitable, so it must be *detected*, not absorbed: parsers
  degrade to empty results, and `oddsock doctor` drives each seam live and exits
  non-zero on drift (CI-friendly).
- A zero-row authenticated application parse is ambiguous, not healthy. Doctor
  reports it as skipped, while non-empty lists report the parsed count. The
  API-key selector has a value-free probe so layout drift can be detected without
  exposing the credential.
- CDP stays for the one-time human SSO login and short-lived form submission.
  oddsock extracts a reusable cookie session, closes the login browser, uses
  plain HTTP for authenticated reads, and starts an isolated headless browser only
  when `apply` must drive the portal's own form.
- Inspection responses carry evidence entries such as `official_api`,
  `standard_metadata`, and `first_party_web_contract`, with the latter labelled
  `fallback`. This makes stability a machine-visible contract rather than a
  documentation footnote.
