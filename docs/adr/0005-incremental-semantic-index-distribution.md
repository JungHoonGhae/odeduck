---
status: accepted
date: 2026-09-01
---

# Reuse semantic vectors by document hash and distribute immutable snapshots

## Context

The API + FILE catalogue expanded the local corpus from roughly 12,000 to roughly
96,000 entries. The exact flat index is still fast enough to search, but asking
every user to embed every public document locally is not acceptable installation
work. On the reference Mac Studio, the first full build took 1,064 seconds and
produced a roughly 420MiB index.

The version-1 index had only one digest over `SyncedAt` and the complete catalogue.
A new sync time or one changed entry therefore made every vector stale, even when
the title, organisation, category and description used for embedding were
unchanged.

## Decision

Keep semantic search optional and preserve one small catalogue-search interface.
Change how the index implementation is refreshed and, later, delivered.

1. Semantic index version 2 stores a SHA-256 hash of the canonical embedding
   document beside every PK and vector. The hash covers exactly the text sent to
   the embedder, not demand counters, formats, sync time, or other ranking metadata.
2. `catalog semantic-build` loads the previous local index and reuses a vector only
   when model, document recipe, PK, document hash, and dimensions remain compatible.
   New and changed documents are embedded in bounded batches. Deleted PKs disappear
   from the new snapshot.
3. An exact version-1 index can migrate without re-embedding when its catalogue
   digest matches the current catalogue. Version 1 cannot be partially reused
   across a changed catalogue because it has no document hashes.
4. The completed index is still written atomically. Search accepts the index only
   when its final catalogue digest matches the loaded catalogue. Partial semantic
   coverage is not silently presented as complete search.
5. A future official release should distribute a versioned catalogue and prebuilt
   semantic index together. The delivery format will use an immutable manifest,
   content hashes, verified downloads, atomic activation, and one-generation
   rollback. This release work is intentionally separate from the local v2 format.
6. Keep exact flat cosine search as the correctness baseline. Packed vectors,
   mmap, 256 dimensions, float16, int8, or ANN require measured memory and Korean
   retrieval-quality evidence before replacement.

## Why this seam

The catalogue command calls one refresh interface and does not decide which rows
to reuse. Model compatibility, document hashing, batching, legacy migration, and
atomic persistence stay local to the semantic-index module. A future prebuilt
downloader or packed reader can replace storage behind the same search workflow
without changing MCP's `catalog_search → describe_api → call_api` interface.

The design follows established patterns rather than inventing a mutable index
protocol: document-hash ingestion caches, immutable index segments with atomic
commit points, and content-addressed snapshot downloads. The supporting primary
sources and repository examples are recorded in
[the semantic index distribution research](../research/semantic-index-distribution.md).

## Consequences

- Running `catalog semantic-build` after a normal sync embeds only new or
  semantically changed documents.
- Metadata-only changes no longer trigger expensive embedding work.
- The CLI reports total, reused, and newly embedded counts so fallback cost is
  visible.
- The first installation still requires one full local build until a verified
  prebuilt release exists.
- Version 2 adds roughly 6MiB of PK-aligned document hashes to the current gob
  file.
- Gob still loads and rewrites the complete matrix. A no-change refresh took
  1.9 seconds but reached roughly 2.9GiB RSS, so packed mmap storage remains the
  next performance seam rather than a vector database.

## Verification

- Tests change sync time and ranking metadata while preserving embedding text and
  prove that the vector is reused.
- Tests change one description, add one entry, and remove one entry; only the
  changed and new documents are embedded.
- Tests migrate an exact version-1 snapshot without calling the embedder.
- The live 95,951-entry version-1 index migrated with 95,951 reused vectors and
  zero new embeddings in 1.9 seconds.
- Hybrid search after migration returned the same cross-domain weather and sales
  candidates as the version-1 index.
