---
status: accepted
date: 2026-09-01
---

# Broad API + FILE discovery with bounded composition options

## Context

odeduck originally synced only data.go.kr OpenAPI entries. That kept every
search hit close to `inspect_dataset → call_api`, but excluded file datasets that
can provide the other half of a valuable cross-domain comparison. The existing
connection workflow also returned at most three final cards. That precision
limit is useful, but callers could not clearly distinguish “the broader set of
things I could combine” from “the three pairs selected as worth verifying”.

Increasing `maxConnections` would multiply weak cards and model context without
increasing evidence. Building an all-pairs graph would be worse: catalogue
metadata does not prove compatible fields, grain, value overlap, or business
value.

## Decision

Keep one deep catalogue module and the existing three-stage MCP interface.

1. `catalog sync` defaults to `ALL` and sweeps both `API` and `FILE`. `--type
   API` and `--type FILE` remain available for narrower snapshots.
2. Each entry and hit preserves `dataTypes`, publisher formats, and an explicit
   delivery state. Every hit now uses the delivery-neutral
   `nextAction=inspect_dataset`. A FILE hit also has `svcType=FILE` and an
   official data.go.kr `detailUrl`; JSON/XML representation badges remain format
   evidence and are not presented as a `call_api` contract.
3. A structured search with an accepted Anchor returns
   `connectionOptions`: up to three nodes per valid Bridge role. At eight total
   roles, the pool is bounded to at most 21 Bridge nodes.
4. `connections` remains a separate precision surface: at most three explicitly
   selected cards, always in `candidate` state. A caller can select a different
   triple from the same option pool and call `catalog_search` again.
5. Lexical/planned search builds options from complete role buckets rather than
   the displayed page. Hybrid search rebuilds the pool from the bounded fused
   lexical + semantic result, so a useful semantic-only node can become an
   option without semantic similarity proving an edge.
6. Evidence instructions are delivery-aware. API pairs use `inspect_dataset` and
   bounded `call_api` samples. Any pair containing FILE data first verifies the
   official file columns, update date, coverage, and download conditions, then
   profiles a bounded file sample.
7. FILE entries usually have no application count. Ranking therefore uses view
   count as the secondary demand signal and PK as a deterministic final
   tiebreaker; otherwise tens of thousands of zero-application files would have
   snapshot-order-dependent ranking.

## Interface consequences

- Existing callers that use `query`, `concepts`, `hits`, or `connections` keep
  working; all fields are additive.
- `restOnly=true` still means exactly portal-hosted REST and excludes LINK,
  FILE, and unknown entries.
- Older API-only snapshots load without migration. Missing per-entry
  `dataTypes` are conservatively inferred from an existing REST/LINK label.
- Re-syncing an API-only catalogue as ALL invalidates the optional semantic
  index until `semantic-build` refreshes it. Version 2 reuses any compatible
  document vectors and embeds the newly added FILE entries. Lexical/planned
  discovery continues to work without Ollama.

## Rejected alternatives

- **Return more final connection cards.** This confuses possibility with
  evidence and encourages merely novel pairs.
- **Make one MCP tool per file or provider.** This expands the interface with
  the size of the catalogue and defeats progressive disclosure.
- **Treat FILE as an API.** A download page has different access, freshness,
  and invocation semantics. `call_api` must not guess them.
- **Build a global relationship graph now.** Metadata-only edges would encode
  unverified field and grain assumptions, while a value-backed graph requires
  many downloads, approvals, versioning, and stale-data handling.

## Verification

- A live ALL sync on 2026-09-01 completed in 429 seconds and produced 95,956
  unique nodes: 7,132 REST, 4,766 LINK, 84,052 FILE, and 6 unknown. This is a
  point-in-time operational measurement, not a permanent catalogue-size claim.
- A synthetic portal fixture proves that one ALL sync preserves API and FILE
  datasets, service types, data types, and formats in a single snapshot.
- Catalogue tests prove that a displayed page of two hits can still expose two
  alternatives for each Bridge role without creating a connection card.
- Hybrid tests prove that a semantic-only Bridge can enter the option pool while
  the final selected card remains subject to the existing server gates.
- MCP tests prove that `connectionOptions` is returned before explicit Bridge
  selection and that `connections` remains empty at that stage.

## 2026-09-04 graph database reassessment

The catalogue now contains 96,866 API and FILE entries, but the decision still
holds. A graph database would traverse already verified relationships; it would
not replace lexical/planned/semantic retrieval or prove metadata-only edge
hypotheses. The measured bottleneck and the explicit transition gates are
recorded in the
[graph database fit evaluation](../research/graph-database-fit-evaluation-2026-09.md).

Instead of a graph database, odeduck now keeps a bounded local connection
evidence ledger. Only `structurally_verified`, `sample_verified`, `blocked`, and
`rejected` assessments enter it; search `candidate` rows cannot. Records retain
dataset PKs, official provenance, field semantics, aggregate join evidence,
valid/observed time, and supersession links, but not raw response values. This
creates a measured corpus for the graph transition gates without changing the
catalogue retrieval architecture.
