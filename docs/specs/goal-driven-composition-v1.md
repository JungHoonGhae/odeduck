# Goal-driven composition v1

Status: experimental implementation on the development branch; not a released/general goal-solving guarantee.
Decision: [ADR-0006](../adr/0006-goal-driven-composition.md).
Evaluation: [2026-09-07 execution audit](../research/goal-composition-evaluation-2026-09-07.md).

## User contract

Natural-language goal → role/crosswalk searches → alternative Data Nodes → inspection and bounded
samples → Composition → actual bounded output. A failed join produces a Discovery Gap and the next
planner decision may search a replacement or intermediate mapping. No initial Anchor is permanent.
MCP host and standalone CLI share the same engine; the server never runs a nested model for MCP.

## Questions and acceptable errors

| Question | Start and path | Scope and false-positive gate |
| --- | --- | --- |
| Which distinct observations can answer my goal? | goal → roles → candidates | candidate only; no inferred factual edge |
| Can these observations join? | source → rows → exact/composite key | preserve tuples and leading zeroes; null never matches null |
| Which mapping fills a missing link? | failed Composition → crosswalk search → new source | no guessed codes or fabricated PKs |
| Does a three-source combination actually yield output? | bounded join sequence → aggregation | execute whole combination, not pairwise overlap claims |
| Can I reproduce the output and its limits? | output → source observation hashes and join metrics | sample label, observation times, recipe, declared assumptions |

Concrete temporal questions for the common module (prospective product checks, not claimed live successes):

| User question | Starting record and permitted path | Time and false-positive gate |
| --- | --- | --- |
| 2025년 고령인구와 같은 기준 기간의 쉼터를 비교할 수 있는가? | population period → region key → shelter reference period | explicit 2025 window; a shelter row dated 2018 cannot pass |
| 행정구역 코드가 바뀌기 전후 자료를 어느 대응표로 연결해야 하는가? | source code → crosswalk validity → target observation | common validity across all three records; pairwise-only overlap is insufficient |
| 특정 폭염 발생일에 운영 기간이 겹친 시설은 무엇인가? | event date → area key → facility operating interval | event day must intersect both inclusive operating endpoints; missing endpoints cannot imply open validity |
| 월별 수요와 연간 공급 자료를 함께 볼 때 어떤 한계가 있는가? | month observation → region key → annual reference period | report calendar-granularity overlap only, not the exact day or intra-year availability |
| 기준일이 없는 원천과 최신 수정일만 있는 원천을 현재 상태로 비교해도 되는가? | source record → missing record date / catalogue timestamp | no substitution of observedAt or catalogue modifiedAt; report unverified time or request another source |

These are bounded record joins plus evidence reports; a graph database is not needed to execute them.
No risk alert or eligibility action may consume calendar overlap as verified identity/validity.

Row-bound scope questions, anchored by the observed Kimcheon/Yeosu collision:

| Question | Record path and scope | Time and false-positive gate |
| --- | --- | --- |
| 김천시 남면 인구에 여수시 남면 쉼터가 붙는가? | population province+city → legal/admin mapping → shelter recorded address | 2026 population / 2019 shelter do not establish aligned time; conflicting address tokens exclude the candidate |
| 제공기관 소재지가 다른 전국 대응표를 사용할 수 있는가? | publisher declaration → selected mapping rows → actual record scope | publisher city is not record location; current mapping is not historical validity proof |
| 분리된 시도·시군구와 전체 주소를 어떻게 대조하는가? | observed field parts → complete token sequence → recorded address prefix | literal whole-token comparison only; no aliases, inferred codes or real-world identity approval |
| 주소가 없는 동명 시설은 일치하는가? | equal local name → missing/null/blank scope part | unknown excludes the pair; no match or absence proof, no catalogue timestamp substitution |
| 대응표 뒤의 범위·기간 충돌이 집계에서 사라지는가? | full left path → next record's scope → time → expansion → aggregate | every configured scope check precedes expansion and output projection; all time bindings still apply |

## Evidence model

- Source: exact data.go.kr PK and inspected delivery/operation/asset.
- Record: bounded row observation with content hash, selectors and observedAt; raw rows stay in memory.
- Entity: no canonical entity merge. Equal keys alone cannot establish real-world identity.
- Claim: proposed Composition and computed execution metrics; no automatic connection-ledger promotion.

| Match class | Current support |
| --- | --- |
| exact | requires a scoped, evidenced identifier; no automatic certification path implemented |
| deterministic | requires grounded multi-field semantics and normalization; text/tuple calculations alone do not grant this identity grade |
| probabilistic | no calibrated entity-resolution model or confidence claim implemented |
| unresolved | candidate rows may be computed for inspection; not an accepted identity edge or completed goal |

`join.normalization=exact` names a value-comparison algorithm, **not** the graph's exact identity match
class. Current compositions keep identity unresolved even when all literal comparisons succeed.

Observations are immutable per source request within a session. Changed operation, request or asset must
produce a new observation. No permanent exclusion follows from a failed sample: the failure is scoped to
its observation and recipe. Existing ledger corrections and valid/observed-time semantics remain intact.
These are observation records, not canonical identity assertions: identifier issuer, reuse rules and effective
time semantics are not machine-verified in this version. Date-value alignment can be computed with the
explicit temporal contract below; assumptions must name remaining unknowns. No high-cost action
(risk alert, eligibility decision, sales stop) may treat sample_executed as a verified entity match.

## Engine and bounds

Start captures the goal and search policy. Advance accepts one typed action: define, search, inspect, layout,
sample, retry_sample, read_evidence, compose, execute or abstain. It owns revision checks, candidate membership, duplicate action suppression,
budget charging, acquisition, execution and feedback. State and completed output are module-produced;
planner JSON cannot submit rows, receipts, computed metrics or a success status.

The engine-owned [planning guide](../../internal/goalwork/planning-guide.md) is the single detailed action
instruction source for CLI prompts and the MCP resource. CLI adds its tool-free JSON-response framing;
MCP adds session/revision/decision wrapping and its host-output boundary. Changes to instructions belong
there, not in copied operator descriptions. See [integration contract](goal-integration-and-cleanup-v1.md).

`executions` retains each known composition attempt's computed metrics, status and bounded failure detail,
including temporal rejections from an empty join. These records reach PlanningView without raw rows so
replanning can distinguish a time mismatch from unsupported fields or missing keys. They are session-local,
bounded by the composition budget, and are not durable global negative evidence.

Standalone planning permits one format-repair invocation per provider when a response cannot be decoded.
It reuses the same state and existing overall three-minute planning deadline, sends only a bounded decoder
diagnostic, and never reflects the invalid response body or its values. Provider invocation errors do not
trigger format repair. A second invalid response fails honestly; no unsupported operator is silently accepted.

Separately, standalone Run permits at most one correction after the engine rejects an already attempted
action. It sends transient `plannerFeedback` (a caller diagnostic, not source evidence) and asks for a
different action. The replay itself does not reacquire, advance revision or change source state. A second
replay terminates, even if another action occurred in between. Context/deadline and all acquisition limits
remain enforced. MCP hosts receive the same engine replay error directly; no nested MCP planner is added.

Search may use role or crosswalk phrases without guessing a common key first. Inspection uses existing
delivery adapters; calls use DatasetCaller so credential/HTTPS/typed operation gates remain enforced.
DatasetCaller now resolves an omitted portal base to the existing official BaseURL, as the inspector already
does. Explicit test/custom bases remain explicit. This is only default configuration for portal description,
not authority to guess provider endpoints or bypass operation/credential validation.
The engine never applies for access automatically. Access failures become gaps with existing recovery actions.

Independent caps cover rounds, distinct candidates, searches, inspections, samples, rows, bytes and
alternative Compositions. Duplicate failures cannot burn an unbounded retry loop. Session expiry discards
in-memory raw material. CLI planner receives bounded metadata, columns and metrics by default. The
[selected-evidence contract](selected-goal-evidence-v1.md) permits bounded selected values only under a
trusted start policy naming one recipient; full sample/artifact rows remain excluded from CLI planning.

`observations[].columnProfiles` provides computed `shape_v1` counts, not examples or value hashes.
Each field reports missing keys, explicit nulls, string values, blank strings, strings changed by Unicode
outer whitespace trimming, and min/max whitespace-separated token counts over nonblank strings.
`decimal`/`groupedDecimal` count string forms that would lexically match those formats with trim=true
and the existing 256-byte input cap; grouped includes plain decimal. Numeric-looking identifiers are not
automatically converted. Differences in token counts can motivate an official mapping search but do not
establish geographic hierarchy, identifier meaning, population coverage or any value correspondence.

Concrete limits: default 32 / maximum 64 rounds, 12 searches, 96 candidates, 12 inspections, 8 sample
attempts, 6 Compositions, 1000 rows / 2 MiB per observation, 8 MiB total observation JSON, 1000 rows /
2 MiB artifact. Each search admits at most 8 hits; later searches may change roles and queries. Search
query/role limits are 500/200 UTF-8 bytes. Remaining acquisition budgets are included in every view.
MCP retains at most 8 goal sessions, bound to their originating MCP connection and expiring after one hour.

Candidate capacity is the product of 12 searches and 8 admitted hits per search. The previous independent
48-candidate ceiling could discard the seventh valid counterpart search; the change preserves all permitted
search results without increasing model rounds, search calls, inspections or sample/byte ceilings.

`inspection.declarations` retains bounded publisher context by request delivery; `observation.declaration`
copies the selected delivery's context into planning and the artifact. Status is always publisher_declared,
not verified identity/scope. FILE carries its description, provider, declared spatial/temporal coverage,
catalogue modification date and whitelisted limitations/notices. Missing values remain unknown; conflicting
HTML coverage text remains a separate notice. Structured Schema.org coverage is not guessed into a label.
API/STD carry only the names/source context actually exposed by their adapters. Truncation is explicit.
Retrieved text is untrusted data, not instructions or permission to change the original goal or policy.
CLI is in-memory only; it does not resume a prior process. Ordinary repeated sample actions remain
suppressed. An eligible failed acquisition can use the explicit same-session retry contract below.

API sampling selects a single flat record array (explicit JSON Pointer when ambiguous), using the existing
authenticated caller. Goal inspection `operations[].name` is the identifier accepted by DatasetCaller:
the existing OperationName endpoint segment for REST/ODCloud, or the typed registry name for LINK.
`title` separately preserves bounded human-readable meaning; titles are not invocation aliases, and an
operation without an invocation identifier is not advertised as callable. Endpoint/key material stays private.

`sampleAttempts` retains the original validated acquisition request, SHA256, start time, revision and
failed/acquired outcome (with observationId on acquisition). Match a failed attempt's revision to its gap.
It includes prior API parameters and CSV where conditions so planning can distinguish delivery, failed
operation and selected scope; it never includes returned rows or credentials added inside a caller.
Parameter/selection maps are copied separately for history and the acquisition adapter. Invalid credential
inputs, invalid selection, exhausted-budget actions and rejected replays do not enter acquisition history.
History is computed by the engine, not accepted as a planner-supplied outcome. It expires with the session
and is not a durable or cross-session claim ledger.

### Explicit acquisition recovery

`retry_sample` accepts `retryOf`, the revision of the latest failed acquisition of an identical request,
and optional `reason` (at most 1000 UTF-8 bytes). It accepts no replacement sample, PK, operation or
other action fields. The engine retrieves the validated request from its private attempt history, runs
the same inspection/caller path and appends a new attempt linked by `retryOf`. Old failures and gaps
remain unchanged. Successful observations cannot be retried through this recovery action.

The adapter supplies typed `AcquisitionError`; the planner cannot submit failure classifications.
Attempt `failureKind` is `access_required`, `transient`, `cancelled` or `unclassified`. The first three
are eligible; invalid returned rows and other unclassified failures are not. The live adapter recognises
existing login/provider-key/key-rejection/propagation sentinels, HTTP 401, context cancellation/deadline
and typed transport timeouts. It never classifies a provider body's prose as a recovery instruction.
Typed error messages omit private causes while preserving error identity for internal handling.
Unclassified errors retain the existing diagnostic path; this is not a general-purpose error redactor.

There are at most **three acquisitions per identical request**, including the original, within the
existing eight-attempt total, round budget, expiry and byte limits. The referenced attempt must be the
latest in its request chain. Invalid, superseded, successful, stale, cancelled-context and exhausted
retry requests cause no acquisition and do not advance the revision. A failed retry appends a new failure;
it does not reset its chain allowance. Ordinary `sample` replay remains forbidden.

`reason` is a planner rationale, not evidence of changed external access. Access recovery must use the
existing human login / approved credential workflow outside this action. The retry never applies,
logs in, changes keys, weakens required semantics or claims completion. It only tries the same read with
the caller's current prerequisites. Transport cancellation can be retried through a live context while
the engine still exists. HTTP 429/503 remain unclassified because this seam lacks `Retry-After`; automated
waiting, rate-limit recovery, process restart and cross-session restoration remain incomplete.

CLI's planning loop and MCP's `advance_goal` use this same transition. MCP still requires the originating
connection. Tests exercise direct engine recovery, stale/success/unknown/cancelled failures, request-chain
limits, live adapter classification and both caller surfaces. Fixture recovery is not live authentication
verification, a durable checkpoint or a passed goal-completion benchmark.

FILE execution supports direct CSV with unique first-row headers,
UTF-8/EUC-KR decoding and a default 8 MiB download cap (explicit streaming mode below). The first 1000 matching CSV rows are an explicit ordered prefix,
not a representative sample. Direct XLSX supports the exact rectangle contract below; ZIP supports an
exact CSV member. DBF remains schema-inspectable only; nested archive sampling is unsupported.

For direct CSV, optional `sample.where` is a map of at most 8 exact string predicates, combined with AND
before the row limit. Fields and values must be nonblank and at most 256 UTF-8 bytes. Header fields must
actually exist; there is no trim, substring/prefix matching, code coercion, null matching or guessed HTTP
query parameter. Every scanned record must be structurally valid, even if excluded by the selection.
Use goal/source-grounded values without redefining the original goal's geographic or population scope.
Different selections produce distinct request hashes and observations; the content hash still describes
the full downloaded bytes, whereas the rows hash describes the returned selected records.

`observation.selection` records mode=`exact_strings_v1`, scannedRows, matchedRows, returnedRows and
exhausted. A truncated scan stops after one extra matching row; matchedRows is not the asset-wide total.
An exhausted scan covers this particular file only, not dataset/population completeness or current validity.
No matching row is an explicit gap, not an absence proof. API/STD reject where instead of ignoring it;
FILE rejects API params/operation/rowPath. The shared acquisition module enforces this for CLI and MCP.

### Full direct CSV scan

`sample.scanCsv:true` selects a streaming scan of an inspected direct comma-delimited CSV, up to 64 MiB
of source bytes and 2 million data records. It accepts the same exact `where` predicates, but rejects
API/STD, ZIP members and XLSX selectors. Existing small-file readers keep their 8 MiB limits. The first
physical header line selects UTF-8 or strict EUC-KR/CP949; each legacy code unit must round-trip exactly.
An ASCII header followed by legacy non-UTF-8 values fails: there is no midstream codec switch. Encoding
is a decoding convention, not publisher certification. Standard CSV newline conventions still apply.

The reader checks all records, including excluded rows and malformed tails, while hashing original
transport bytes. Successful EOF alone supplies the full hash and `exhausted:true`. Limits are 256 columns,
64 KiB per decoded field, 1 MiB decoded record and a 2 MiB parser-input window per record (plus bounded
read-ahead). It neither buffers nor archives the complete source. Trusted local consumers can visit every
matching record; any failure invalidates their interim state. The nearest-record consumer below is
explicit; whole-source aggregation remains separate work, not an implied capability of sample retention.

Sampling retains only the first 1000 matching rows within 2 MiB. Selection mode
`exact_strings_full_scan_v1` reports whole-file `scannedRows` and `matchedRows`, separate from `returnedRows`.
`prefix` remains true when matches exceed retained rows, even though `exhausted` is true. Complete scanning
is not complete retention, population coverage, identity approval or source-wide nearest-neighbor analysis.
No matches yields a gap, not absence proof. The request flag, raw source hash, decoding and original record/
line positions survive the common CLI/MCP execution path. External CLI planning excludes full rows;
explicitly authorized selected evidence follows the separate bounded disclosure contract.

#### Exact value sets — 2026-09-08

For this full direct CSV scan, `sample.whereIn` maps observed fields to 1–32 distinct nonblank
string values each. Values within a field use OR; fields and existing `where` predicates use AND.
The two selectors share the 8-field limit and cannot name the same field. Each field/value stays
within 256 bytes. Matching preserves leading zeros, case, spaces and original values; no ranges,
substring matching, coercion, empty/null matching or provider query expansion is implied.

The Engine sorts each detached set before request/replay hashing, so order alone cannot buy another
acquisition. Requests, retries, artifacts and reviewer inputs preserve these canonical selectors.
Selection fields must have explicitly disclosed semantic evidence before either review kind proceeds.
The full scanner reports `exact_string_sets_full_scan_v1` when a value set is used, retaining the same
separate scanned/matched/returned counts, original record positions, source hash and malformed-tail gates.

`whereIn` requires `scanCsv:true`; bounded direct reads, ZIP/XLSX, API/STD and source reduction requests
reject it instead of silently ignoring it. The existing complete-stream consumer can use it too.
This scope follows the observed multi-region/cohort acquisition need, not a new general expression language.
Fixed environmental and population calibration goals/oracles stay unchanged; actual acquisition recipes
are separate data in `testdata/goalbench-v1/analysis-acquisition.json`. The development command compares
real acquisition hashes, retained coverage, record positions and original values before model review.
File completeness is still not population certification or proof that its fields answer the original goal.

### Conditional nearest source records

For G2's frozen distance question, `sample.scanCsv:true` accepts `nearest` with method
`spherical_nearest_records_v1`, `anchor` and `candidate` original observation IDs, `anchorLatitude`,
`anchorLongitude`, candidate `latitude`/`longitude` observed field names, and `k` from 1 to 10. All points
come from retained source values or the trusted scan; the caller supplies no coordinate values. Candidate
must be an already observed direct CSV of the same PK/asset. Its source and inspected-contract hashes pin
the new scan. A source change fails the whole reduction; acquiring a new candidate revision needs a new
permitted request/session, not rewriting the earlier observation. Direct CSV refresh remains a separate
recovery gap. This selection rejects layoutId, ZIP/XLSX, API/STD and earlier spatial outputs.

The shared goal engine interprets explicitly named fields as ungrouped decimal degrees (no whitespace,
exponents, grouping, axis swap or reprojection). Missing, malformed and out-of-range coordinates in ANY
retained anchor or matching candidate fail. Structurally malformed excluded CSV rows also fail through
the full reader. The fixed sphere radius is 6,371,008.8 m; haversine distances are clamped against floating
point endpoint drift. Distance then original candidate data-record order breaks ties. This is a conditional
calculation convention matching the independent oracle, not a discovered CRS. Spherical versus spheroidal
distance is a real distinction in the [PostGIS distance contract](https://www.postgis.net/docs/manual-3.5/en/ST_DistanceSphere.html);
this implementation adds no spatial database or inferred SRID.

Every exact-filter matching candidate is compared against every retained anchor. Budgets are 1–100 anchor
rows, 10 million comparisons, at most 1000 retained pairs, 2 MiB of retained candidate values and 2 MiB/256
columns of final paired rows. Existing scan/session/sample/retry limits still apply. Any failure discards
all interim results, including candidates found before a bad tail. Fewer than k matching records yields
only those records; scan counts and request k preserve that shortfall. There is no padding or absence proof.

Rows contain `anchor.<original field>`, `candidate.<original field>`, numeric `distance_m` and `rank`.
These are derived pair rows, not original CSV records. `observation.spatial` retains both input observation
references, anchor row/content/request hashes, candidate request hash, full candidate scan report, fixed
method/radius and comparisons. Its pairs identify `anchorRow` (1-based in the retained anchor observation),
`candidateDataRecord` (1-based AFTER the CSV header), physical `candidateStartLine` and rank. Follow the
anchor observation's CSV/table/archive provenance to its original source position. Artifacts include both
input observations/requests even when only the reduced observation participates directly in a join.
Distances and copied row values remain private by default; explicitly authorized selected evidence may
disclose them with original addresses. References/positions/counts are planning metadata. Earlier input
observations remain immutable, and subsequent evidence corrections do not overwrite them. Copied CSV
empty strings stay empty strings rather than fabricated nulls in both evidence packets and artifacts.

A spatial base may be projected or aggregated without another join; it already connects two original
sources. Use grounded keys when joining accessibility or other observations. Field-level source addresses
preserve original-role attribution, prevent conflicting back joins and extend the duplicate-sum guard to
copied values. Read [source-record lineage](source-record-lineage-v1.md) when changing joins, measures,
sums, role attribution or time on these derived observations. Count counts pairs, not distinct destinations.
All spatial meaning remains `meaningVerified:false`; datum, coordinate accuracy,
snapshot compatibility, anchor coverage, physical-stop deduplication, route and current operation need
their own evidence/acceptance policy. This feature does not clear `review_required`, complete G2 or pass M3.

### Exact ZIP member acquisition

For ZIP assets, the existing `layout` action without a sheet returns `format:ZIP` and exact decoded member
paths, filename decoding convention/raw-name hash, declared sizes and extensions. This is directory
metadata, not decompression/checksum/schema success. The whole archive is bounded to 8 MiB downloaded,
128 entries and 64 MiB declared expansion. UTF-8 names are preserved; legacy names use an explicit EUC-KR
fallback with byte-for-byte round-trip verification. Decoding is a convention, not publisher certification.
Duplicate decoded names, unsafe paths, encrypted/link/special entries fail. Names stay source-local:
no basename fallback, path normalization, extraction to disk, recursive archives or concatenation.

`sample.member` selects one exact CSV path. It supports the same `where` and row cap as direct CSV,
rejects API/STD and XLSX selectors, and may pin `layoutId` to the same archive revision. A selected member
has its own 8 MiB expanded limit and is fully read/checksummed before CSV prefix selection. Other members'
contents are not verified. The common byte parser preserves source values under CSV decoding conventions;
UTF-8/EUC-KR conversion must be lossless. Standard CSV newline handling applies, including CRLF-to-LF
normalization inside quoted fields. Unsupported encoding fails instead of inserting replacement text.

`observation.archive` retains member path, nameEncoding, rawNameSha256, memberSha256 and memberBytes.
The observation's contentSha256 still identifies the whole ZIP. `observation.csv`, for both direct and
archived CSV, records decoding convention, dataRecords (1-based logical data records AFTER the header)
and startLines (physical CSV lines). Quoted multiline fields and ignored blank lines mean these positions
need not coincide. Compare external ordinal conventions explicitly: G2's frozen `csvRecord` includes the
header as record 1, so it equals dataRecords + 1. The oracle's values/positions are not rewritten.
These metadata, selectors and both content hashes survive shared CLI/MCP execution to artifacts, while
planning excludes full rows except explicitly authorized selected evidence. Selection coverage concerns the chosen member only, never the ZIP,
dataset or population as a whole. Direct transit CSV full scanning and conditional nearest-record reduction
are supported above; general spatial operations, interpretation and source-wide aggregates remain M3 gaps.

The reader uses Go's [ZIP interface](https://pkg.go.dev/archive/zip) and
[CSV record/line positions](https://pkg.go.dev/encoding/csv#Reader.FieldPos); it executes neither archive
members nor caller-supplied code.

For direct XLSX, `sample.xlsx:{sheet,range}` selects an exact worksheet name and uppercase A1 rectangle
(single cells allowed), at most 1000 rows by 256 columns within Excel's coordinate bounds. It requires an
asset from the trusted FILE inspection; it accepts neither CSV `where` nor API selectors. Worksheet names
resolve through the workbook's internal relationship, never an ordinal fallback or an external target.
Download cap is 8 MiB; archive caps are 128 entries and 64 MiB expanded. Ambiguous/path-traversing members,
duplicate sheet/relationship identities and invalid row/cell coordinates fail. The full selected worksheet
XML is scanned through EOF; only selected cell values are retained. Rows and cells must have explicit,
ordered coordinates, and duplicated values/formula elements fail rather than being overwritten. This is
a bounded supported subset, not a complete OOXML validator or Excel calculation engine.

Returned keys are original column letters. All requested row positions, including entirely blank rows,
are retained. Absent/blank cells are null; explicit strings retain whitespace and leading zeroes. Rich-text
runs concatenate base text, excluding phonetic annotations. Numeric storage lexemes remain strings;
boolean `0`/`1` become booleans. Number formats, date serials, escaped string conventions and merged headers
are not interpreted. Formula code is not evaluated: stored caches are observations, not fresh calculations;
missing caches become null. Error cells and unsupported selected storage types fail. Selected raw/resolved
text each has a 2 MiB cap; the engine additionally applies its serialized row and session byte budgets.

`observation.table` retains sheet, member, range, original rowNumbers, formulaCells (at most 4096) and
formulasWithoutCache. Source hashes identify the whole downloaded workbook; row hashes identify returned
cells. Request hashes and retry history include the exact XLSX selection. Artifacts retain this provenance;
planning receives only explicitly authorized selected evidence, never the full rectangle. Reading an entire rectangle does not certify population coverage,
column meaning, formula-cache freshness or common geography/time. Physical layout discovery below helps
choose positions; header meaning and relevant table selection still require separate evidence.

`layout` accepts `layout:{pk,asset,sheet?,refreshOf?}` after an existing FILE inspection. Without sheet it
lists exact workbook worksheet names and internal members; with a discovered exact sheet it scans the
whole worksheet XML and returns cell-derived extent, stored row/cell counts, the first 128 row shapes
(row number, first/last column, stored cell count) and 128 merge ranges. Explicit truncation flags distinguish
retained metadata from total scan counts. The optional workbook dimension is not trusted. Stored cells
may be blank or style-only; neither extent nor a merge is a semantic table/header or population boundary.
No cell text, formula code, numeric observations or inferred column meanings enter this result.

The shared engine retains `state.layouts` with request, ID, request/content hashes and observation time.
Reads have six attempts per session, count toward the existing round limit, and retain at most 32 KiB of
metadata per successful read. Source download/ZIP limits are shared with XLSX sampling. Same-request
replays are rejected; `refreshOf` explicitly links a reread to an earlier layout for the same PK/asset/sheet,
without resetting any budget or overwriting prior evidence. Failures remain acquisition gaps.

`sample.layoutId` optionally pins XLSX sampling to that discovered workbook revision. The engine verifies
PK, asset and sheet before acquisition, then rejects changed content hashes before retaining rows.
After drift, discover a new layout and reconsider the selection; a new ID denotes a new request, not an
unchanged retry or semantic approval. Manual, separately grounded selections remain supported without
layoutId. Artifacts retain the layout records referenced by their source requests (and earlier session
layout history). CLI planning and MCP use the same contract and budget. Cell-text interpretation remains
unavailable from structure alone; authorized selected evidence may expose observed text but does not
certify its interpretation. Structure discovery does not complete G4 or M3.

The value/cache distinction follows Microsoft's [formula documentation](https://learn.microsoft.com/en-us/office/open-xml/spreadsheet/working-with-formulas);
phonetic annotations are separate from base text under [PhoneticRun](https://learn.microsoft.com/en-us/dotnet/api/documentformat.openxml.spreadsheet.phoneticrun?view=openxml-3.0.1).

STD sampling uses the portal's observed first-party header/JSON-page contract, not an invented CSV/API
endpoint. The official monthly importer preserves STD; old unknown-delivery entries can be resolved only
by a returned exact PK plus validated table/column schema. Selectors are bound to a private inspection
handle. The planner supplies only PK and delivery=standard. The first page is capped at 1000 rows/2 MiB,
preserving JSON numbers, strings, nulls and record dates. Declared totalCount does not prove coverage.
Header and row JSON must be valid UTF-8 with paired Unicode surrogate escapes. Malformed text fails
before decoding instead of collapsing different identifiers to U+FFFD; genuine U+FFFD remains valid.
See [observed portal contract](../reverse-engineering/portal-catalog.md#standard-data-public-download-contract-verified-2026-09-07).

## Goal acceptance contract

### Row-bound scope comparison

Optional `joins[].scopes` adds up to 4 conditions to a key-equal candidate pair. Each condition has
`leftParts` (1–4 distinct qualified fields already in the accumulated left relation), `rightParts` (1–4
distinct original fields of the new right observation), and one explicit rule. Source text is split on
Unicode whitespace and concatenated in field order. Every part must be a nonblank observed string;
missing/null/blank is unknown. Non-string values, fields absent from the observation, more than 2048 UTF-8
bytes per field or more than 64 tokens per side fail the composition rather than silently converting data.

- `equal_tokens_v1`: identical complete ordered token sequences.
- `left_prefix_v1`: the complete left sequence is the beginning of the right sequence.
- `right_prefix_v1`: the complete right sequence is the beginning of the left sequence.

Prefixes are whole tokens, not character prefixes or substrings. By default no case folding, Unicode
normalization, abbreviation/administrative alias mapping, inferred address parsing, code extraction or
geocoding is done. An explicitly selected published label vocabulary is the exception described below.
Row parts compare actual fields, not a supplied constant or arbitrary publisher metadata. A separate
citation path below can retain a proposed source-scope interpretation. For example, explicitly bound
province and municipality parts can be compared to the start of a recorded address, but the role of that
address and the meaning of its scope remain proposed interpretations.

All scope conditions must pass before temporal checks, bounded expansion, measures, aggregation and
projection. A projected-away scope column cannot bypass the check. `metrics[].scopeChecks` retains the
binding, candidatePairs/matchedPairs/conflictPairs/unknownPairs and meaningVerified=false. Each condition
counts lineage-compatible key-equal candidate pairs; earlier original-record conflicts are counted
separately in `lineageRejectedPairs`. Counts across scope conditions may overlap. Temporal rejection counts
cover only pairs which passed every scope check. The counts are not unique records, population estimates,
or a judgment that differently spelled places cannot be the same place.

An empty scope-filtered join keeps the goal exploring with its execution metrics and Gap, allowing a new
source or composition. Scope metrics are value-free; actual values require authorized selected evidence.
An omitted scope stays unchecked, and even
a matched scope does not clear review_required or promote a ledger claim. This operator does not itself
discover missing scope bindings or certify names; those acceptance capabilities remain incomplete.

### Versioned scope vocabulary

A row-bound scope may specify `vocabulary:"kr_sido_labels_20260907_v1"`. This is a frozen correspondence
between 17 published Korean province full/short labels, applied to the first whitespace token on **both**
scope sides. Remaining tokens still obey the selected exact/prefix rule. Both sides must use original row
parts, not source citations. The chosen fields must be interpreted as province names or province-first
addresses; that interpretation remains a separate semantic review item.

The [versioned vocabulary](../../internal/goalwork/vocabularies/kr-sido-labels-20260907.json) records original
publication URLs, byte hashes, extraction locations and limitations. Its two sources belong to the same
SGIS code namespace: the official full-name code table and the `selectSidoNameChk` short-label table used
by the official map interface. Commented-out historical names and the national `00` entry are excluded.
SGIS codes link these two reference tables only; they are never interpreted as codes from a data.go.kr
record. Three inspected public code files use 28, 4 and 23 for Incheon, demonstrating why numeric equality
is not enough for a cross-source identifier mapping.

This vocabulary is bundled reference evidence, **not** a new SGIS catalogue, API/provider adapter or
runtime network call. Dataset discovery/acquisition remains data.go.kr-scoped. A changed publication
requires a new vocabulary revision and tests, not silently replacing the meaning of an existing recipe.

Unknown versions fail the composition. A nonempty first token absent from the selected vocabulary becomes
unknown and rejects the pair; it is not a proven geographic conflict. For example, this revision compares
`인천` with `인천광역시`, but does not silently modernize `강원도`, which is absent from the non-commented
full-name table. It does not strip arbitrary suffixes, normalize facility names, rename districts, extract
codes or certify geographical boundaries/historical continuity. Even equal numeric strings cannot select
an entry. Literal rules remain unchanged when no vocabulary is selected.

`scopeChecks[].vocabulary` carries the exact vocabulary ID, its JSON byte SHA-256, publication observation
date, original source hashes/selectors and limitations. The join binding identifies the selected source
fields; source observations and row lineage retain their own revisions. Original strings, identity keys,
output values and prior failed recipes are untouched. A proposed/caller-supplied mapping or verification
receipt is not accepted. Counts still concern key-equal pairs, not distinct entities. `meaningVerified`
remains false, and selecting a vocabulary does not clear semantic review, satisfy population coverage or
promote a connection-ledger claim.

The G2 source replay first reproduces nine literal-token rejections, then matches nine candidate pairs
using this explicit vocabulary. The three facility accessibility values match the independently frozen
source reference. The original population contract and all unresolved semantics remain partial.

### Cited source-scope hypotheses

When a parent field is absent, a scope side may use `leftCitation` **instead of** `leftParts`, or
`rightCitation` instead of `rightParts`. A citation has `observation`, `field` (`description` or
`spatialCoverage`) and a verbatim nonblank `quote` of at most 256 UTF-8 bytes / 64 whitespace tokens.
The field is read only from that sampled observation's retained publisher_declared declaration. The left
source must already participate in the accumulated relation; the right source must be exactly the join's
new right observation. Missing declarations, wrong-side/unused sources, unsupported fields, nonexistent
quotations and simultaneous row parts + citation fail before candidate comparison.

The quote supplies text only to this hypothesis's scope predicate. It is not inserted as a record column,
join identifier, temporal binding or output value. Provider identity, dataset titles, metadata dates and
arbitrary constants/URLs are not accepted as scope citations. Prefer actual row-bound fields when present.

The executor emits `scopeChecks[].leftClaim/rightClaim` as Cited Scope Claims, always status=proposed_scope.
Each retains the citation, source PK/landing-page URL, observedAt, observation/declaration/evidence/claim
SHA256 hashes, firstMatchByte, non-overlapping occurrence count and declarationTruncated. These hashes
use the module's JSON digest: the observation and declaration snapshots, the retained field string, and
the claim with claimSha256 absent, respectively. They are not hashes of archived provider HTML. SourceURL
identifies the dataset's retained source URL; the exact quote was located in observation.declaration, not
independently re-fetched from that landing page. Full retained declaration/notices remain in artifact.sources.
A changed declaration or observation produces a different claim hash without replacing an earlier receipt.

The five acceptance questions for this hypothesis path are:

| Question | Evidence path | Gate |
| --- | --- | --- |
| 행에 상위 지역이 없는 연수구 자료를 어떻게 대조하는가? | source description quotation → address tokens | proposed scope only; no invented row column |
| 기관 이름에 나온 지역을 데이터 범위로 쓸 수 있는가? | provider/title versus explicit scope/description field | provider/title citations rejected |
| 부정 문장·혼합 coverage에서 이름만 인용하면 검증되는가? | exact quote plus retained full declaration/notices | quote location never clears semantic review; full interpretation remains unresolved |
| 다른 원천이나 다음 경로의 범위를 빌릴 수 있는가? | cited observation → actual join side | wrong-side/unused-source citation rejected |
| 원천 정정 후에도 이전 가설을 재현할 수 있는가? | observation/declaration hashes → quote receipt | new receipt for changed evidence; observedAt is not real-world valid time; no durable archive claim |

Matching a quotation does not prove it describes all records, is affirmative, remains temporally valid,
or has the right geographic granularity. Negated or mixed-coverage text can contain a literal region name;
such a mechanically located quote still cannot become verified identity, a negative absence claim, ledger
sample_verified or goal success. Current tests explicitly preserve this distinction. Claims are bounded
inside the existing composition/scope budgets and retained in session execution evidence, not a new global
graph store. Automatic interpretation/approval and cross-session ledger reuse remain incomplete.

### Required roles and outputs

Before acquisition, `define` fixes outcome, region, period, sample/population coverage, 1–8 indispensable
roles and 1–16 required outputs (responsible role and expected scalar type). Original goal text remains
present. The definition cannot be weakened after seeing results. This freezes a planner interpretation;
it is not fabricated human approval or proof that the interpretation captured every implicit requirement.

Each composition binds roles to observations that actually participate in the executed path (including
original spatial ancestors) and were
discovered under that role. Outputs must survive projection/aggregation, have the required non-null type
in every returned row, and originate from the responsible role's observation. Required strings cannot be
empty or whitespace-only; numeric zero and boolean false remain valid. `count(field)` requires an
observed field and skips null; unqualified count cannot invent role-specific lineage.

The artifact remains `sample_executed`. Separately calculated `evaluation.status=partial` leaves the engine
exploring with gaps; a different composition can complete missing outputs. `requirements_met` describes
structural checks only. If `needsSemanticReview=true`, the session enters `review_required` and can continue
under the [review-continuation contract](goal-result-execution-v1.md#검토-중-재계획--2026-09-08-추가-계약).
CLI reports a non-success exit while approval is missing. Review is not a verified
connection or a detection of every contradiction. The current evaluator always requires semantic review:
region, time, namespace meaning and goal interpretation are not independently verified. Therefore current
composition does not produce `output_ready`; that status is reserved for a future evidence-backed acceptance
path. There is no planner-controlled approval or review-clearing action. This deliberately does not claim
that identity reasoning is implemented merely because unsafe completion is now blocked.
Current bounded acquisition cannot pass population coverage. Neither status promotes ledger evidence to
`sample_verified` or authorizes consequential decisions.

Requested narrative limits belong in `contract.explanations`, not invented data columns or mandatory
time/coverage datasets. Each entry has a unique ID, description and topic: provenance, temporal, coverage,
identity or measurement (maximum 8). The engine produces `evaluation.explanations` from participating
observations, executed checks and remaining uncertainty; the planner cannot submit the report text.
An identity explanation claims key comparison only when a join actually executed; a zero-join
projection retains spatial/source lineage without claiming an additional exact/trim key match.
Required data outputs remain independently enforced. Explaining an unverified time scope can satisfy a
request to state limitations, not a request to establish actual validity or replace missing numeric output.

Measurement explanations distinguish executed conversion, row-field sums, group sums and counts; they
do not describe a sum as executed for a projection-only recipe. Participating spatial observations add
their method, radius, anchor/candidate observation IDs, candidate PK, scanned/matched counts, comparison
count and retained pairs. These are pre-composition spatial counts, not final coverage or distinct places.
Nearest is conditional on the selected source and exact filters, not evidence of everyday proximity,
complete transport coverage or wheelchair routes. Raw row values are not needed for these explanations.
Scope unknown counts mean missing values **or** membership unresolved by a selected vocabulary; they
must not be described only as missing values or as proven geographic conflict.

### Field-bound temporal alignment

An explicitly requested date range can be frozen as `contract.timeWindow:{from,through}` alongside the
original period text. Dates are inclusive YYYY-MM-DD. Composition cannot remove or change this window.
An unspecified user period must not be silently replaced with a chosen date range.

`composition.time:{window:{from,through},bindings:[{observation,fromField,throughField?,format,meaning}]}`
requires exactly one binding for every participating original observation, including crosswalks and both
parents of a spatial observation (not the derived container). Window is optional
unless frozen by the goal. Fields are original unqualified record keys, never acquisition metadata.
Formats are date_v1 (YYYY-MM-DD), month_v1 (YYYY-MM), year_v1 (YYYY), compact_date_v1 (YYYYMMDD) and
compact_month_v1 (YYYYMM). Exact numeric JSON year/month tokens are allowed without exponent coercion.
Meaning is validity (requires both endpoints), reference_period or event (single period field only).

The executor carries the cumulative interval intersection through the entire join path. Month/year values
represent their declared calendar granularity, not fabricated exact event days or evidence of continuous
availability. Disjoint/out-of-window or missing periods do not match; malformed calendar values and
reversed validity endpoints fail explicitly. Empty/missing dates remain unknown, never an unbounded lifetime.
Original rows are unchanged. Calendar dates use no inferred timezone or timestamp conversion.

Join metrics expose common_overlap_v1, rejected candidate pairs and unknown-time pairs (a subset of
rejections). `evaluation.temporal` and requested evidence explanations reach the planner without raw rows.
`status=checked` means that date values were compared; `meaningVerified=false` remains explicit. No time
contract yields not_checked. Filtered-out source records are not evidence of population absence.

## Composition

An ordered connected join plan may include intermediate Data Nodes and composite keys. Supported
operations are exact/trimmed string-key joins, projection and bounded aggregation. Null keys do not match.
Every selected field must exist in observed rows. Duplicate expansion is checked before materializing the
result; output limits fail explicitly instead of returning a silently truncated success. Count and sum
aggregations operate after joins. Sum requires JSON numbers or an explicitly converted Measure.
There are no left/anti/spatial joins, arbitrary code, causality or absence inference. Explicit calendar
period alignment is supported as above; timezone conversion and field-semantic certification are not.

### Explicit numeric measures and source-grain sums

`measures:[{as:"capacity_number",field:"o1.ACEPTNC_POSBL_CO",format:"decimal_v1",unit:"persons"}]`
creates a new numeric field after joins, before aggregation/projection. Original observations and join keys
are unchanged. A measure alias cannot be used to establish identity in a join. At most 16 measures are
allowed; aliases are unique/unqualified and reference original qualified fields, not other aliases.
Output acceptance follows measure → source field (also through an aggregate), so renaming an unrelated
measurement cannot satisfy a required role.

The default measure operation is `convert` with one `field`. For disjoint columns within a source row,
`{as:"elderly",op:"sum_fields",fields:["o1.65-69세","o1.70-74세"],format:"grouped_decimal_v1",unit:"persons"}`
adds the explicitly named terms. An actual 65+ total must include every required age band, not just this
syntax example. It requires 2–32 distinct original fields from one observation AND one original source
record kind/side, no singular `field`,
cross-source arithmetic, alias chaining or repeated term. Every input is validated; any missing term makes
the result null instead of a partial total. The recipe preserves all field dependencies, and aggregate/role
lineage remains anchored to that same source record. Disjointness and common units are declared semantic
assumptions, not proven by arithmetic. Total population cannot substitute for an elderly count.

`decimal_v1` accepts ASCII signed decimals without exponents, commas or unit suffixes.
`grouped_decimal_v1` additionally accepts correctly grouped thousands commas. Optional `trim` and exact
`nullTokens` are declared explicitly; malformed or ambiguous values never become zero. Nulls remain null,
and required non-null outputs still fail evaluation. Units are recorded interpretations, not certified facts.
Numeric tokens are bounded to 256 bytes; existing JSON number exponents/scales are bounded to 1000.

Decimal conversions and sums preserve exact decimal values (including integers beyond 2^53). A sum
tracks contributing source row ordinals within each group. Repeating the same source row after a 1:N join
fails, while distinct original rows with equal values both contribute. Counts retain their explicit
joined-row/count(non-null field) semantics. Source pre-aggregation, allocation and cross-group total
semantics still need separate contracts; this guard does not certify aggregate usefulness.

CLI and MCP emit the exact JSON numeric token. `advance_goal` retains typed SDK input handling but
serializes its engine-owned output without the SDK v1.6.1 inferred-output-schema round trip through
float64. Its exact JSON is present in both structured content on the wire and text content; an inferred
output schema is consequently not advertised for this tool. Consumers must use a decimal-aware decoder:
the current Go MCP client's eager StructuredContent decoding itself rounds large numbers. Decode text
content with `UseNumber` (or equivalent) when exact numeric output matters.

An observed base may be projected or aggregated without an additional join. The same role/output,
lineage, time and coverage checks apply; ordinary single-source results are not automatically approved.
The current writer emits `sample_executed` for all recipes, replacing the experimental `sample_joined`;
historical diagnostic records keep their original status. See [Goal Result execution](goal-result-execution-v1.md).

The artifact contains the recipe, source hashes and times, computed joins/coverage metrics and bounded
rows. It is labelled sample_executed, not population verification or proof of business/causal value.
It also returns source requests, inspection-contract hashes and row hashes. CSV content hashes cover
downloaded bytes; API content hashes cover decoded canonical JSON. Provider bytes are not archived, so a
later request can produce different content. This is an auditable recipe, not guaranteed historical replay.

## Acceptance

- A scripted planner can execute a three-source fixture through the real acquisition adapters. Natural
  language role generation is evaluated separately against installed model CLIs and the live catalog.
- First incompatible candidate leads to an intermediate code mapping and a successful alternative.
- Composite-key marginal overlap does not masquerade as tuple overlap.
- Pairwise overlaps with an empty complete join, ambiguous row arrays, null keys, wrong field names,
  excessive expansion, unknown PK, stale revision, exceeded budget and replayed actions fail honestly.
- CLI and MCP share computed output and errors; strict semantic failure cannot return lexical candidates.
- A live catalog run separately demonstrates semantic.status=used. Fixture planner success is not a
  natural-language retrieval benchmark; live discovery outcomes are reported with evidence limits.

## Remaining goal-level work

- Validate the semantic interpretation of required roles, region, period and outputs against the original
  user goal and actual source evidence, beyond the implemented structural acceptance contract.
- Additional bounded tabular readers, pagination, source-grain aggregation, row-wise arithmetic and spatial
  transformations, driven by real blocked goals rather than inferred endpoints. STD first-page acquisition
  is not complete coverage.
- Structured namespace/valid-time evidence and source-record gold cases, including homonyms, changing
  identifiers and corrections. Unit fixtures do not establish entity-resolution precision or recall.
- Reuse of reviewed ledger evidence during retrieval, with freshness and supersession checks. The current
  engine retains local goal gaps; it does not learn durable global relationships from a successful join.
- Real goal benchmark: keyword-unknown → distant roles → required mapping → useful output, measured
  separately from search relevance and sample execution success.
