# Goal composition execution audit — 2026-09-07

This is a development-branch execution audit, not a success-rate benchmark or a release claim.
Current acceptance warning: an earlier natural-language run produced a cross-city same-name false join.
Historical output_ready/exit-0 records below are not goal success. The current engine returns review_required
for structurally complete but semantically unverified candidates; see the critical hard negative below.
The latest unseeded G2 diagnostic autonomously used the published-label vocabulary and produced six
candidate rows, but abstained. Its Line 2-only transit candidates were 7.2–8.95 km away; structural output
checks do not establish useful nearby transport. See the dated follow-up diagnostic below.

## Deterministic evidence

- `internal/goalwork/engine_test.go`: a natural-language goal is retained while a scripted planner searches
  people/shelters, attempts an incompatible direct join, receives a Gap, searches a code mapping and
  executes a three-observation alternative. Both alternatives remain visible. This validates the engine,
  not the model's ability to infer that plan from an unseen goal.
- `internal/goalwork/scope_test.go`: row-bound text scope checks exclude cross-city name collisions,
  preserve conflict/unknown counts without raw values, and keep the same engine open for another source.
  Matching scope still requires semantic review. Scope/time checks run before expansion and aggregation.
- `internal/goalwork/live_test.go`: the actual catalog search, unified FILE inspector, typed download
  resolution and CSV reader connect three HTTP fixture sources into an artifact with contract/content/row
  hashes. HTTP is a test transport; no credential or external provider contract is invented.
- `internal/goalwork/compose_test.go`: composite marginal overlap, leading-zero/type mismatch, null keys,
  unknown columns, duplicate expansion and pairwise-overlap/globally-empty paths do not pass. Numeric
  aggregation does not silently coerce string measures.
- `internal/mcpserver/goal_test.go`: an MCP goal cannot be accessed from another MCP connection; stale
  revisions fail; MCP and the standalone shared loop produce the same fixture rows.
- `internal/agentplan/goal_test.go`: supported provider response wrappers decode, fabricated evidence
  fields are rejected and artifact rows are removed from the external planner prompt.
- `cmd/odeduck/solve_test.go`: an explicit abstention is output as JSON and exits unsuccessfully, not as
  a fabricated completed goal.

These are synthetic contract cases. They do not yet constitute the real-source identity-resolution gold
set required for automatic consequential decisions.

Validation on this development branch: `go test ./...`, `go vet ./...`, `go build ./...`,
`go mod tidy -diff`, and race tests for goalwork/dataset/agentplan/mcpserver/cmd/odeduck passed.

## Local semantic runtime

The installed catalog contained 96,866 entries (snapshot synced 2026-09-04). The local semantic index was
initially absent. A complete build with installed `embeddinggemma:300m-qat-q4_0` produced 96,866 new
embeddings in 1146.6 seconds. This is local configuration state, not a checked-in or released artifact.

Command:

```sh
go run ./cmd/odeduck catalog search '폭염 야외 근로자' --require-semantic --limit 3 --format json
```

Observed `mode=hybrid`, `semantic.status=used`, model `embeddinggemma:300m-qat-q4_0`. Returned PKs included
15075950 (weather/heatwave data), 15103567 (Boryeong shade facilities) and 15100990 (outdoor-work heat
prevention material). This confirms execution of the semantic path, not correctness or composability of
all hits. A PDF search hit can be relevant while remaining non-executable.

## Live natural-language goal

```sh
go run ./cmd/odeduck solve \
  '폭염 때 어르신들이 쉴 곳을 찾는 데 도움이 되는 서로 다른 공공데이터를 연결해 표본으로 보여줘. 전국 단위이며 지역과 연도 해석의 한계를 명시해줘.' \
  --max-rounds 12
```

The first run exhausted 12 rounds on the search query/role validation contract. The original error did not
identify which field or length limit failed. The planner prompt was narrowed to short role labels and the
engine now returns field-specific limits, lengths and a correction instruction. No candidates or success
were fabricated during this failed run.

The second run made four semantic searches, all with `semantic.status=used`, and retained 31 candidates.
The inferred roles were shelter supply, elderly population, heat exposure and an alternative shelter source.
It performed inspections and attempted an API sample, but produced **zero observations and no artifact**
within the deliberately reduced 12-round budget. Result: `budget_exhausted`, exit code 1.

Observed blockers:

| PK | Observation | Honest consequence |
| --- | --- | --- |
| 15013199 | 전국무더위쉼터표준데이터 is in the catalog but has no recognized service type/dataTypes | unified inspection refuses to guess the delivery |
| 15107290 | 지역별 무더위 쉼터 metadata exposes request parameter names, but a callable endpoint/detail contract is missing | DatasetCaller refuses the sample instead of inferring a credential destination |

The runtime demonstrated keyword-free role planning, actual hybrid retrieval and contract-aware failure
feedback. It did **not** demonstrate an end-to-end real-data joined answer. More rounds alone are not
evidence that these contracts become callable.

## Decision and next gate

### Follow-up: fixed requirements and actual STD acquisition

A four-round natural-language smoke run first defined six required roles and eleven outputs, then made
two searches with semantic.status=used and retained 16 candidates. It still hit the unknown-delivery
inspection failure for 15013199 before the STD adapter was added. No observations/artifact; budget_exhausted.
The inferred period was explicitly unspecified. This confirms contract generation, not semantic validity
of all inferred roles or outputs. Synthetic CLI/MCP tests now separate partial artifacts from output_ready,
reject weakening the contract and reject renamed fields from an unrelated role.

The first-party standard page and JavaScript established a previously unsupported public JSON contract.
After implementing the bounded adapter, the actual command succeeded without login or keys:

```sh
go run ./cmd/odeduck inspect 15013199 --observe
```

Observed at 2026-09-07T01:09:06Z: delivery STD, 22 declared and observed column keys, 42,226 declared rows,
5 actual first-page records, 3,205 response bytes. Header SHA-256:
`31b4980d42cdc48139892ca6f9febb0d0a272c3f0edfe44f60265a15d0390962`;
page SHA-256: `501d8473a1b517582721cd9043bfb7c962fe9210ab1e7c98814ba43b54f4b61f`.
A separate two-record contract probe returned REFERENCE_DATE 2018-05-15. Catalogue freshness does not
establish row freshness. This is actual acquisition of one source, **not** an end-to-end goal result.

Contract tests reject wrong PK, duplicate/unadvertised/unsafe selectors, foreign or fabricated inspection
handles, nested/trailing/oversized JSON, over-limit rows and model-provided table/operation/asset inputs.
Exact JSON numeric precision, leading zeros, nulls and column labels are preserved. No release claim.

Re-running the same natural-language comparison goal with `--max-rounds 8` after the STD adapter reached
one actual observation by round 4: PK 15013199, STD, **1000 rows** and 22 observed column keys. It continued
with population and heat-related searches/inspections, ending with 24 candidates, three semantic searches
all marked used, one observation and no composition/artifact. Status budget_exhausted, exit 1, no action gaps.
Observation time: 2026-09-07T01:13:14.769085Z. Content SHA-256:
`970077613bbc263a55df1acddc771773fcfaf2ca1bf3c83c9f36ab4409f7706f`;
rows SHA-256: `e5b030b04f5fff19f1d9703fd3e168ef530aa1dc47cde6bdce2776f0423abcf3`.

All returned column types were strings, including capacity and coordinates. The initial contract requested
numeric capacity, so explicit validated numeric conversion is needed; weakening the requirement or
implicitly coercing identifier strings would be wrong. The planner also expressed narrative scope/time
limitations as source-bound outputs, revealing a further need to distinguish data columns from derived
evidence explanations in goal contracts. This run proves acquisition progress, not goal completion.

### Numeric execution follow-up

The live STD observation above drove explicit decimal/grouped-decimal Measure conversion rather than
loosening the numeric output requirement. Contract tests preserve source strings and identifier zeros,
reject guessed locales/unit suffixes/missing values, and carry measure lineage into required-role checks.
The three-source CSV adapter fixture now produces a numeric population output through an explicit measure.

Exact sum tests cover 9007199254740993 + 1 and 0.1 + 0.2. A population row joined to two shelters previously
contributed twice; a failing test reproduced the incorrect sum. The executor now tracks source row ordinal
contributions per sum group and rejects repeats, without collapsing distinct source rows of equal value.

The MCP public-seam test exposed a separate precision bug: SDK v1.6.1 output schema processing changed
9007199254740993 to 9007199254740992. `advance_goal` now serializes its typed engine result directly.
The regression test decodes the emitted text JSON with UseNumber and compares it to standalone output.
The SDK client's eager StructuredContent decoding can still round numbers; this consumer requirement is
documented rather than claiming arbitrary clients preserve decimal precision. No SDK dependency was edited.

These are deterministic contract verifications, not a new successful real-data goal benchmark. Temporal
alignment, narrative evidence outputs, source pre-aggregation and useful complete live outputs remain open.

### Temporal/evidence follow-up and 24-round live attempt

The development engine now binds observed record periods (day/month/year or declared compact formats),
intersects time across the complete path, and preserves an explicit requested window. Tests reject stale
records in a newer window, missing time, invalid leap days, reversed windows and pairwise-only period
overlap. A CLI/MCP shared-engine fixture carries the numeric result, computed temporal report and evidence
explanations through the public tool seam. Date-field meanings remain unverified, not silently promoted.

Requested narrative explanations are separate from required data outputs. A fresh run of the same natural
language comparison goal with `--max-rounds 24` generated two data roles (people/shelters), four actual
output requirements and three evidence explanations (temporal/coverage/identity). It no longer required
invented source columns for narrative limitations. Region stayed nationwide and period stayed unspecified.

Five searches all reported semantic.status=used. The run retained 36 candidates and acquired four actual
observations by revision 17. The first STD source remained available. One later source was PK 3062428,
`충청남도_노인인구 현황_20251130.csv`, 16 rows observed at 2026-09-07T01:43:21.800368Z. Its content hash was
`9e18b692e334f63a7e387eaaf47c56b0713e6a7beddbac52b560ef09a0607fd3` and rows hash was
`82e17114f7f724468843a3bfedd7792abd42d4780467ce419c096026409ef415`. It exposes separate age-band columns
65–69 through 100+, so a total elderly count would require an explicit row-wise calculation or another
source with that observed total; total population cannot be substituted for elderly population.

The process ended with exit 1 before its next action because the planner response could not be decoded.
State remained exploring; no composition/artifact was produced. The invalid raw response was not retained,
so the exact malformed field/operator is unknown. This is **not** evidence that joining the sources is
impossible, nor a completed natural-language goal benchmark.

After this run, a bounded format-repair path was added and verified with an actual helper subprocess:
one invalid decision followed by a valid correction succeeds; persistent invalid output stops after two
invocations. Invalid response values are not echoed into the repair prompt. Failed composition metrics are
also retained for the next planner action. The complete live goal has not been rerun after these repairs.

### Row-wise age counts and real geographic-grain hard negative

The observed age bands in PK 3062428 drove `Measure.op=sum_fields`: explicitly named distinct fields
from one source row, exact decimal arithmetic, null propagation, preserved field dependencies and the
existing repeated-source aggregate guard. Public goal/MCP tests preserve 9007199254740993 + 1 through
this operator. Duplicate terms, cross-source terms, alias chains and invented fields are rejected.

A scripted read-only comparison then acquired two sources through semantic search, inspection and the
existing FILE adapter. This fixed source pair/recipe is **not** a natural-language autonomous benchmark.
It attempted a trim join from population `구 분` to shelter `시군구명` and failed with zero matched rows.
Inspection of public geographic labels showed municipality names such as `공주시` on the population side
and smaller-area labels such as `공주시 계룡면` on the shelter side. Sharing a province in catalogue metadata
and having an apparently compatible column name did not provide the same observed join key.

The failed attempt was retained as the explicitly named opt-in hard-negative test
`TestLivePublicChungnamDirectJoinRejectsDifferentGeographicGrain`. Its passing assertion means **the
incorrect direct join remains rejected**, not that the original comparison succeeded. It uses
`ODEDUCK_LIVE_COMPOSITION=1`; no login, API key, application or external write is performed.

The recorded rerun acquired:

- PK 3062428, `충청남도_노인인구 현황_20251130.csv`, 16 rows,
  observed 2026-09-07T02:06:05.610295Z. Content hash
  `9e18b692e334f63a7e387eaaf47c56b0713e6a7beddbac52b560ef09a0607fd3`;
  rows hash `82e17114f7f724468843a3bfedd7792abd42d4780467ce419c096026409ef415`.
- PK 15118638, `N11914_충청남도_충청남도_재난안전포털_무더위쉼터_20240819.csv`, first 1000 rows,
  observed 2026-09-07T02:06:12.503762Z. Content hash
  `d2dbe699fdef96003c4a9ccb574d9901f897f7c3903e681199c343220aaac69b`;
  rows hash `e89c68fd6d5059840db1ce2258e77d8f991bc720f1b30a756b2358d50eb23a3c`.

Revision 9 retained a failed execution: 16 left rows, 1000 right rows, zero matched left rows and zero
output rows; status remained exploring with no artifact. No identity or aligned-time claim was made.
Asset dates differ and are not per-record validity evidence. A grounded administrative mapping or an
alternative source remains needed; guessing a string prefix would not resolve the identity contract.

The engine now exposes value-free `shape_v1` column profiles to planning: the population label has one
whitespace token, shelter labels two to three. Missing/null/blank and decimal-form counts similarly reveal
acquisition shape without sending sample values to the external planner. A public seam test verifies that
neither raw labels nor a sentinel value appear in planning JSON. Shape is not semantic certification.

An independent failing acceptance test showed blank strings incorrectly satisfied required outputs.
Blank/whitespace-only strings now leave requirements unmet; numeric zero and boolean false remain valid.
None of these deterministic fixes or the negative live test completes the original user goal.

### Scoped CSV acquisition and a real three-source sample output

The [official nationwide legal-dong file](https://www.data.go.kr/data/15063424/fileData.do) provides legal
codes alongside province, municipality and smaller-area names. A separate read-only CSV inspection counted
20,561 rows: the first 1000 had no Chungnam records, whereas the complete file had 2,275 Chungnam rows.
An initial-prefix-only adapter could miss the needed mapping despite successful search and download.

`sample.where` now performs bounded, exact string selection before the CSV output cap. Tests require
matching original values with AND semantics, reject prefix/code coercion/unknown columns, and do not skip
malformed excluded rows. An existing three-source public adapter fixture now places its mapping after
1001 irrelevant rows and still executes the comparison. Request hashes and selection scan counts survive
into the artifact; invalid API/STD filtering is rejected through both the engine and MCP tool.

The predeclared opt-in `TestLivePublicGongjuComparisonThroughOfficialMapping` then scoped its own goal to
a Gongju sample and fixed three sources. It did **not** change the separate nationwide natural-language
goal, and must not count as proof of autonomous discovery. All three searches used the actual semantic
path, and all acquisitions used inspected public FILE assets without login, API keys or external writes.

| Source | Selection | Observed rows and time (UTC) |
| --- | --- | --- |
| 3062428, Chungnam elderly population | 구 분=공주시 | 1 of 16 scanned; 2026-09-07T02:14:56.705391Z |
| 15118638, Chungnam heat shelters | first 1000 rows, no where | 1000; 2026-09-07T02:15:04.113185Z |
| 15063424, nationwide legal-dong | 시도명=충청남도 AND 시군구명=공주시 | 199 of 20,561 scanned; 2026-09-07T02:15:10.764263Z |

The two filtered file scans reached EOF; this only establishes selection coverage of those files.
Population content hash remained
`9e18b692e334f63a7e387eaaf47c56b0713e6a7beddbac52b560ef09a0607fd3`, selected rows hash
`f2cac6124c83bd230be6b956ae7c4c2db8cda48cc71a935339fda40392a0bee6`.
Shelter content hash remained `d2dbe699fdef96003c4a9ccb574d9901f897f7c3903e681199c343220aaac69b`,
rows hash `e89c68fd6d5059840db1ce2258e77d8f991bc720f1b30a756b2358d50eb23a3c`.
Mapping asset `국토교통부_전국_법정동_20260729.csv` content hash was
`e5657d4b53a16f72e42e9c0d91e84ff2d647e70dc56b9408da6fd057d0da45c3`, selected rows hash
`130cabc4a2e143e7ba1fecd59f3327f08e70728e0d56bef270355f1044e35654`.

The exact municipality join expanded one population row into 199 mapping rows; eight mapping rows then
matched the shelter legal-code field and produced **62 output rows**. Eight age bands yielded a numeric
65+ population of **32,242**. Example output (not a total across facilities):

| Municipality | 65+ population context | Shelter | Shelter source year | Matched legal code |
| --- | ---: | --- | --- | --- |
| 공주시 | 32,242 | 신영1리경로당 | 2023 | 4415025000 |
| 공주시 | 32,242 | 신영2리경로당 | 2023 | 4415025000 |
| 공주시 | 32,242 | 추계1리경로당 | 2023 | 4415025000 |

Engine status was output_ready, artifact sample_joined and evaluation requirements_met **for this fixed
sample contract**. needsSemanticReview stayed true and temporal.status stayed not_checked. The population
value repeats as municipality context per shelter and must not be summed across these rows. Asset years
and shelter row years differ; no current operation, aligned reference period, canonical identity or nationwide
representativeness is claimed. The provider also warns that its processed code list can differ from the
administrative-standard source. A current lookup is not a historically verified mapping.

### Separate 32-round natural-language rerun

The unchanged nationwide comparison goal was rerun with `--max-rounds 32` after row-wise measures,
value-free profiles and planner-format repair, **before** scoped CSV selection was available to that process.
It made seven semantic-used searches, retained 48 candidates and acquired seven observations:
15013199 (STD, 1000), 15129528 (insurance population, 1000), 15134776 (Gwangju Seo-gu, 14),
15159665 (Gimcheon, 20), 15159663 (Gimcheon, 23), 15064001 (Changwon mapping, 1000),
and 15118638 (Chungnam shelters, 1000). It did not suffer the earlier decode failure.

At revision 21 candidate capacity was exhausted. At revision 27 it abstained with no composition/artifact:
Gimcheon administrative areas did not have a grounded correspondence to the acquired shelter legal areas,
the Changwon mapping did not establish applicability, and the Chungnam shelter source lacked its matching
population observation. Exit code was 1. The contract retained nationwide sample scope, two required roles,
four typed outputs and three evidence explanations. This is a further **failed autonomous completion**,
not a success inferred from the separate scripted Gongju result. Targeted counterpart acquisition and
mapping selection within search budgets remain open, as do historical namespace and time validation.

### Source-context/admission audit and a further runtime failure

Inspection was reading publisher descriptions, geographic/temporal declarations and restrictions but
dropping them at the goal adapter seam. FILE declarations now survive into planning and the selected
observation/artifact. They remain publisher_declared, not verified record identity or validity. Tests retain
conflicting HTML coverage text as notices, explicitly mark UTF-8-safe truncation, avoid guessing structured
Schema.org coverage, and exclude unrelated metadata keys. API/STD retain only available names/source
context; missing scope stays unknown. The planner cannot submit a declaration as computed evidence.

The 48-candidate ceiling also conflicted with the existing 12-search × 8-hit allowance. A failing public
engine test reproduced rejection of the seventh distinct search. Capacity now follows that product (96),
while a thirteenth search remains blocked before network access. No search, inspection, sample, row,
byte or round limits were otherwise increased.

A fresh unchanged nationwide goal run with scoped CSV selection, these declarations and the corrected
candidate capacity ended at revision 7, not successfully. Two searches used semantic retrieval; 16 candidates
were retained, and PK 15013199 yielded one 1000-row STD observation. PK 15099158 was inspected with
API+FILE deliveries; its inspected asset was `지역별(법정동) 성별 연령별 주민등록 인구수_20260630.csv`.
Its publisher description reached planning.

The sample step for 15099158 failed with `unsupported protocol scheme ""` on `/data/15099158/fileData.do`.
The subsequent proposed action was a replay; the old standalone loop stopped with `action already attempted`.
No composition or artifact was produced, and exit code was 1. The inferred contract remained nationwide
sample scope with two roles, five typed outputs and three evidence explanations. This short run does not
establish whether the corrected candidate capacity improves autonomous completion.

The common DatasetCaller constructor had retained an empty base URL, unlike UnifiedInspector. Its public
test reproduced relative openapi.do and fallback fileData.do requests before any credential read. It now
defaults to the existing official portal base and preserves explicit test origins. Provider matchers, typed
operations, credentials and invocation permissions did not change.

Run also now gives one bounded chance to replace a replayed decision, without executing that request
again. Tests verify one external acquisition despite repeated planning, termination after a second replay,
and no reset of this allowance after an intervening action. This is distinct from provider JSON-format
repair. Neither correction by itself establishes acquisition/identity/completion success; the following
rerun isolates further gaps.

### Invocation identifier and acquisition-history follow-up

The unchanged 32-round nationwide natural-language goal, after base-URL/replay corrections but before
the identifier/history changes below, ended at revision 29 with status abstained and exit 1. Seven searches
used semantic retrieval, retaining 51 candidates. Ten inspections and all eight sample attempts were spent.
Four observations were acquired: STD 15013199 (1000 rows), FILE 15118638 (1000), FILE 3062428 (16),
and FILE 15044580 (305); observedAt ranged from 2026-09-07T02:38:16Z to 02:41:55Z.

One composition attempted a code bridge from Chungnam shelters through the groundwater service's
municipality-code file to elderly population. Its first exact province+code join matched zero of 1000
left rows against 305 mapping rows. The engine rejected the empty full path; no artifact was returned.
This does not establish absence or shared geographic identity. Other failures were a CSV column ceiling
(15152120), an 8 MiB download ceiling (15149025), and two API operation-selection failures (15099158,
15077871). The captured tool output truncated part of the large inspection list; the intact progress,
budget, observations, composition, execution and gaps support these counts, not a complete raw JSON archive.

The operation failures were an adapter mismatch, not demonstrated provider drift: goal inspection exposed
human operation titles, while DatasetCaller resolves REST names by the endpoint's final segment. Goal
inspection now uses the existing OperationName function and preserves the human title separately. A
public LiveDependencies Inspect→Sample test passes the returned name directly into the actual DatasetCaller
with fixture transport and credentials. It first reproduced both normal REST and uddi: failures before
credential access, then passed with exact invocation paths after the fix. LINK retains registry operation
names; no endpoint override, resolver, provider matcher or credential scope was broadened.

Planning also previously lost the original request conditions behind successful/failed samples. The shared
engine now supplies sampleAttempts: validated original request, hash, revision, time, outcome and observation
ID. A public test proves failed API parameters and successful CSV selection survive while injected caller
keys, mutated parameter maps and private rows do not. Replays and credential-rejected inputs do not add
acquisitions; CLI planner decoding cannot forge history/outcomes, and MCP preserves the same request hashes.
This is bounded session history, not the durable connection ledger or verified coverage.

Full go test/vet/build/tidy-diff and goalwork/MCP/agentplan race checks passed after the implementation.
A fresh natural-language rerun after identifier/history correction produced the false positive below;
these fixture passes are not autonomous goal-completion evidence.

### Critical hard negative: Kimcheon/Yeosu same-name join

The unchanged nationwide natural-language goal next returned exit 0 and output_ready at revision 21:
5 semantic-used searches, 36 candidates, 6 inspections, 7 acquisition attempts (5 observations), one
composition and 9 artifact rows. These historical statuses are **an unsafe acceptance defect, not success**.
The corrected operation identifier reached the credential seam, where the current missing login session
blocked API 15099158. Its FILE alternative exceeded 8 MiB; the planner continued with public alternatives.

It acquired Kimcheon elderly population (15159663, 23 rows), Kimcheon administrative areas (15126900,
23 rows), the national shelter STD (15013199, first 1000 rows), and a national administrative/legal mapping
published by Uijeongbu (15137835, first 1000 and then 186 selected rows). The selected request used
시도명=경상북도 AND 시군구명=김천시, scanning all 21,694 mapping rows. The engine's new history preserved
these two distinct mapping observations and the two acquisition failures.

The composition joined population to the selected mapping on province+municipality+administrative name:
22 of 23 population rows produced 185 rows. It then joined mapping 법정동명 to STD LEGALDONG_NM alone.
One mapping row matched nine shelters. Every resulting row joined **김천시 남면**, population 1390 dated
2026-05-30, to **전라남도 여수시 남면** shelters whose operation dates were 2019-05-15 to 2019-09-30.
The first shelter was 두포경로당 at 전라남도 여수시 남면 두모리 1397-2. This is a different municipality,
not a useful within-area comparison. A warning about duplicate names and needsSemanticReview=true did
not make it acceptable to return goal success.

Evidence for the three participating observations (SHA256, actual observation time):

| PK | observedAt (UTC) | contentSha256 | rowsSha256 |
| --- | --- | --- | --- |
| 15013199 | 2026-09-07T02:49:25.326202Z | 970077613bbc263a55df1acddc771773fcfaf2ca1bf3c83c9f36ab4409f7706f | e5b030b04f5fff19f1d9703fd3e168ef530aa1dc47cde6bdce2776f0423abcf3 |
| 15159663 | 2026-09-07T02:50:28.753534Z | 44562395a3a1a9a43252b664b5352fa6b64b27e0cd693cbca3485a68b14e539b | 55efa497e5daec77babc77b14c82aba149dc3cdc4ae0ce5f7cfedda5937a51a8 |
| 15137835 | 2026-09-07T02:52:17.62082Z | 0eadd502d5dcdf4da71d22d1edd70e0104af8e19e722fd3cda78642639bbba5d | fe62245e98938ade509fe0d35aab65a9ecc4ebb2b715f87b08c787c59356a136 |

Full process stdout and stderr were captured separately under `/tmp/odeduck-goal-audit.lqxRHd/`
(`result.json` and `progress.log`); this temporary local capture is not a versioned or durable archive.

The immediate safety correction makes structural requirements_met + needsSemanticReview terminate as
review_required, retaining the candidate artifact but returning CLI failure. The current evaluator has
no evidence-backed path to clear semantic review, so it no longer automatically emits output_ready.
A reduced public-engine regression reproduces the cross-city name collision; CLI rejects both the new
review status and a legacy output_ready carrying unresolved review. MCP and standalone retain the same
candidate status and exact numeric rows. Existing scripted artifact tests now expect review_required;
their historical row-generation evidence is preserved, not reclassified as verified identity.

This blocks an unsafe success signal; it does **not** yet detect contradictory source scopes, reject all
false candidate rows, validate historical identity, or autonomously repair the join. Next acceptance work
must bind identity/scope evidence to actual source records and reject/replan on contradiction. Model
assumptions, matching labels, publisher descriptions and “official” titles cannot certify an edge.

### Row-bound scope check: actual nine-pair rejection

The current shared executor can bind additional text-scope conditions to observed fields of a key-equal
candidate pair. Whole-token equality or directional prefix rules are explicit, bounded and computed before
time checks/expansion/aggregation. Missing/null/blank is unknown, not agreement. Invalid types, absent
fields or unsupported transformations fail. No geographic parser, alias table or canonical entity claim
was inferred from the benchmark. Scope-check meanings remain unverified and cannot clear review_required.

`TestRowBoundScopeRejectsCrossCityNamesAndKeepsReplanning` first failed because the old executor ignored
the proposed scope condition and returned the wrong-city candidate for review. After implementation it
rejects the candidate and retains scope metrics, then acquires another fixture source in the **same** goal
and yields a scope-matched review candidate. Separate tests cover whole-token boundaries, Unicode
whitespace, reverse/equality rules, aliases left unresolved, unknown values, invalid fields/types/limits,
multiple AND conditions, temporal coexistence, pre-expansion filtering and projection/aggregation.
The 200×200 shared-name fixture computes 40,000 candidate pairs, excludes 39,800 by scope and materializes
only 200; it does not first overflow the 1000-row output limit. MCP preserves these typed conditions and
computed counts; planner decoding rejects invented matched counts or verified flags.

Live fixed-source recheck (18.96 seconds, 2026-09-07):

- 15159663: 23 population rows, observedAt 03:08:38.671770Z.
- 15013199: 1000 shelter rows, observedAt 03:08:41.814459Z.
- 15137835: 186 selected mapping rows, observedAt 03:08:49.832616Z; all 21,694 asset rows scanned.

All three contentSha256 and rowsSha256 values exactly matched the preceding unsafe run's table. The first
province+city+administrative-name join still produced 185 rows from 22 of 23 population rows. The second
join's nine equal legal-name pairs were tested with leftParts=[population.시도, population.시군구],
rightParts=[shelter.LNMADR], rule=left_prefix_v1. Counts were candidatePairs=9, matchedPairs=0,
conflictPairs=9, unknownPairs=0. The engine returned no artifact, retained a failed execution at revision 12,
and remained exploring. The live test passed by reproducing and excluding the known false candidates,
not by generating a correct comparison table. Full test/vet/build/tidy-diff and related race checks passed.

A separate unchanged natural-language goal run using the new planning contract ended at revision 13 as
review_required, exit 1 (stdout/stderr under `/tmp/odeduck-scope-audit.6CeS76/`). Three searches used semantic
retrieval; 24 candidates, 3 inspections and 4 sample attempts yielded 2 observations. FILE/API attempts for
15097972 failed on sample byte budget and missing login respectively. The planner independently selected
Yeonsu-gu shelters (15157635, 68 rows) and local elderly population (15064935, 15 rows), then made a
trimmed administrative-name join and exact numeric measure conversion, producing 68 review-candidate rows.

The source declarations describe both datasets as Yeonsu-gu. All 68 output addresses begin with the two
tokens 인천광역시 연수구; 15 distinct original administrative-name strings participate. The first example
has 옥련1동, elderly population 3943, shelter 연수새마을금고 at 인천광역시 연수구 한나루로 181. These are
candidate output observations, not independently certified identity, population completeness or current
operation. The population asset is labelled 2025 Q4 and the shelter declaration 2026-06-02; no row-bound
dates were available for alignment. Repeated local population is context per shelter, not an additive total.

| PK | observedAt (UTC) | contentSha256 | rowsSha256 |
| --- | --- | --- | --- |
| 15157635 | 2026-09-07T03:11:30.109830Z | 2bb881122f8a5a1cb282196c0e63812fa5391ae399b2373b1b27a671147e2b1f | 3ba1d049a79a9dd0e6e31fe2e61c8313218deb0ba7f45749f12b5c0201eff8c0 |
| 15064935 | 2026-09-07T03:12:23.209995Z | f041a63fa5fc771ea7584a1e405a85558d81202b2ea17a61274c781675127bc8 | f13c01b8772f6bdb4bbaf5c13bd4442d622eca8ba03d205cfc6185752b0ebbee |

The planner did **not** use the new scope operator: the population observation lacked row-level province
and municipality fields, which it correctly recorded as a remaining gap in its assumptions. Thus this run
demonstrates targeted alternative discovery and actual candidate-row generation, not autonomous scope
validation or goal completion. It ran before the follow-up explicit no-scope explanation text/meaningVerified
serialization was added; the executable scope comparison and planning contract were already present.

The next evidence-model issue is a source's declared coverage versus each record's identity. Requiring an
unavailable parent field cannot manufacture stronger evidence; nor should a publisher's location or an
arbitrary matching quotation certify record scope. Source-scoped claims need exact provenance, explicit
uncertainty and action-specific acceptance rules, with contradictory/mixed-coverage hard negatives.
The current automatic review-clearing path remains unimplemented.

### Cited source-scope hypothesis: parentless population records

The Yeonsu candidate exposed a real missing interface: the population rows contain administrative names
but not parent province/municipality columns. Scope predicates can now reference an exact quotation from
that observation's retained description or spatialCoverage, instead of row parts on that side. The common
module locates the text and returns a proposed_scope claim with observation/declaration/evidence hashes,
source identity, quotation position and truncation. It does not insert a constant into source rows, use
the publisher's name as geographic scope, or accept a planner-supplied verification receipt.

The public-engine test first failed with the old missing-field-side error, then passed after the citation
path was implemented. One compatible address remains while a foreign-city address is excluded; original
population columns remain unchanged and no raw foreign row reaches PlanningView. Tests reject wrong-side
or unused source references, nonexistent quotes, missing declarations, duplicate metadata, provider/title
citations, simultaneous citation + parts, invalid UTF-8 and oversized quotes. Source corrections change
the observation/declaration/claim hashes without rewriting an earlier receipt. MCP preserves the same
grounded claim; the standalone decoder cannot submit claims, source URLs or verified flags as evidence.

Negated and mixed-coverage declarations are explicit semantic hard cases: a literal quote can occur in
them and even pass a mechanical predicate, but remains proposed_scope / meaningVerified=false. Tests
do not pretend to have implemented a natural-language negation or coverage verifier. The engine still
requires review, and the explanation states that locating text is not validating each record's scope.

`TestLivePublicYeonsuComparisonRetainsCitedScopeProvenance` re-read population 15064935 at
2026-09-07T03:32:44.405049Z and shelter 15157635 at 03:32:51.168577Z. Both content and row hashes matched
the preceding natural-language run's table. The 15 population rows and 68 shelter rows produced 68
candidate pairs; the cited `인천광역시 연수구` prefix matched all 68 recorded addresses, with zero textual
conflicts and zero missing scope values. The quote occurred once at byte offset 14 of the retained
population description; that declaration was not truncated.

The emitted receipt had:

- observationSha256: `31fb5d20e6958d04ccb42480ca1b31070ee479b402083a5eb32423b63cff077b`
- declarationSha256: `2af69fcf6193dadb9084b88a3edfadd63f94dfefcdccaf21aeb22c1fe02b5378`
- evidenceSha256: `6a749b123518281e9c1b435dbd12a03a87e386a4fe743673449b2532d25deb66`
- claimSha256: `dd12a14f1c07b8047de67203e2051343841b1e402920fcfc210573fb7b1eca67`

Status remained review_required, not output_ready. The test passes for executable provenance-backed
hypothesis comparison, **not** verified identity or autonomous goal completion. No new natural-language
run has yet established autonomous selection of this citation contract. Full test/vet/build/tidy-diff and
related race checks validate the shared implementation; interpretation, action-specific acceptance and
cross-session evidence reuse remain unfinished.

### XLSX source-reader replay against the frozen G4 reference

`TestLiveXLSXSchoolReaderMatchesIndependentCitywideReference` downloaded the public school workbook from
the exact URL retained in `goalbench-v1/education-citywide-reference.json`. SHA256 remained
`c8f57fc8e3bd7e70175ff0debd1529a17539e526e436dc244fcea39ed9a0f695`.
The production reader's `구·군별!A27:AM37` matched every retained original cell in the 11 reference rows,
including whitespace. It recorded 292 formula cells as cached observations, without executing formulas.
The reference was not rewritten from the reader output. No login or external mutation was needed.

A separate HTTP fixture uses the actual search/inspect/FILE adapter to read XLSX, join CSV and convert
numeric storage strings only through explicit measures. Source row numbers, formula locations, request
hashes and source hashes survive to the artifact; copies cannot mutate retained selection evidence.
Planner decoding and MCP schema tests accept the selector, while API/STD and mixed CSV predicates reject
it. The fixture remains `review_required`; these are source-reading/contract tests, not unseeded discovery,
verified header semantics, population acceptance or completion of G4/M3. Goal inspection does not yet
expose worksheet layout in that initial reader change. The following structural discovery work closes
that narrower gap, not the remaining interpretation/completion requirements.

### Value-free XLSX layout discovery and revision pinning

The same public G4 workbook was reread through `LayoutXLSX`: 12 exact worksheet names were discovered.
Selecting `구·군별` returned cell-derived extent `A1:AM41`, 40 stored rows and 1462 stored cells. All 11
reference row positions were present in retained row shapes; merge metadata was not truncated. The
subsequent original-cell replay still matched the frozen source hash, all retained reference cells and
292 cached formula positions. This establishes structural discovery and unchanged-source reading, not
automatic selection of the post-reorganization table or interpretation of its April census date.

The HTTP adapter fixture now runs search → inspect → list layout → selected-sheet layout → hash-pinned
sample → CSV join/explicit numeric conversion. MCP tests use the same layout request and sample pin.
Separate tests reject workbook drift before retaining rows, preserve the failed attempt, acquire a new
layout revision and then recover through a newly pinned request. Refresh cannot exceed the six-read
budget. Malformed coordinates/tails and unknown sheets fail; retained-row/merge truncation is explicit.
Layout JSON contains no cell values, formula code or inferred header text. The existing planner policy
still excludes raw rows. Bounded source-text interpretation requires a separate transmission-policy
decision; no such permission is inferred from this structure-only feature.

### Exact ZIP CSV acquisition against the frozen G2 records

The production reader discovered exact member paths in the public accessibility ZIP and matched all
eight retained museum/accessibility records across its two CSV members. ZIP SHA256 remained
`204d7e775c1b8854d7aa1ca520685695303bad52e0d71edb4ef3baa540ac755f`; selected member hashes matched
the independent reference. No reference values, IDs, member paths or source ordinals were changed.

The initial replay exposed an ordinal convention mismatch, not source drift: independent Python rereading
of the same bytes located 검단선사박물관 at data record 133, which is CSV record 134 including the header.
The production provenance explicitly starts AFTER the header, so the replay now compares dataRecords + 1
to the frozen csvRecord. All eight full source-record value maps then matched (empty CSV values map to
the existing reader's null representation). This convention is explicit in the implementation contract.

The shared engine HTTP fixture discovers a ZIP layout, pins its source revision, selects/filter-reads two
CSV members, and joins their observations while retaining member hashes and original record/line positions.
MCP wire tests retain the same selector and provenance. Unsafe/duplicate decoded paths, nested archives,
damaged member CRC tails, invalid delivery/selector combinations and lossy text decoding fail. Reading the
whole selected member for checksum validation does not make a retained prefix representative or complete.
These are source-acquisition proofs, not G2 completion: larger transit acquisition, spatial comparison,
current-operation evidence and action-specific acceptance remain unfinished. No raw source rows were
added to external planner input, and no live authentication or external mutation was needed.

### Full direct CSV scan against the frozen G2 transit source

`TestLiveFullCSVScanMatchesIndependentTransitReference` used the public file URL retained in the frozen
mobility-comparative reference. The initial UTF-8-only implementation failed on its legacy header; the
reader now streams strict, round-trip-checked EUC-KR without buffering the complete source. The replay
read 20,735,435 source bytes, 227,065 data records and 9,278 exact `도시명=인천광역시` matches. All 12
independent stop-record value maps matched at their original data positions (reference CSV ordinals
include the header). Source SHA256 remained
`1db7e5d0cdc541e86a957b7584f2ddf8e4b1d29b02594984a01c5752ac13af06`.

Fixture tests check excluded malformed tails, UTF-8/EUC-KR failures and buffer boundaries, oversized
fields/parser input/retained rows, cancellation and consumer errors. The shared HTTP execution fixture
preserves the explicit full-scan request through a three-source join; MCP preserves whole-file counts
separately from partial retention. Planner decoding accepts selectors but rejects fabricated scan reports.

This is a seeded source-reader proof, not autonomous source discovery or G2 completion. Sample retention
still caps at 1000 rows / 2 MiB; nearest records must be computed over all 9,278 matches, not that prefix.
Coordinate interpretation, current-operation evidence and acceptance remain unverified. No login, external
mutation, oracle rewriting or raw-row transmission to an external planner was needed.

### Conditional nearest-record replay across all G2 transit matches

`TestLiveNearestMatchesIndependentMobilityDistances` read the original museum ZIP member again and
selected the four frozen facility record positions. This test-only anchor subset is seeded by the oracle;
it is not a claim that the planner autonomously selected four museums. The candidate CSV was first read
to pin its actual source/contract revision, then scanned again through the shared production adapter and
goal engine's nearest reducer. Source hashes stayed equal to the frozen reference; reference values and
expectations were not rewritten.

The reducer scanned 227,065 candidate records, selected all 9,278 exact Incheon records and performed
37,112 pair comparisons. All 12 nearest source IDs, manager namespaces, original record positions and
distances matched the independent oracle (distance tolerance 0.000001 m is numerical replay tolerance,
NOT positional accuracy). The paired result remained `review_required` with `meaningVerified:false`.
It preserves ICB/GGB records separately and does not infer distinct physical stops or accessible routes.

A separate full HTTP fixture exercises catalogue search, actual FILE inspection, baseline observation,
whole-source nearest reduction and composition through the shared `Run` loop. Its nearest candidates are
beyond the retained 1000-row prefix. MCP wire tests use the same typed observation references and retain
scan/pair evidence. Negatives cover invalid/missing coordinate values and axes, wrong observation/asset/
field/method, changed source or contract, incomplete scans, tail errors and memory/anchor limits. Request
and evidence views are detached; raw pair values remain outside planning input. Sum of copied pair-source
values is explicitly unsupported rather than bypassing the original-record duplicate-sum guard.

G2 still needs autonomous grounded anchor selection, accessibility/date/event composition and an explicit
coordinate/coverage acceptance policy. General spatial transformations, source-wide aggregation and M1's
additional hard negatives are not completed by this replay. No login, external mutation or broader
source-row transmission permission was used.

### Original-record lineage through spatial composition

The first source-lineage tests failed in four distinct ways: all spatial-source sums were blocked, mixed
anchor/candidate `sum_fields` were accepted as one-record arithmetic, copied fields lost their original
role attribution, and a spatial result required an artificial back join before output. The shared engine
now resolves original record addresses through pair provenance for joins, measures, sums, output roles
and temporal alignment. [The lineage contract](../specs/source-record-lineage-v1.md) defines those semantics.

Fixture verification distinguishes two equal numeric values from different candidate records (sum 2)
from repeated contributions by the same original anchor or candidate (error). Separate groups may each
use one original record; this does not make groups additive. Two same-name/same-coordinate anchor records
produce four intended pair rows; a back join excludes four conflicting original-row substitutions rather
than expanding to eight rows. Both join directions reject a prefix row whose latitude matches a selected
candidate but whose original CSV address differs. These numeric fixtures test record arithmetic, not
meaningful addition of geographic coordinates or physical-stop identity.

The live G2 replay now projects the spatial observation directly. Four actual ZIP source records, all
9,278 exact Incheon candidate records and 37,112 comparisons still yield the same 12 frozen nearest
records/distances. No expected values, reference positions or source hashes were changed. The final status
remains `review_required`; direct projection is not autonomous anchor selection or semantic approval.

Temporal tests bind both original parents. An actual selected candidate dated 2025 can pass a 2025 window
even when the acquisition prefix contains 2024 records. Selected 2024 or missing dates reject both pair
rows, including when no further join occurs. Temporal eligibility after nearest ranking does not refill
ranks among all eligible candidates; this is recorded as a limitation, not a nearest-in-period proof.
MCP replays a failed lineage back join, then succeeds at a source-aware sum without an artificial join.
Original source roles and all three observation revisions survive into the artifact. No raw source values
were added to external planner input, and no authentication or external mutation was required.

### Unseeded G2 diagnostic: retrieval works, address interpretation still blocks composition

On 2026-09-07, 07:06:00–07:12:26 UTC, the real `solve` command received only the frozen G2 question.
No dataset PK, source name, oracle row, coordinate or scripted action was supplied to the Codex planner.
It used the default required-semantic policy and a 32-round budget against the 96,866-entry catalogue
synced at 2026-09-04T05:25:13.745338183Z. The opt-in diagnostic took about 386 seconds. Its Go test PASS
means that the harness captured a valid view; **the actual solve command failed** with `abstained`,
revision 27, seven searches, 50 unique candidates, seven successful observations, one failed acquisition,
one failed composition and no final artifact. No login, application or credential change was needed.

The versioned [diagnostic record](../../internal/goalwork/testdata/goalbench-v1/mobility-unseeded-diagnostic-20260907.json)
retains the interpreted contract, source revisions, requests, spatial provenance, attempted composition,
computed failures and the separate retrieval replay. It is not a positive goal oracle or an M5 gate pass.
`cmd/odeduck/solve_live_test.go` reproduces a new run only when `ODEDUCK_LIVE_SOLVE=1`; normal tests skip
it. The live test logs a private temporary directory containing `view.json` and `summary.json`. A new
model/source run can differ; preserve this dated record rather than replacing it with the next result.

Actual progress:

1. All seven searches reported `semantic.status=used`, with `embeddinggemma:300m-qat-q4_0`.
2. The planner discovered and read the accessibility ZIP, culture-space CSV, five-museum CSV and transit
   sources without source hints. It selected the ZIP member through the real layout contract.
3. Bus source 15074309 failed on candidate record 1's out-of-range latitude. The planner did not swap
   axes or treat this as evidence of no nearby bus. It searched another role-compatible source.
4. Rail source 15083751 supplied 71 records. A complete scan made 355 comparisons for five facilities
   and retained 15 nearest station records. Common datum, positional accuracy and traversability remain
   unverified; this is autonomous conditional spatial reduction, not an accessible route result.
5. Joining the accessibility observations produced nine exact-name candidate pairs, all rejected by
   `right_prefix_v1` scope comparison. Searching a standard museum alternative then consumed the last
   acquisition slot. Generic `FCLTY_INFO` was not relabelled as observed wheelchair accessibility.

The final failure must not be described as nine proven geographic conflicts. A subsequent independent
download of [the five-museum CSV](https://www.data.go.kr/data/15147282/fileData.do) had 3,185 bytes and SHA-256
`83ce13e2d347c188a183de56cc01dc27b9d04df434592b1075c579e8b427833e`, equal to the run's observed revision.
Its source date is 2025-03-31. Compare the following original text with the separately frozen accessibility
records in [the G2 reference](../../internal/goalwork/testdata/goalbench-v1/mobility-comparative-reference.json):

| Exact matching facility name | Museum data record (after header) / address | Accessibility CSV record (including header) / separate location fields |
| --- | --- | --- |
| 인천광역시립박물관 | 3 / 인천광역시 연수구 청량로160번길 26 | 943 / 인천 · 연수구 · 청량로160번길 · 26 |
| 인천도시역사관 | 4 / 인천광역시 연수구 인천타워대로 238 | 937 / 인천 · 연수구 · 인천타워대로 · 238 |
| 한국이민사박물관 | 5 / 인천광역시 중구 월미로 329 | 805 / 인천 · 중구 · 월미로 · 329 |

The accessibility ZIP hash also equals its frozen revision. These three facility-name candidates expand
to nine pairs through three station records per facility. The first scope token is `인천광역시` versus
`인천`; exact-token disagreement explains rejection. The remaining shown address pieces agree, but this
does not establish current same-entity identity, survey dates, public status or a globally valid alias.
The museum file also says `검단선사박물관` where the accessibility reference says
`인천광역시검단선사박물관`: name candidate generation has a separate representation gap.

The interpreted contract added `coverage:population`, although the original question did not explicitly
say every facility. This is the planner's interpretation, not independently verified user scope. The
diagnostic preserves that issue; neither calling every prefix population-complete nor silently narrowing
the user's intended scope would fix I1. Even a clarified sample scope would not resolve the failed
accessibility join or complete the date/meaning requirements.

#### Post-hoc retrieval comparison, not a causal goal ablation

The seven *actual generated queries* were replayed against lexical and hybrid retrieval with the same
limit of eight and local catalogue. This did not rerun the planner in a lexical-only condition.

- For `인천 문화 관광시설 장애인 편의시설 휠체어`, the actually acquired accessibility source 15109171 is
  hybrid rank 1 and absent from lexical top 8. It is also absent from the union of lexical top-8 results
  across this run's seven queries. This establishes a useful candidate-retrieval contribution.
- The acquired museum source 15147282 appears at rank 7 in both modes for the public-museum query.
  The rail source 15083751 is lexical rank 2 and hybrid rank 3 for the rail query. Semantic retrieval did
  not improve every needed source and must not receive credit for all discoveries.
- Model calls, different future queries, unseen goals and final-output usefulness are not controlled by
  this replay. It cannot establish an autonomous completion-rate improvement or satisfy the I3/M5 gate.

The next architectural work belongs to I1/I5/I7 and M2/M4: distinguish literal scope disagreement from
verified geographic nonidentity, acquire revision-scoped official identifier/name/address correspondence,
and make that evidence usable for bounded transformations and replanning. Preserve the raw lexemes and
the existing hard-negative checks. A global suffix-strip rule, fuzzy name merge, larger search budget or
graph database does not supply the missing evidence. Raw source rows were not added to the external
planner; any change to that policy remains a separate decision. Source acquisition and spatial operators
now have autonomous-use evidence, but this run is still an unsuccessful user outcome.

### Published scope vocabulary and replay of the nine-pair G2 failure

The next investigation found that a generic numeric province-code bridge would introduce another
identity error. Actual downloads from three official data.go.kr FILE contracts gave these records:

| Source | Source snapshot label | Bytes / full CSV SHA-256 | Observed Incheon code |
| --- | --- | --- | --- |
| [K-water groundwater province codes, 15044581](https://www.data.go.kr/data/15044581/fileData.do) | 2024-12-31 | 387 / `a14e5b6a70461124a8eb6f183a8a3f2df1d86e5cff090d1ba934d523412135e7` | 28 |
| [EPIS agriculture portal province codes, 15122582](https://www.data.go.kr/data/15122582/fileData.do) | 2023-09-11 | 328 / `326067242732e638fe75aee6e8c001073b2d4ea387dc25febce9534beda83c85` | 4 |
| [Agricultural area survey province codes, 15137167](https://www.data.go.kr/data/15137167/fileData.do) | 2025-09-10 | 260 / `53a97337075ee882dc04d1a2541c6fcce1c7dd1802c74ead326aee1c51ece889` | 23 |

The first file says code 26 is 부산광역시; the third says 26 is 울산광역시. Titles saying `시도코드` and
equal decimal strings do not establish shared namespace. These files were reconnaissance, not a
production crosswalk. Another catalogue candidate, 15151008, had no observable FILE asset; no download
contract was invented for it.

An actual full/short *label* correspondence was found in the same publisher namespace instead:

- The [SGIS province code table](https://sgis.mods.go.kr/developer/html/openApi/api/dataCode/SidoCode.html)
  has 3,007 bytes and SHA-256 `6bb5ea7101f87b4fa161f1ab9926e3a989f82e21ede73f9ebefdf8226486b1ff`.
  Non-commented table rows associate full names with SGIS codes. The HTML also contains commented-out old
  names; these were explicitly excluded rather than silently making historical equivalences.
- The official [map script](https://sgis.mods.go.kr/statexp/js/map.js), 369,918 bytes, SHA-256
  `96b73197feb076e4891296f64a35b4d59f619576f2443e25ce1111c94f5000ac`, has `selectSidoNameChk`'s checked
  branch at line 4481 associating short labels with those same SGIS codes. The script was read as text,
  not executed. The [official tutorial](https://sgis.mods.go.kr/statexp/jsp/tutorial/statsexp/index.html)
  describes this province short-label display option.

The frozen [17-label vocabulary](../../internal/goalwork/vocabularies/kr-sido-labels-20260907.json) therefore
records published spelling equivalence, not an assertion that the publication is a complete current
administrative map. Its observation date is distinct from any geographic effective period. Runtime does
not call SGIS or expand dataset acquisition outside data.go.kr. Source-record numeric codes never enter
this label comparison.

Red tests first reproduced the failed `인천광역시`/`인천` scope comparison through the public goal engine;
the previous scope type did not retain or apply the proposed vocabulary field. The shared engine now
accepts the explicit fixed vocabulary ID, applies it to both row-bound first tokens only, and retains its
source evidence in computed scope metrics. Tests keep the original failed recipe, original strings,
observation hashes and required semantic review. All 17 full/short label pairs are exercised. Different
provinces/districts, word substrings, facility-name differences, numeric codes, unlisted old names and
source-citation mixing do not pass. Planner decoding rejects invented mappings/receipts; MCP wire tests
exercise the same literal-failure → vocabulary comparison path.

`ODEDUCK_LIVE_SCOPE=1 go test ./internal/goalwork -run TestLivePublishedScopeVocabularyRecoversRecordedMobilityPairs -count=1 -v`
reacquired the exact accessibility ZIP, museum CSV and railway CSV from their official contracts. All
source hashes and retained row counts matched the earlier diagnostic. It omitted unused acquisitions,
so observation IDs were remapped in the test recipe; this is explicitly a **seeded replay**, not another
autonomous discovery. The nearest computation still scanned 71 station records and compared five
facilities. Literal scope checks again rejected nine pairs. Adding only the published vocabulary recovered
nine candidate rows: three each for 인천광역시립박물관, 인천도시역사관 and 한국이민사박물관. Original ramp/toilet
`Y` fields agree with the independently frozen G2 reference; the museum reference date remains 2025-03-31.
These are recorded source attributes, not current accessibility or route safety claims.

The original `population` contract was preserved. Status remained `exploring`, evaluation `partial`, with
semantic review required. Name variants, whole-goal coverage, reference dates, common datum and real
facility identity still need work. This establishes a concrete M4 label-interpretation path without
claiming M4 or the user's objective complete. Full build/tests and goalwork/agentplan/MCP race tests passed.

### Unseeded G2 follow-up: autonomous vocabulary use, still not useful nearby-transit completion

The same opt-in `TestLiveSolveUnseededMobilityDiagnostic` harness ran from
2026-09-07T07:37:14.408323Z to 07:43:51.173645Z (396.78 seconds). Only the frozen G2 natural-language
question was supplied: no source PKs, expected rows, action script or source-cell text was injected into
the external Codex planner. The [versioned diagnostic](../../internal/goalwork/testdata/goalbench-v1/mobility-unseeded-vocabulary-diagnostic-20260907.json)
retains the interpreted contract, requests, source hashes, recipes and computed metrics, excluding raw
output cells. The private harness retained the full artifact for local inspection.

All five searches used semantic retrieval (`embeddinggemma:300m-qat-q4_0`), returning 35 distinct candidates.
Seven observations were acquired. The planner explicitly selected `kr_sido_labels_20260907_v1` in both
executed compositions. The first produced 12 facility/accessibility rows. The second produced six rows
with transit; all three roles and four typed output requirements passed **structurally**. The inferred
`population` requirement still failed; temporal alignment was `not_checked` and semantic review remained
required. Final status was `abstained` at revision 26 and the solve command failed. The Go test's PASS
means the diagnostic was captured, not that the user goal passed.

The source path was accessibility ZIP 15109171 (first 1,000 member rows), culture-space CSV 15066560
(940 rows), Line 2 station CSV 15041339 (27 rows), museum list 15091408 (29 rows, then 15 exact-filtered
public-museum rows), museum coordinates 15147282 (five rows), and 15 derived nearest pairs against the
27-station file. The second composition's museum back-join had 12 key candidates, six scope matches
and six scope conflicts. The following accessibility join had six candidates and six scope matches.
Conflicts are text-rule outcomes, not independently established nonidentity.

Post-run local inspection exposed an important usefulness failure: the six executor distances ranged
from **7,200.5763299829805 to 8,952.183195478106 m**. Three rows concerned 인천도시역사관 and three
한국이민사박물관. These were nearest records in an **Incheon Line 2-only** file, not the whole transit
network. No independent distance oracle was computed for this new source; the numbers are actual engine
outputs, not frozen expected answers. Even perfectly computed distances against that limited candidate
set do not establish useful nearby transit, let alone wheelchair routes. A string in `nearby_transit`
cannot serve as an acceptance test for the meaning of “nearby.”

This run establishes autonomous use of the new label rule, not a causal improvement rate: source choices
and the interpreted contract differ from the earlier run. The original question did not explicitly say
“all”; the planner inferred population coverage again. That interpretation remains unverified and was
not silently weakened after failure. Final planner prose also mixed Chinese and Korean; computed metrics,
not the prose's claims, define the recorded outcome.

The artifact revealed a separate explanation defect: the measurement paragraph described `sum` despite
the recipe doing no summation. A public-engine red test reproduced this, and the explanation now reports
only executed operation types plus spatial method/radius, candidate PK, scan/match/comparison counts and
pre-composition pair counts. It explicitly distinguishes conditional nearest from whole-network coverage,
ordinary proximity and wheelchair-route availability, without sending raw row values to the planner.
Scope explanations now call unknown pairs “판정 불가,” including unlisted vocabulary tokens, rather than
mislabeling all of them as missing values. The diagnostic preserves its historical pre-fix explanation;
it was not rewritten as a new live run. Better explanation is **not** resolution of the candidate-domain
or goal-suitability failure. Those still require grounded coverage/interpretation, additional acquisition
and independent expected outputs before M4/M5 or the user's objective can pass.

### Remaining gate

Keep goal state and bounded composition in a shared module. Do not add a graph database to solve missing
delivery contracts or claim the user's final objective is already achieved. The next release gate should
include real cross-domain goals with known source records, required intermediate mappings and explicit
output criteria. Measure goal coverage/completion and false accepted joins separately from retrieval hits.
Standard-dataset and required-role/output contracts now have implementation evidence. The next work is
autonomous use of scoped counterpart/mapping acquisition, stronger identity/time semantics, and broader
real goals with independent expected outputs—not another claim based only on a scripted sample join.
