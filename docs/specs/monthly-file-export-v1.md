# Inspected monthly FILE export

Status: bounded acquisition implemented; verification in progress. Connects I4/I7/I9 to the original G4 applicability gap;
it does not replace the goal, oracle, independent comparison or autonomous completion gates.

## Interface and choice

Keep `inspect → sample`: an exact portal-advertised external FILE family may expose registered
`exports`. The caller selects a FILE `operation` and its typed `params`; it cannot provide URLs,
headers, HTTP methods, raw form fields or credentials. The private inspection reference authorizes
the operation. Static FILE assets, historical portal versions and supporting documents keep their
existing contracts. An export selection is not a portal historical version.

The initial operation is `mois-monthly-age-csv`. Its required parameters are `month` (`YYYY-MM`),
`registration` (`all`, `resident`, `unknown`, `overseas`), `provinceCode` (ten-digit province code),
`ageFrom` and `ageTo` (inclusive publisher age columns; 100 denotes the publisher's 100+ bin).
One request selects one month and one-year groups. Existing 256-column limits bound a single
observation; at most 83 age columns per sex/total block fit. This is an odeduck limit, not a
publisher limit or permission to drop requested ages. Missing or unsupported choices fail explicitly.

The operation owns complete CSV scanning and province-code selection; it cannot combine with
asset, fileVersion, rowPath, where/whereIn, scanCsv, ZIP/XLSX/layout or local reduction selectors.
Its rows remain publisher strings, including the combined administrative name/code field.
Code extraction used for selection is recorded, not added as a publisher-supplied column.

Compared alternatives were: a new staged export/layout/projection interface, and month selection
at inspection producing a bound Asset. The existing sample operation retains the smallest caller
interface and keeps provider form knowledge in `dataset`. Native age-range export is now observed
to cover G4's requested ages without a generic projection framework or wider scanner. Full-age
export and the prior full-field research comparison remain distinct.

## Acquisition and provenance

Resolve only the exact advertised `https://jumin.mois.go.kr/ageStatMonth.do` family. Submit its
registered selection form, require a successful bounded UTF-8 HTML page, verify selected date,
registration, age conditions and the observed export form/button contract, then POST only to the
registered CSV endpoint. Use credentialless no-redirect transport with the exact source-page Referer.
HTML is bounded to 1 MiB, CSV to the existing 64 MiB stream limit, and the acquisition deadline
is bounded. An HTTP 200 response alone is not a valid form or CSV contract.

Validate the expected month/age/sex header and every CSV record through EOF, including excluded
rows and the tail. Retain at most 1000 province-matching rows / 2 MiB while continuing the scan.
The public form's province selection does not constrain the observed national export: requested
conditions, local trailing-code prefix selection, scanned/matched/returned counts and original
logical/physical positions remain separate. Keep aggregates, zero rows and branch offices as
publisher records; selection is not an aggregation or identity/coverage approval.

CSV provenance carries the inspected export reference, typed request choices, adapter revision,
selected-page hash/bytes, exact non-secret form request, returned columns and code-selection rule.
The observation retains the complete original CSV byte hash and immutable rows. Source documents,
computed comparisons and original rows remain separate evidence kinds. No raw HTML or unselected
values are automatically disclosed to a model. Existing evidence recipients, sample/packet/byte
budgets and review gates remain in force.

## Verification and remaining work

Test the `fetch.Client` public-form transport and `dataset.Inspector` inspection/export interface,
then shared `goalwork.Engine` acquisition and CLI/MCP delivery. Substitute only external HTTP;
selection, parsing, hashing, provenance and disclosure execute in-process. Test exact requests,
cookies/redirects, uninspected references, mixed selectors, reflected-condition/header drift,
malformed excluded tails, row/byte limits and original row positions. Live checks are opt-in and
separate from fixtures and model decisions.

The official request/response and independent numeric comparison are recorded in
[population source applicability](../research/population-source-applicability-2026-09-08.md).
After acquisition, implement a bounded deterministic source comparison with explicit code/field
transformations and both revisions. This computed evidence must not masquerade as a provider
document. Connect it and school-boundary evidence to the original G4 result/review, then test
autonomous discovery independently. Successful export alone is not G4 or product completion.
