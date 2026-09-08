# Reuse disclosed original cells across observations

Status: design selected, packet feasibility verified; runtime reuse is not implemented.
Continues I5/I7/I9 through the original G4 evidence-delivery gap. The original question,
population contract, source revisions, oracle and autonomous completion gates remain unchanged.
Uses [selected evidence](selected-goal-evidence-v1.md),
[analysis review](goal-source-report-review-v1.md) and ADR-0007/0008.

## Observed gap and choice

The archived G4 age-definition diagnostic used all eight evidence packets. Its school computation
and same-file context disclose 95 distinct original cells in two packets. Those cells fit an existing
19-row, five-field selection of the larger retained observation: the public Engine produces one
11,522-byte packet without losing values, nulls or worksheet positions. This is a disclosure
feasibility result, not a new acquisition or a reviewed G4 result.

Analysis currently reconstructs disclosed inputs only from matching observation IDs and row hashes.
It cannot recognize the same physical cell disclosed through a different immutable observation.
Adding the complete source-comparison summary therefore exceeds the packet count even though all
required school cells fit one packet. Other source-applicability and school-boundary gaps remain.

Compared three designs under `codebase-design`:

- Internal original-cell disclosure resolution behind the existing actions: chosen. Callers select
  actual observations as before; the Engine proves any cross-observation correspondence.
- Explicit Composition evidence bindings: distinguish interpretation from computation, but make
  callers supply another mapping that the Engine must independently verify anyway.
- Consolidate acquisition into one larger rectangle: the current Composition cannot exclude the
  header/total/note rows by a physical range. This changes computational participation or requires
  an additional selection operation unrelated to the observed disclosure problem.

No new action, permission, external adapter, storage service or question-specific production recipe.
Keep acquisition, interpretation support and computational participation separate.

## Original-cell identity

Start with original XLSX observations. Cross-observation correspondence requires matching nonempty
PK, asset, valid content/contract hashes, archive member, exact worksheet member/name, physical row
and column, and the same raw cell interpretation. Validate each observation's own request, rectangle,
row positions and row revision. Different rectangles and retained-row numbering are allowed.
API sample ordinals, documents and computed observations retain exact-observation identity. Other
address kinds need their own supported provenance, not guessed equivalence or numeric key coercion.

Compare the selected packet cell's JSON type, value and presence against the target's retained original
cell in-process. Missing, null, false, zero and empty string stay distinct. A value match at another
address proves nothing. Conflicting values or presence states at one purported original address
reject review with a value-free provenance error; no precedence rule chooses a winner.

Only already-disclosed cells may reconstruct existing participating inputs. A heading elsewhere in
the same file cannot substitute for a required field or row. All contributing/excluded-cell checks,
local calculation replay and original acquisition/full-scope eligibility remain in force. This
correspondence is not entity resolution, definition applicability, time alignment or approval.

## Review representation and lifetime

Use one collection of immutable packet bodies for an analysis review. SourceContext refers to the
packet ID and retains its source metadata, actual request, proposed targets and purpose. Engine-produced
attribution references connect selected packet rows/fields to participating observations and retained
rows. They contain no replacement values. A packet may serve calculation disclosure and interpretation
context without appearing twice. Context-only and all-reused-input reviews must both be representable.

Concentrate packet lookup, original-cell resolution, reconstruction, citation grounding and canonical
review identity in the existing in-process review module. Keep the source-report v1 single-packet
contract; analysis consumers use the new reference representation consistently. Update the review
contract version and active consumers together, replacing the old analysis packet enumeration rather
than maintaining two writable representations. Historical archives keep their original wire shape;
read-only archive tooling must name its legacy decoding explicitly, never rewrite prior trials.

Canonical review deduplication uses original cell identity, presence and value where correspondence
is verified; otherwise it remains observation-bound. Repackaging, observation aliases or row/field
order cannot buy another review of the same execution. Full input hashing still binds requests,
attribution, recipe and interpretation proposals. Actual new cells remain new evidence.

Existing fixed recipients, three review attempts, expiry and all disclosure/review limits remain:
20 rows/eight fields, 2048-byte cells, 16 KiB per packet, eight packets/64 KiB per session, 96 KiB
per review input. Successful disclosures never receive refunds. A new diagnostic may consolidate
its selections upfront; prior packets, failed attempts and archives remain immutable.

## Implementation and verification

The delegated public seams remain Engine Start/Advance/View/PlanningView, CLI/MCP and the reviewer
adapter. Substitute only external acquisition/model dependencies. Begin with a failing overlapping
XLSX disclosure case that uses different retained positions, then implement the shared resolution.
Cover context-only packets, all-reused inputs, missing/null/type conflicts, different revisions,
assets/members/sheets/coordinates, unsupported address kinds, withheld fields, input mutation,
canonical deduplication, cancellation/expiry and unchanged budgets. Verify source-report v1 separately.

Update the shared planning guide and analysis review guide only with the implemented contract, and
verify actual CLI/MCP delivery. Preserve every original G4 selected cell while combining school
disclosure, then add the complete comparison summary and measure actual whole-input size. This
does not by itself provide the still-missing school-boundary evidence or the final explanations.
Independent source expectations, actual model judgement and autonomous completion remain separate.
