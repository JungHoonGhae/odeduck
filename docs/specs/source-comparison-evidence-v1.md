# Computed source-comparison evidence

Status: local calculation, selected support, CLI/MCP regression and fixed-source replay implemented;
independent review fixes verified through `94c6dd1`. Advances I5/I6/I7 through the original G4 source-applicability gap;
it does not replace the original goal, sources, oracle or autonomous completion gates.

## Interface and choice

Use `sample.compare` for a local, support-only computation over two immutable original observations.
Keep the left source's PK and request delivery; omit all acquisition, reduction and spatial selectors.
`compare.left` and `compare.right` each name `observation`, exact `rowsSha256`, and 1–8 `keys`.
Each key names an original field and a rule: `exact`, `trim`, or
`trailing_parenthesized_digits_v1` with an explicit `digits` width (1–32 ASCII digits).
Both sides need the same key arity. Keys stay strings, preserving leading zeroes and tuple boundaries.

`checks` contains 1–64 distinct IDs and left/right `Measure` operands, with unqualified original
fields, empty `as`, explicit numeric format and the same declared unit. Conversion or same-record
`sum_fields` reuse the existing exact decimal rules. They do not infer units, rewrite original fields,
chain aliases or change Composition's measure limit. Recipe bounds precede execution; missing fields,
unknown rules and stale revisions fail without producing a report.
`Measure.as` is optional on the shared JSON schema so comparison operands can omit it; ordinary
output measures still require a nonempty alias at execution. No new arithmetic language is introduced.

Compared alternatives: a separate comparison action/store, and attached Composition comparisons.
Local sample computation reuses the existing derived-observation lifetime, byte/sample budgets,
selection and support binding. It can run even when the final composition cannot yet execute.
The retained-input computation is in-process; inspected external acquisition remains unchanged.
No new dependency interface, graph storage, generic expression language or arbitrary code is needed.

## Computation and provenance

Index every retained row on both sides. Only keys occurring once on each side make pairs. Preserve
every duplicate member as ambiguous; do not choose a winner or form Cartesian products. Missing,
null, non-string, malformed or overlong keys remain unresolved. Unmatched records on either side
remain separate. Never use name similarity or numerical identifier coercion.

For each unique pair and each declared check, count exact equality, inequality, missing/null operand,
or invalid numeric input. Missing and invalid inputs do not enter equality counts and cannot become
zero. Data discrepancies produce a report, including zero-match reports; structural errors,
cancellation or overflow reject the whole computed observation. At most 1000 report rows / 2 MiB,
within existing goal sample/storage budgets; never truncate a successful discrepancy report.
An evaluated numeric operand exceeding the shared result range also rejects the whole report,
even when its opposite operand is missing or invalid; malformed source inputs remain diagnostics.
This includes an expanded intermediate term within `sum_fields`, not just its final sum.

Report rows start with 13 `metric`/`value` summary rows: `leftRows`, `rightRows`, `matchedPairs`,
`leftOnly`, `rightOnly`, `leftUnresolved`, `rightUnresolved`, `checks`, `comparisons`, `equal`,
`different`, `missing`, `invalid`. Their complete counts fit one existing evidence packet. Then
come per-check counts and all unresolved/unmatched/numeric discrepancy records. Summary counters
refer only to the declared retained inputs, not the full source population or independent observations.
The recipe, method version, both request/content/contract/row revisions, unique pair memberships and
diagnostic source positions form provenance. Keep original CSV/XLSX/API addresses reachable through
the immutable source observations. No provider Source Declaration is attached to this local result.

## Disclosure and proposed applicability

The computed Observation uses `DERIVED` delivery and explicit comparison provenance. Values remain
private until existing `read_evidence` selects them. Its origins are `computed_comparison`, never
publisher records. Existing recipient policy, credential guards, 20 rows / 8 fields, 16 KiB packet,
8 packets / 64 KiB session limits apply unchanged.

Explicit `composition.support` may attach disclosed comparison packets to participating original
sources. Each proposed target must receive all 13 summary metrics/values through its bound packets;
selective equality counts without discrepancies/denominators are insufficient support. Before review,
replay the comparison from both retained revisions and verify report/provenance.
Supply only selected computed cells, the recipe, both source metadata/requests and original positions;
undisclosed input values stay local. Existing citation, review-input size, deduplication and expiry
rules apply. Support does not alter the result, required roles, periods, population eligibility or
approval policy. Numeric agreement is evidence to interpret, not automatic definition applicability.
Comparison observations cannot become base/join/reduction/spatial inputs in this support-only version.

### Analysis review projection v3

The complete original G4 comparison input initially used eight packets but serialized to about
107,207 bytes, exceeding the unchanged 96 KiB review limit. The comparison recipe appeared both in
the computed Observation and its SampleRequest (about 9 KiB twice); its left original also appeared
in Artifact.Sources and ComparisonSources. This is a delivery failure, not a model judgement.

Keep immutable observations, requests, packets and execution artifacts unchanged. Project only the
analysis review input. A comparison SourceContext keeps the exact request, with the recipe once in
`request.compare`; `comparison` retains method, both revision records, all pairs and all record origins.
Its projected `source` retains its other metadata but omits the duplicate `source.comparison`.
Before removing that copy, verify recipe equality and the exact request hash.

In left/right order, each `comparisonSources` entry is exactly one of:

- `{artifactSource: "existing observation ID"}`: resolve to Artifact.Sources by ID and its same-index
  Artifact.Requests entry. Verify both projected metadata and request against the immutable original.
- `{source, request}`: inline both when the original does not already participate in the artifact.

For a shared source, union the metadata fields needed by computation/disclosure and by comparison
keys, operands and original selection predicates. AnalysisSource.Fields keeps the computational-use
projection; the union neither discloses extra cells nor changes participation. All declarations,
selection/export conditions, hashes and original CSV/XLSX positions remain reachable. Do not use
an equal PK/content/value to share distinct observation IDs, or hide discrepancies to fit a budget.

Compared a review-wide canonical source registry, a role-specific registry with field-use references,
and this comparison-only projection. Choose the latter: it removes the observed duplication behind
the existing in-process review seam without migrating unrelated source-report/ordinary context
contracts or adding a store, generic graph serializer, action or adapter.

Active analysis replay/result methods and CLI/MCP guides use v3 together. No legacy active fallback
or custom serializer that silently expands the model input. Source-report v1 and historical archives
remain unchanged; explicit archive decoder tests remain response replays, not new model trials.
Detachment, credential checking, full-input hashing, canonical evidence deduplication, fixed recipients,
expiry and all existing budgets still apply after projection.

The complete stored-byte G4 replay now reaches the fixture reviewer in about 91,325 bytes (timestamp
formatting can vary by a few bytes), with eight packets, all prior 157 disclosed cells, 13 comparison
summary rows, 162 pairs/42 checks/6,804 equal positions, and all 15 right-only originals. This proves
delivery and retained-source replay, not new acquisition, independent model accuracy or G4 completion.
School-boundary/applicability evidence and requested final explanations remain open.

## Verification

Use the delegated public Engine Start/Advance/View/PlanningView, CLI and MCP seams. Substitute external
HTTP/model adapters only. Begin with an independently worked cross-domain literal example, then test
duplicate keys, composite keys/leading zeroes, missing/null/malformed operands, exact large decimals,
both unmatched sides, stale revisions, mixed selectors, disclosure, provenance, budgets and no role
promotion. Keep actual G4's 162 original rows, 177 official rows, 42 checks and 15 extra records in
source-specific independent fixtures, not production rules or planning hints. Verify the real product
calculation against the independently preserved source comparison before a new actual model diagnostic.
Fit the original goal's required evidence within existing budgets; this design alone proves neither
that fit nor G4 completion. School-boundary evidence and autonomous held-out completion remain required.
