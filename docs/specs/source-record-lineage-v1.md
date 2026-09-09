# Source-record lineage v1

Read this contract when changing spatial composition, sums, row-wise measures, role attribution or temporal
alignment of derived observations. Parent contract: [goal-driven composition](goal-driven-composition-v1.md).
Status: implemented observation-scoped execution lineage; not canonical entity resolution or durable provenance.

## Record addresses and field origins

A record address is scoped to an immutable observation. Original CSV values use the source data-record
ordinal AFTER the header, not their position in a retained prefix. API/table rows without CSV positions use
their retained row address. Computed pair values use the pair-row address. These address kinds remain
distinct. Equal values, names or identifier suffixes never define record identity.

| Spatial output field | Original record address | Output-role attribution |
| --- | --- | --- |
| `anchor.<field>` | referenced anchor row, resolved through its CSV position when present | original anchor observation |
| `candidate.<field>` | candidate observation + original full-file CSV data-record ordinal | candidate observation, or its nearest-derived candidate-role container |
| `distance_m`, `rank` | computed spatial pair row | spatial observation; a calculation, not an original source measurement |

Direct and transitive source dependencies are retained in artifacts and used for role participation.
A spatial base already connects two original sources and can be projected/aggregated with no extra join.
Ordinary bases also permit zero joins for projection or analysis; see [goal results](goal-result-execution-v1.md).
Unneeded back joins should not be invented to make a source count or role check pass. Semantic role
interpretation remains unverified. Copied values preserve original scalar states, including CSV empty strings;
a blank cell is not converted to null when constructing a spatial pair.

Before execution, nonempty retained row hashes must match actual rows. Spatial input revisions, pair count,
anchor addresses and candidate full-file addresses must agree with retained source evidence. Missing or
corrupt lineage fails execution; computed distances or copied values cannot repair it.

## Joins, measures and sums

When two join inputs carry an address for the same referenced source, that address must agree. Conflicting
key-matched pairs are excluded and counted in `lineageRejectedPairs`. This preserves an actual source-row
reference when names/coordinates happen to match another row. It is not a new real-world identity inference.
There is no automatic coalescing across separate observations, changed revisions, archives or providers.

`sum_fields` still requires distinct qualified fields within one observation, and now checks that every
field resolves to the same original record kind/source. Anchor and candidate fields in one pair container
are not terms from one source row. Copied source fields cannot be combined with computed pair fields under
that one-record contract. Unit compatibility and disjointness still need separate semantic evidence.

`sum` follows the source address through measure aliases. Distinct original records with equal values
remain additive. Repeating one original record within a sum group fails; it is neither summed twice nor
silently deduplicated. The same record may contribute once to each separate group. Groups are not thereby
certified disjoint or additive across groups. `count` continues to count result rows/non-null fields, not
distinct entities. This replaces the initial blanket ban on summing copied spatial values.

## Time on derived observations

Time bindings reference every participating ORIGINAL observation, including both spatial parents. They
use original field names and the existing reference/event/validity formats; the spatial container cannot
stand in for both source periods. Anchor dates use the referenced anchor rows. Candidate dates use actual
selected pair copies, which may come from far beyond the candidate observation's retained prefix. A
candidate-only prefix provides observed schema/revision, not substitute dates for those selected records.

The engine intersects both source periods and the requested window before a spatial result can contribute.
A zero-join projection applies this too, emitting a `base_temporal` metric with rejected/unknown pair counts.
Unknown or disjoint periods cannot pass; malformed dates fail. Field meanings remain `meaningVerified:false`.

These checks occur AFTER nearest selection. Dropping an ineligible pair does not refill ranks or establish
the nearest records among all time-eligible candidates. Such a claim needs a new full scan with grounded
candidate eligibility. Reports preserve this limitation and the original selection evidence.

## Evidence and remaining work

Public-engine tests cover equal-valued distinct records, repeated anchors/candidates, separate groups,
same-name back-join collisions, wrong prefix positions, mixed-origin measures, original-role attribution,
zero-join output, corrupted revisions/addresses and actual selected-record dates. MCP exercises failed
lineage joins followed by a successful source-aware sum. The frozen G2 live replay still matches 12 source
records/distances and can now project the spatial relation directly without an artificial back join.

Raw samples/artifacts remain excluded from external CLI planning. Explicitly authorized
[selected evidence](selected-goal-evidence-v1.md) can include bounded source values with these original
addresses; it does not confer meaning or identity approval. No graph database or canonical entity IDs were added.
Cross-observation identity, persistent lineage/ledger reuse, source-wide aggregation, temporal eligibility
before nearest ranking, semantic approval and unseeded goal completion remain separate requirements.
