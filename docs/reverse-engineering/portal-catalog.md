# data.go.kr Portal Catalog

Source of truth for the portal surface odeduck depends on. It should grow before
the Go client grows: add a row here (with how you verified it) before writing code
against a path.

Verified against the live portal on **2026-07-25** with an authenticated session
(CDP network capture + plain-HTTP probes).

## Status legend

- `public` — works with no session
- `auth` — needs a logged-in session
- `precondition` — needs `auth` **plus** something else (a cookie, a prior page)
- `driven` — odeduck does not call it directly; it drives the page's own JS
- `keyed` — needs a serviceKey, not a session

## Hosts

| Host | Role | Notes |
| --- | --- | --- |
| `www.data.go.kr` | portal web app | server-rendered; scraping target |
| `auth.data.go.kr` | SSO / login wall | never automated (government SSO) |
| `apis.data.go.kr` | OpenAPI gateway | the actual data APIs; serviceKey, no session |

## Paths

| Status | Method | Path | Purpose | odeduck mapping |
| --- | --- | --- | --- | --- |
| `public` | GET | `/sso/login.do` | login entry page | `login` opens this for the human |
| `auth` | GET | `/sso/profile.do` | **SSO trampoline** — auto-submitting form, needs JS | probe loops settle past it |
| `auth` | GET | `/iim/api/selectAcountList.do` | 활용신청 현황 list | `applications`; also the auth probe |
| `auth` | GET | `/iim/api/selectApiKeyList.do` | 인증키 발급현황 (the serviceKey) | CLI `key`; `call`/MCP `call_api`가 내부에서 자동 주입하며 MCP 도구로 키 자체를 노출하지 않는다. Parse `#pblisrCrtfcKeyPlain` (hidden input = the ACTIVE key; the table also lists superseded ones). **Note the markup has a duplicate `value` attribute — take the first.** |
| `precondition` | GET | `/tcs/dss/redirectDevAcountRequestForm.do?publicDataPk={pk}&isBusinessApply=N` | 활용신청 form | `apply`; needs cookie `currentMyMenuId=M020105`, else bounces to `index.do` |
| `driven` | POST | `/iim/api/saveDevAcountRequest.do` | 활용신청 submit (AJAX) | **not called directly** — `apply` invokes the form's `fn_save()` so the page builds/validates the payload (ADR 0001) |
| `public` | GET | `/tcs/dss/selectDataSetList.do?dType=&org=&keyword=&currentPage=&perPage=` | dataset search | `search` (plain HTTP, no browser) |
| `public` | GET | `/data/{pk}/openapi.do` | OpenAPI detail (operations, request variables, 참고문서) | `describe` (plain HTTP, no browser) |
| `keyed` | GET | `https://apis.data.go.kr/{org}/{service}/{operation}` | the actual data API | `call`; `serviceKey` query param |

### Not data-bearing (checked, ignore)

`/templates/*.hbs`, `/uim/cmm/selectMberInfo.json`, `/cmm/cmm/selectCommonCodeSelectboxList.json`,
`analytics.google.com/g/collect`. These were the only XHR/fetch requests in the
read pages examined on 2026-07-25 (ADR 0001), not a claim about every portal page.
The subsequently verified standard-data download surface below is data-bearing.

## Standard-data public download contract (verified 2026-09-07)

Unauthenticated GET of `/data/15013199/standard.do` loaded the first-party
`/js/biz/mvc/std/std-download-manager.js`. Its `getHeader`, `getData`, and
`preProcessDataParam` methods establish these paths; both were then called
successfully without cookies or keys:

| Method/path | Observed contract |
| --- | --- |
| GET `/download/columList.json?pk={pk}&ext=CSV` | `fileName`, `columList[{columCode,columNm}]`, `tableVO.publicDataPk`, `tableVO.svcTableNm`, `tableVO.colNmList`, `totalCount` |
| GET `/download/standard.json?publicDataPk={pk}` | Repeat `colNmList` query keys exactly as the first-party JS's `traditional:true` serialization; use the header's `svcTableNm` and `totalCount`, plus 1-based `page` and bounded `perPage`. Response is a JSON record array. |

For PK 15013199 the header returned 22 columns, 42,226 declared rows and table
`tn_pubr_public_heat_wve_shltr_svc`. A two-row page requested SHLTR_NM,
LEGALDONG_NM and REFERENCE_DATE; the response also included INSTT_NM/INSTT_CODE
(both advertised in the header). Both records had REFERENCE_DATE 2018-05-15:
current catalogue metadata does **not** prove that individual rows are current.

These are fragile first-party web contracts, not a documented OpenAPI. The
browser generates CSV/XLS/JSON locally from returned rows; there is no need to
invent a direct CSV URL. The adapter must preserve original JSON types/nulls,
validate the returned PK and schema, keep table/column selectors inside its
trusted inspection handle, cap bytes/rows, and label results as an ordered page
with unverified population coverage. Unknown legacy catalogue delivery can be
resolved only by this verified schema contract, never by a title or HTTP 200 alone.

Implementation integrity rules: both STD requests reject redirects before following them; bounded
streaming is required in production and tests. Schema and record JSON reject duplicate decoded object
members, invalid UTF-8/unpaired surrogate escapes and nesting beyond 32 levels before values are
retained. Genuine replacement characters and exact JSON numbers remain valid source values.

On 2026-09-08 at 02:29 UTC, `odeduck inspect 15013199 --delivery standard --observe` with the new
reader succeeded without login: 22 declared/observed columns, 42,226 declared rows, and 5 observed
first-page records (3,205 bytes). Schema SHA-256 was
`c764e942a0a9d7d20d0dff491a8d2a5dbb68442fedec2ab265f94e3ee0b19608`; page SHA-256 was
`501d8473a1b517582721cd9043bfb7c962fe9210ab1e7c98814ba43b54f4b61f`. Only schema and aggregate
observation metadata were returned; this is a live contract check, not an unseeded goal-completion test.

## FILE reference-table observation (2026-09-07)

`/data/15063424/fileData.do` advertised `국토교통부_전국_법정동_20260729.csv` through the existing
`/cmm/cmm/fileDownload.do` contract (`atchFileId=FILE_000000003687312`, `fileDetailSn=1`, observed dataNm).
The downloaded CSV contained 20,561 rows and columns 법정동코드/시도명/시군구명/읍면동명/리명/순번/생성일자.
The first 1000 rows contained no 충청남도 records; the full file contained 2,275, including 199 whose
시군구명 was 공주시. This motivates bounded **local row selection before the output limit**, not a new
portal filter endpoint. The provider describes currently existing codes and explicitly warns that these
processed K-GeoP codes can differ from the administrative-standard-code source. Current membership and
creation dates alone do not validate a historical crosswalk or prove 행정동/법정동 equivalence.

## Historical FILE editions (verified 2026-09-08)

The public [file-detail script](https://www.data.go.kr/js/biz/datset/script_fileDetail.js),
`fn_histAndCsvData`, loads the portal's “주기성 과거 데이터” section. Unauthenticated
POSTs and actual old-file acquisition verified this path for PK 15097972:

| Method/path | Observed contract |
| --- | --- |
| POST `/tcs/dss/selectHistAndCsvData.do` | `publicDataPk` and the current page's `publicDataDetailPk`; `#tab-layer-file-05 h3 span` count, `a.openFileDetailPopup` with `data-public-pk` and `data-public-detail-sn` |
| POST `/tcs/dss/selectDpkDetailInfo.do` | Listed `publicDataDetailPk` and `publicDataHistSn`; `#file-detail-popup`, its own metadata and explicit `fn_fileDataDown(pk, detailPk, attachment, serial, format)` buttons |
| POST `/tcs/dss/selectFileDataDownload.do` | Existing resolver: exact listed/button IDs plus `publicDataTyCode=PR0051`; then existing `/cmm/cmm/fileDownload.do` GET |

Accept at most 8 MiB listing and 2 MiB popup responses under the fetcher's existing
buffer bound. Require a single history section and agreement between declared and
recognized counts; selector drift is unknown coverage, not zero history. Expose
the first 32 editions in portal order, total recognized count and truncation.
Select only from that prefix after fresh membership validation. Resolve 1–8
explicit attachments and reject changed PK/detail/attachment IDs, malformed IDs,
duplicate asset names and disagreement between button format and filename.
Repeated buttons for the same attachment/serial may deduplicate only when their
advertised formats agree case-insensitively; conflicting declarations are rejected.

The July popup advertised CSV and JSON. The JSON resolver returned a `.json`
filename but `atchFileExtsn=csv`; preserve the matching button/filename JSON format
and report that discrepancy. This does not validate content or enable JSON sampling.
Historical metadata comes only from the selected popup, never the latest edition.
Membership and popup POST URL/forms are retained as inspection evidence. They
contain public IDs, not keys. Registration and modification dates are not row dates.

The current page listed 54 editions; the July selection was
`uddi:95f114ef-87c9-4669-971d-aab9aff9d7d4/2`, attachment `FILE_000000003807403/1`.
Actual re-acquisition matched the original July hash; see the
[G4 diagnostic](../research/goal-result-execution-validation-2026-09-08.md#과거-버전의-실취득-복구).
No historical-date URL synthesis, latest fallback, archive guarantee or population
approval is provided. CLI/MCP behavior is defined in the
[shared inspection contract](../specs/goal-integration-and-cleanup-v1.md#과거-file-버전-선택--2026-09-08-추가-계약).

## Preconditions worth remembering

- **`currentMyMenuId=M020105` cookie** before the apply form, or the portal
  redirects to `index.do`. Set it explicitly.
- **SSO trampoline** (`/sso/profile.do`) is an auto-submitting form, so the first
  authenticated navigation in a fresh tab must run in a browser, not plain HTTP.
  Once the session has settled, the cookies work over plain HTTP.
- **Auth cookies are session-scoped**: Chrome drops them on exit, but the values
  stay valid server-side. odeduck copies them out at login (`internal/portal/session.go`)
  and closes the window.

## Capture workflow

To add or re-verify a row:

1. Log in with `odeduck login` (this leaves a session; add `--keep-browser` if
   you need the window).
2. Attach over CDP on the debug port and enable the Network domain, then load the
   page and record only `XHR`/`Fetch` requests — that separates data calls from
   template/telemetry noise.
3. For a form action, read the handler's source (`fn_save.toString()`) instead of
   submitting, so you learn the endpoint without mutating the account.
4. Record the path, its status, the preconditions, and the date verified.

## Gateway propagation

A freshly approved API answers **403 Forbidden** at `apis.data.go.kr` for a while:
the portal auto-approves instantly, but the gateway takes minutes (up to ~1 hour)
to accept the key for that service. Verified 2026-07-25 — an API applied for
minutes earlier returned 403 while one applied ~2 hours earlier returned 200 with
the same key. `call` surfaces this as a hint so it is not mistaken for a key error.

## Specs that live only in the guide document

Some datasets document nothing on `openapi.do` — no endpoint, no 요청변수 table —
and ship the whole spec in the attached 참고문서 (hwp/xlsx/zip). Example: pk
15012005 (소상공인시장진흥공단 상가(상권)정보). `describe` surfaces this as an empty
`operations` plus a `note` and a **`guideDocUrl`** built from the page's
`fn_fileDownload('FILE_...','N')` handler:

    {baseURL}/cmm/cmm/fileDownload.do?atchFileId=FILE_...&fileDetailSn=N

Verified 2026-07-25: that URL returns the guide (HTTP 200, 8 MB zip). odeduck
never parses the document — reading it is the agent's job, by design (spec §9).
For 15012005 the zip also carried a 30 MB `주요상권현황` CSV, i.e. guide documents
sometimes contain bulk data the API itself does not expose.

## Known gaps

- **심의(manual-review) applications** are not handled — auto-approved APIs only.
  `list_applications` shows their status; approval and re-call are up to the user.
