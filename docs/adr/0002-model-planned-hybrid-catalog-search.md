---
status: accepted
---

# Model-planned hybrid retrieval, with an optional flat local vector index

The catalogue has roughly twelve thousand OpenAPI entries. Literal search works
when a user already knows the government's vocabulary, but it fails at the more
valuable questions: “what could help me earn money?”, “what service should I
build?”, or “what is changing in Korea?”. Splitting those sentences into words
produced thousands of false matches (`벌` matched 갯벌 and 벌채), while a fixed
synonym/opportunity dictionary could only encode opportunities we already knew.

## Decision

`catalog_search` combines three layers behind the same first-stage tool:

1. The MCP host model preserves the user's original intent and decomposes broad
   goals into 2–8 concrete, diverse data axes. This is semantic query planning,
   not a hard-coded industry taxonomy. For standalone use, `catalog discover`
   obtains the same small plan from an already-authenticated Codex, Claude Code,
   Gemini CLI or Cursor Agent process. It does not read agent credentials.
2. odeduck searches every axis deterministically across title, organisation,
   category and official description, then round-robins the axes so a prolific
   publisher cannot occupy the whole first page.
3. When an optional Ollama index exists, odeduck embeds the intent and axes,
   performs local cosine retrieval, and fuses it with lexical ranks. Failure or
   absence falls back to layers 1–2 and is reported in `semantic.status`.
4. Hybrid ranking protects the leading strict lexical match for each explicit
   axis before semantic expansion. This keeps actionable objects such as an
   exact support or tender notice from being displaced by merely similar history.

The default local provider is Ollama with
`embeddinggemma:300m-qat-q4_0`. Documents use EmbeddingGemma's documented
`title: ... | text: ...` form; queries use `task: search result | query: ...`.
Official description previews are returned only for exploratory/planned search,
so known-item lookup stays compact.

ACP is not used as the planner boundary. ACP connects an editor/client to an
agent session, while MCP connects that agent to odeduck's tools. Codex, Claude,
Gemini and Cursor can therefore host `odeduck mcp` directly. The CLI adapters
exist only for standalone `catalog discover`, where no host agent is present.

## Why no vector database yet

11,902 × 768 float32 values are about 35 MB of vector payload (about 51 MB in
the persisted Go index). A flat dot-product scan is fast enough: on the reference
development machine, a cold Ollama search took 1.30 seconds and a warm search
about 0.3 seconds. Adding a vector database now would impose another install,
process, schema and failure mode without a user-visible retrieval benefit.

The embedding provider and `SemanticIndex` are separate boundaries. Move to an
ANN/vector-database implementation when the corpus grows by orders of magnitude,
when concurrent service traffic makes flat scans material, or when server-side
metadata filtering is needed. That future change must not alter the MCP
`catalog_search → describe_api → call_api` contract.

## Consequences

- The normal MCP path needs no nested model or Ollama process: the host model
  already supplies the plan. Standalone `catalog search` also remains local and
  deterministic; only explicit `catalog discover` sends the stated goal to the
  selected agent CLI using that CLI's configured authentication and billing.
- Ollama is optional and free/local; no catalogue text or query leaves the
  machine through odeduck's default semantic provider.
- `odeduck catalog semantic-build` downloads the model and builds the cache in
  one command. After a re-sync, version 2 reuses vectors whose model, recipe and
  document hash still match, then atomically saves an index for the new snapshot.
- Search results expose `matchedQuery`, short previews and semantic status so an
  agent can explain why an unexpected candidate appeared instead of presenting
  opaque vector similarity as fact.
- Vector similarity improves recall but does not decide whether an API is useful.
  The agent must still inspect the official specification with `describe_api`
  before applying or calling.
