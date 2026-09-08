# Changelog

All notable changes to odeduck are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/); versions follow
[SemVer](https://semver.org/). The release workflow uses the `## [X.Y.Z]`
section matching a `vX.Y.Z` tag as the GitHub release notes.

## [Unreleased]

### Added

- Local `sample.compare` through the shared CLI/MCP goal engine compares explicit
  keys and numeric measures from two retained original revisions. Exact decimal
  checks preserve duplicates, discrepancies and both unmatched sides. Selected
  computed summaries can support interpretation review without replacing source
  records, changing result roles or approving identity, applicability or coverage.

- Registered monthly FILE export through the shared CLI/MCP goal `sample` path,
  with typed period/registration/province/age choices, credentialless no-redirect
  form POSTs and complete CSV scan provenance. Publisher rows and selected-age
  columns stay original; source applicability and goal completion remain separate.

- Registered supporting HTML documents in FILE inspection and goal `sample`
  (`delivery:document`), with credentialless exact-origin acquisition, selected
  original DOM addresses and page/text hashes. Explicit evidence disclosure and
  `composition.support` carry document context into review; documents cannot
  become computational data or certify applicability or population coverage.

- Additional opt-in full-scope goal review (`solve --review-full-scope`, MCP startup
  `--review-goal-full-scope`), requiring existing analysis/disclosure authorization.
  Complete CSV retention or exact XLSX rectangle acquisition enables review, not
  population approval. Every original source needs a separately cited scope finding;
  missing evidence, truncated selections and unsupported acquisition types remain incomplete.

- Optional `composition.support` links already-disclosed evidence from another
  original observation to proposed interpretation targets in analysis review.
  Exact packets and acquisition provenance are preserved; support does not change
  calculations, satisfy required roles or certify applicability or population scope.

- Bounded portal FILE history discovery and exact edition selection through CLI,
  MCP and goal inspection. Historical requests retain edition-specific metadata
  and acquisition provenance, reject latest-file fallback and preserve old
  observations across reinspection. Listing versions grants no ledger verification.

- Optional `composition.reportUnmatched` projects selected excluded source fields
  into a separate goal result table, retaining stage-local record addresses and
  missing/null distinctions. It shares result limits, does not change calculations,
  and requires selected evidence before analysis review.

- Already-disclosed same-file header/context evidence in opt-in goal analysis
  review, pinned to source content and contract revisions. Context keeps its
  selected cells, original addresses and acquisition request separate from
  computational sources; file association is not proof of meaning or identity.

- Stage-local unmatched record addresses in goal join metrics, including empty
  joins, for bounded evidence reading and replanning. Relational review now also
  requires disclosed comparison fields for excluded direct records and replays
  exclusion metrics; unrelated excluded values remain private.

- Bounded exact value-set selection (`sample.whereIn`) for full direct CSV scans,
  preserving source record positions and separate scanned/matched/retained coverage.
  Selection order cannot bypass acquisition replay limits, and both review kinds
  require selected evidence for predicate fields. Development calibration can
  now acquire inspected sources and check unchanged independent records before review.

- Additional opt-in review of typed relational calculations via CLI
  `--review-analyses` or MCP startup `--review-goal-analyses`. Local replay and
  original record participation bind already-disclosed result values; separate
  relation, period, measurement and coverage findings accompany goal/output checks.
  Source-report authority does not enable this scope. Review budgets and canonical
  evidence deduplication are shared; spatial, causal and hypothesis approval remain unsupported.

- Source-grain aggregation of retained observations before cross-source joins via
  goal `sample.reduce`. Exact sums, contributing-record groups, source revisions,
  original-period checks and shared budgets survive CLI/MCP execution. Computed
  groups remain distinct from publisher records and do not acquire source-report approval.

- Opt-in, separately invoked model review for bounded source-field reports via
  CLI `--review-source-reports` or MCP startup `--review-goals-with`. Original-goal
  fit and every output's source support are assessed separately against already
  authorized evidence; qualified completion carries reviewer/method/input/execution
  provenance. This is not field verification or approval of joins, calculations,
  population claims or hypotheses. Default-off disclosure and three-review limits apply.

- Native STD inspection through CLI/MCP, plus bounded CSV, ZIP-member and exact
  XLSX-rectangle readers with source hashes, original positions and explicit limits.
- Optional, recipient-bound selected evidence for experimental goal planning:
  CLI `--share-evidence` requires one explicit agent; MCP disclosure is fixed by
  server startup `--share-goal-evidence`. Original record addresses, exact values,
  missing/null states, cumulative disclosure budgets and session expiry are retained.
  Default planning remains value-free; evidence access is not semantic approval.
- Experimental shared goal-based discovery and bounded composition via CLI `solve`
  and MCP `advance_goal`, with required-role/output checks, source-record lineage,
  typed FILE/STD acquisition, explicit retries and versioned scope-label comparison.
  Candidate artifacts remain distinct from goal completion; general semantic
  approval and durable goal resumption are not implemented.

### Fixed

- Continue experimental goal planning after `review_required` within the original
  contract, disclosure policy, budgets and expiry. Pin evaluations to execution
  revisions and clear stale current results before a replacement execution.
  Restarting the planning loop no longer refills its one replay correction.
- Share semantic-search policy across CLI and MCP, keeping degraded retrieval
  visible and preserving the connection limit through progressive discovery.
- Strengthen credential isolation at the shared REST and LINK response boundaries;
  withhold unsafe responses without creating replacement source observations.
- Preserve empty CSV strings instead of inventing nulls; keep STD reads bound to
  the first-party response and reject ambiguous JSON object members.
- Allow ordinary single-source projection and aggregation through the shared goal
  executor; remove the spatial-only zero-join restriction and unexecuted join explanations.
- Rename the experimental goal artifact state from `sample_joined` to `sample_executed`
  for all current operations. Historical diagnostics retain their original state;
  this does not change semantic approval or CLI completion requirements.
- Reject lossy Unicode decoding in native STD schema/row JSON before retaining
  source identifiers. Preserve valid replacement characters and surrogate pairs.
- Describe only executed key/measurement operations and preserve the conditional
  source coverage of nearest-record calculations in goal evidence explanations.
- Preserve original CSV empty strings in nearest-derived evidence and artifacts.

### Changed

- Share one engine-owned goal planning guide across CLI prompts and the MCP
  resource; remove duplicate operator instructions and obsolete join requirements.

## [0.18.0] - 2026-09-05

### Added

- Record evidence for cross-domain connections in a local, append-only ledger,
  including the datasets, identifier fields, provenance, sample overlap and join
  expansion used to support a decision.
- Verify sample claims against recent `call_api` profiles in the same MCP session,
  with delivery-, operation- and request-bound receipts that never persist raw
  response values.

### Changed

- Require explicit expected-key mappings, consistent count aggregates, safe
  data.go.kr provenance URLs and delivery-compatible evidence records.
- Keep read-only empty-ledger queries side-effect free and bound receipt memory;
  preserve ambiguous or damaged JSONL tails instead of deleting evidence.

### Fixed

- Prevent fabricated sample metrics, credential-bearing provenance and
  whitespace or ordering variants from bypassing validation or idempotency.
- Make ledger locking, concurrent append, cancellation, cross-platform builds
  and profile evidence hashing deterministic and test-covered.

## [0.17.2] - 2026-09-04

### Changed

- Adopted the stable black-glasses Odeduck logo and regenerated the social and
  launch artwork from the same source.
- Shortened the README around the product problem, a concrete cross-domain
  example, quick start, measured proof and safety boundaries.
- Kept the original no-glasses cabinet animation as the README hero and moved
  experimental video variants out of the public assets.

### Fixed

- Updated the README, launch kit and installer examples to point to v0.17.2.

## [0.17.1] - 2026-09-04

### Fixed

- Align the deterministic lexical release gate with its documented contract:
  retain exact and cross-domain term checks, while leaving broad intent such as
  “cheap investment property” to agent-planned or semantic discovery instead of
  assuming an undocumented synonym mapping.

## [0.17.0] - 2026-09-04

### Changed

- Introduced 오데덕 as the product character and renamed the CLI, MCP server,
  repository, Go module, configuration root and release assets to `odeduck`.
- Removed unreleased compatibility binaries and legacy identity fallbacks so
  installations expose one unambiguous command.
- Extended `docs/brand/brand.json` as the canonical source for the display name,
  command, repository, module path and configuration directory, with CI checks
  for identity drift.
- Retry transient transport failures while resolving the official monthly
  catalogue download contract, preventing a single portal dial timeout from
  aborting an otherwise valid release.
- Rewrote the opening story around finding a missing sock by looking across
  everyday contexts, then carried that behavior into cross-domain public-data
  discovery and evidence notes.
- Credited Steve Jobs's 2005 Stanford commencement address for the
  “connecting the dots” framing and separated that source from 오데덕's
  evidence-first application of it.

## [0.16.2] - 2026-09-04

### Added

- Added an opt-in high-trust discovery mode that requires semantic retrieval and
  fails with an actionable recovery hint instead of silently relying on keyword
  fallback results.
- Added bounded XLSX worksheet inspection, including shared and inline strings,
  merged multi-row headers and schema evidence for FILE datasets.

### Changed

- Made geographic title relevance win over description noise even when results are
  ranked by recency, improving cross-domain discovery for regional questions.
- Reframed the product story around finding the missing context across unrelated
  public-data domains, with a sourced before-and-after workflow diagram and a
  reproducible four-dataset Jeju example that keeps unverified joins explicit.

### Security

- Stream and bound XLSX workbook, relationship, worksheet, merge and shared-string
  metadata so crafted public files cannot amplify into unbounded memory or CPU work.
- Corrected release configuration ownership coverage for the GoReleaser file and
  repository rulesets.

## [0.16.1] - 2026-09-03

### Changed

- Reframed the README around the everyday problem of testing a small-business idea
  without knowing official public-data terms, while preserving the doorway character
  and existing brand mark.
- Clarified where existing public-data MCPs still hand work back to people and showed
  the real REST, FILE and LINK paths without implying that oddsock makes the final
  business decision.
- Added a reproducible, no-login first-run example plus GitHub and LinkedIn launch
  artwork that preserves the existing character mark.
- Made the macOS/Linux and Windows installers fall back to public GitHub downloads
  when the GitHub CLI is absent or unauthenticated, while retaining private-preview
  support for authenticated contributors.
- Added a current architecture map and a manually dispatched anonymous launch smoke
  test that verifies the public repository, release assets, installer, catalogue and
  first no-login search before promotional links are distributed.
- Replaced the stale pre-rename video brief with the current rainy-season cafe demo
  and marked its old Higgsfield job IDs and render as non-publishable records.

### Security

- Added a redacted full-history Gitleaks check to CI, with exact-fingerprint ignores
  for reviewed test fixtures and protocol examples so new findings still fail.
- Ship checksummed installer scripts as versioned release assets, with hermetic
  fallback and checksum-rejection tests on Linux and Windows.

### Community

- Added contribution, security and conduct policies, structured issue forms, a
  pull-request checklist, funding metadata and monthly dependency updates.
- Removed paused local-tracker scratch documents from the public tree; their history
  remains recoverable in Git while GitHub Issues stays the canonical tracker.

## [0.16.0] - 2026-09-02

### Changed

- Reframed the README around oddsock's quiet doorway character and a single-line
  natural-language question, with concise proof points for discovery, inspection,
  application and first-call workflows.
- Replaced the meerkat mark with a transparent doorway-character SVG and matching
  2048px PNG, including a light keyline that stays legible on dark GitHub themes.
- Restored authenticated private-repository installation, source-install catalogue
  bootstrap and legacy Homebrew/Windows upgrade guidance.

### Security

- Documented when standalone discovery sends a goal to a logged-in Codex, Claude,
  Gemini or Cursor process, including sensitive-input and untrusted-metadata bounds.

## [0.15.0] - 2026-09-02

### Changed

- Renamed the canonical repository, Go module, CLI binary, release archive,
  configuration root, MCP server and guide URI to `oddsock`.
- Kept `opendatactl` and `gongctl` as deprecated compatibility binaries. The
  canonical command reuses either former configuration root when it contains
  login state, keys or a catalogue, and logout cleans all three roots.
- Installers now prefer `ODDSOCK_*` settings and oddsock release assets, then
  fall back through the former `OPENDATACTL_*` and `GONGCTL_*` contracts for
  version-pinned upgrades.
- Pull-request CI now validates the GoReleaser configuration and builds only the
  native runner target. Full six-target packaging remains a mandatory release
  gate, avoiding 18 redundant cross-builds on every ordinary change.

## [0.14.0] - 2026-09-02

### Added

- Added one delivery-neutral `inspect_dataset` workflow for REST, LINK and FILE
  nodes. FILE inspection can resolve provider assets and observe bounded CSV/DBF
  schemas; API inspection now exposes fact-level provenance.
- Release builds now publish a checksummed official catalogue snapshot that the
  macOS/Linux and Windows installers validate and install atomically.
- Public monthly catalogue streaming and a release-only official-file+web
  composite make the prebuilt reproducible without an institutional API key.
- A reviewed release-search golden set now protects representative direct and
  natural-language discoveries from catalogue or ranking regressions.
- Added the `oddsock` public brand and one-sock meerkat mark. Brand name,
  tagline and asset path are generated from one checked configuration while the
  `opendatactl` command and compatibility contracts remain unchanged.

### Changed

- Catalogue sync now prefers the documented data.go.kr bulk API and joins its
  logical dataset, OpenAPI-operation and file-version lists. An explicit web
  Adapter remains available when the bulk API is not approved.
- Official snapshots retain REST/LINK operations, request-variable names,
  approval modes and FILE co-representations. Portal HTML is a labelled fallback
  only for facts the machine interfaces omit, such as required/sample parameter
  details and concrete file asset identifiers.
- Every catalogue hit now points to `inspect_dataset`, preserving the compact
  search → inspect → call progression across delivery types.
- API+FILE co-representations now expose both contracts by default, with an
  explicit `delivery=api|file` selector and a FILE detail handoff even when API
  remains the primary service type.
- The redesigned combined API+FILE page, referenced official ODCloud Swagger,
  and the portal's current individual-account application redirect are now
  supported.
- Automatic refreshes preserve a broader existing composite snapshot when an
  upstream fallback would silently replace it with a narrower catalogue.
- FILE observation streams GET and POST assets through bounded temporary files,
  avoiding the ordinary 32 MiB in-memory response limit while retaining archive
  entry, expanded-size and total-download guards.
- Composite snapshot builds retry bounded transient network timeouts at each
  source while parser/schema drift still fails immediately and loudly.

### Safety

- Release coverage gates require API, FILE, REST, LINK and official-contract
  minimums, preserve at least 50,000 API+FILE alternatives, and cap unknown
  types; a high total row count alone cannot pass.
- Prebuilt installation rejects malformed/non-official snapshots and upgrades a
  newer but narrower web/API-only local snapshot while preserving a newer,
  equally complete official snapshot.
- Bulk-only catalogue metadata cannot trigger a credentialed API call until a
  detailed first-party or official Swagger contract proves the invocation;
  cancellation is propagated and large FILE streams use a bounded ten-minute
  timeout instead of the short metadata timeout.
- Account-wide catalogue credentials are pinned to the documented
  `https://api.odcloud.kr` origin. Test portal overrides cannot receive them and
  credentialed catalogue requests never follow redirects.
- Installers checksum and structurally preflight the optional prebuilt catalogue
  with the staged binary before replacing an installed binary.
- The release baseline now uses Go 1.26.6 and patched `x/net`/`x/text`
  dependencies; CI and tag builds run `govulncheck` before packaging.

## [0.13.0] - 2026-09-01

### Added

- Discovery now indexes both OpenAPI and FILE catalogues by default, expanding
  the locally searchable snapshot to 95,951 current portal entries. FILE hits
  expose their official detail page, declared formats, available
  representations, view demand and a capability-specific next action without
  pretending they are immediately callable APIs.
- Planned and hybrid discovery return a bounded `connectionOptions` pool with up
  to three real datasets per complementary role. Agents can compare alternative
  cross-domain combinations before explicitly selecting the small set of
  candidate connection cards that merit schema and value-level verification.
- Added reproducible broad-catalog, semantic-search and distribution research,
  including measured full-build and incremental-refresh resource costs for the
  current 95k-entry catalogue.

### Changed

- `catalog sync` now defaults to `ALL`; `--type API` and `--type FILE` remain
  available for deliberately narrower snapshots. `--if-stale` upgrades a fresh
  legacy API-only snapshot instead of incorrectly skipping the first ALL sync.
- Semantic index format v2 stores per-document hashes, model dimensions and an
  embedding recipe version. `semantic-build` reuses unchanged vectors, migrates
  an exact v1 index without embedding, and automatically rebuilds incompatible
  or changed indexes instead of requiring manual cache deletion.
- Hybrid retrieval builds connection choices from its expanded fused candidate
  pool while keeping the visible result page and final connection cards bounded.
- Planned and semantic retrieval still score every eligible catalogue row but
  retain only the exact top-ranked candidates as `Hit` values. This removes
  per-axis 95k-result arrays without changing exhaustive top-K ordering.

### Fixed

- API and FILE representations sharing a portal PK are merged without losing
  the more specific REST/LINK contract, file formats, demand signals or either
  representation label.
- FILE popularity is ranked by portal views when utilization-application counts
  are unavailable, preventing useful download datasets from clustering at an
  artificial zero-demand tie.
- Semantic cache loading rejects oversized, structurally inconsistent,
  dimension-mismatched and non-finite vector data before it can be reused.

### Safety

- FILE discovery remains inspection-only: `call_api` is not advertised until a
  real invocation contract exists. Connection options remain explicitly
  unverified and require column, coverage, grain and sampled-key evidence before
  a join or business claim can be promoted.

## [0.12.0] - 2026-09-01

### Added

- Added a real LINK provider adapter seam and evidence-backed reference
  implementations for SafetyKorea, VWorld, FoodSafetyKorea and Seoul Open Data
  Plaza. Together they cover the three providers found in 56% of the top-50
  demand sample plus the product-safety reference contract.
- Contracts now expose a stable `adapterId`, integer `adapterRevision` and, when
  proven by the publisher URL, `providerServiceId`. Provider documentation
  version and `verifiedAt` remain separate so source updates and matcher changes
  are not conflated.
- Added 11 live canaries across legacy/current URL shapes and Seoul's separate
  general/metro credential scopes. `doctor --adapters-only` runs these checks
  without login or a browser.
- Added a weekly GitHub Actions drift monitor that opens or updates a canonical
  issue when a canary changes or a contract exceeds the 180-day verification
  window, plus an adapter contribution/maintenance guide and official-source
  research record.
- Added provider-scoped credential lifecycle commands:
  `provider-key set|status|delete`. Secrets are accepted through hidden terminal
  input or stdin, never as command arguments or MCP fields.
- Added typed LINK invocation behind the existing `call --pk` and MCP `call_api`
  surface: SafetyKorea's five certification/recall operations,
  FoodSafetyKorea's service-specific official parameter-table inspector, and
  VWorld data, address, search, WMS and WFS families.
- Added copyable matcher, typed-operation registry and fail-closed contract-test
  templates, plus a fixed before/after search evaluation covering eight
  natural-language scenarios and the 50 highest-demand LINK datasets.

### Changed

- Known-provider matching now uses exact official origins, path families,
  allowlisted single-value queries and validated provider service IDs. Similar
  hosts, apex redirects, unknown paths, unknown Seoul OA identifiers and
  ambiguous queries fall back to `inspection_required`.
- Credential scopes now identify the actual provider API prefix rather than a
  broad host. Seoul general and real-time subway keys are classified separately;
  both remain non-invokable because the official endpoints are still HTTP.
- The provider registry remains inside the single Go module while adapters share
  one binary and release cycle. Its one-method interface and fixture contract
  preserve an extraction seam if independent packages or runtimes are needed.
- `catalog search`, `catalog discover` and MCP `catalog_search` now include LINK
  datasets by default so 40% of the catalogue is not silently hidden. Use
  `--rest-only` or `restOnly=true` only for an explicit portal-REST-only search.
- CLI and MCP now share one dataset caller: it describes a PK once, validates the
  advertised operation, resolves only the matching credential scope, and
  dispatches REST or an implemented external provider without accepting raw
  external endpoints.
- Distribution now matches the repository's private visibility: install scripts
  use an authenticated GitHub CLI session, releases remain private, and public
  Homebrew tap publishing is paused instead of emitting unusable private URLs.

### Fixed

- `describe` now surfaces every VWorld parameter accepted by the typed caller;
  provider operation metadata and request validation are generated from one
  registry so they cannot drift into contradictory schemas.
- VWorld 2D contracts bind nine reviewed guide service IDs to their exact
  published `data` codes and inject them automatically. Unknown service IDs are
  non-invokable, and the provider's not-yet-available `GetFeatureType` operation
  is no longer advertised.
- `logout` attempts every credential cleanup even when portal cleanup or one
  provider file fails, then reports the combined errors instead of silently
  leaving later provider keys on disk.
- Known LINK contracts now give capability-specific next actions: request access
  for implemented callers, use the official provider directly when no caller is
  implemented, and choose another dataset when credential transport is unsafe.
- Repeated FoodSafetyKorea calls reuse a bounded five-minute, revision-aware
  request-schema cache instead of downloading and parsing the same official
  parameter table before every data request.
- External HTTP-200 responses now require provider-specific success envelopes;
  FoodSafetyKorea/VWorld XML errors and OGC exception documents can no longer be
  mistaken for successful calls.
- `logout` clears provider keys and interrupted atomic-write fragments from both
  current and `gongctl` compatibility roots. Provider files use a protected
  current-user/SYSTEM DACL on Windows instead of ineffective POSIX mode bits.

### Safety

- External provider calls require exact HTTPS hosts and path prefixes, reject
  redirects, bound response size and request values, validate provider-specific
  operations/enums/page limits, and interpret errors embedded in HTTP 200 bodies.
- Header, query and path credentials are redacted in raw and escaped forms.
  Provider credential files are atomically written as Unix `0600` or a protected
  current-user/SYSTEM Windows DACL; `logout` clears them with the data.go.kr
  session state.
- Seoul general and real-time subway contracts remain discoverable but are
  `blocked_insecure_transport` because their official invocation endpoints do
  not currently provide verified HTTPS.

## [0.11.0] - 2026-09-01

### Added

- **LINK datasets now lead somewhere actionable.** `describe` and
  `describe_api` resolve the publisher starting point hidden behind the current
  data.go.kr button and return a structured handoff instead of an unexplained
  empty specification.
- Handoffs expose their provider host, trust and safe-fetch policy, current
  state, next action, and a structured failure when the portal lookup cannot be
  completed. The first evidence-backed provider contract covers SafetyKorea's
  product-safety OpenAPI documentation, separate application, and scoped
  `AuthKey` requirement without claiming that invocation is implemented.
- `doctor` now has an independent LINK canary that detects portal resolver
  drift, provider-contract downgrade, and contracts that have not been
  re-verified within 180 days.

### Changed

- LINK remains inside the compact `catalog_search → describe_api → call_api`
  workflow, but it is now an explicit external-provider branch. LINK responses
  never expose call-shaped operations, and both CLI and MCP invocation reject
  them until a provider adapter is deliberately implemented.
- Known provider contracts match an exact HTTPS origin and verified wire path.
  Unknown hosts and unverified paths stay discoverable with
  `inspection_required` rather than receiving an inferred contract.
- Publisher pages are marked untrusted and require a fetcher that validates DNS
  destinations and every redirect hop. Agents are instructed to stop when no
  such fetcher is available and to treat external page content as data, not
  instructions.

### Fixed

- Restored LINK URL resolution after the KRDS redesign moved the target from a
  labelled HTML row to the portal's `selectApiLinkUrl.do` lookup.
- The portal lookup no longer follows redirects, and handoffs reject obvious
  local, private, link-local, credential-bearing, non-HTTP, and ambiguous
  numeric-literal targets before surfacing a URL.

## [0.10.0] - 2026-08-31

### Added

- **Bounded cross-domain connection discovery.** `catalog discover` and MCP
  `catalog_search` can now separate a natural-language goal into unique data
  roles, inspect actual first-pass results, retrieve complementary Bridge nodes,
  and return at most three explicitly selected Anchor→Bridge candidates.
- Connection cards explain the proposed edge kind and keys, incremental value,
  metadata evidence, claim boundary, and evidence still required. Search never
  labels a connection verified and returns an explicit abstention when no
  selection passes the contract.
- `call --profile-field` and MCP `call_api.profileFields` return bounded raw-value,
  path, null, distinct, and duplicate evidence for up to eight response fields.
  This helps a host compare common slices without storing API responses or
  pretending that a field-name match proves a join.

### Changed

- Connection discovery uses a post-retrieval precision gate: a model must choose
  a real PK from the current result page and explain why its metadata supports
  the candidacy. The server binds role, incremental value, edge kind, expected
  keys, and proxy transform to the retrieved hit instead of trusting the selector
  to rewrite them. Search rank alone can no longer create a card.
- The MCP surface keeps the existing search → describe → call tool types. New
  structured `axes`, `anchorPks`, `bridgeSelections`, and `maxConnections` fields
  are additive, while existing `concepts` callers keep their previous behavior.
- Discovery roles and cards are capped and deduplicated; `maxConnections` is now
  honestly bounded to three. Optional Ollama remains a recall aid rather than a
  connection-verification authority.

### Safety

- Bridge selections outside the current hits, selections without metadata
  evidence, duplicate roles, and incomplete join hypotheses are rejected.
- Anchor selections must also be present in the current result under the
  original anchor role; an arbitrary catalog PK cannot be relabelled as one.
- JSON response profiling preserves integers larger than JavaScript's safe
  integer range, avoiding silent corruption of numeric identifiers.
- Profiling no longer merges identically named leaves from different response
  paths, and scalar arrays are counted under their owning field.
- Post-retrieval agent prompts mark publisher metadata as untrusted, disable
  provider tools, use an empty workspace, and receive a minimal environment
  instead of inheriting unrelated process secrets. Cursor remains available for
  initial planning but abstains before untrusted metadata because its current
  ask-mode sandbox still allows reads outside the workspace.
- Candidate cards state coverage limitations and keep business success,
  causality, schema compatibility, and sampled value overlap outside the search
  layer's claims.

## [0.9.0]

### Added

- **OpenDataCTL is the new product and command name.** The name spells out the
  product's job: an AI control plane for Korean open data, connecting natural-
  language discovery, specification inspection, utilization application,
  approval status, credential injection, and the first live API call.
- Release archives contain both `opendatactl` and a `gongctl` compatibility
  binary. Legacy `gongctl_*` archive names also remain available for saved v0.8
  macOS/Linux installers and explicitly version-pinned Windows installers. An
  unpinned saved v0.8 Windows script must be replaced by the current installer.
  The old `GONGCTL_VERSION` and `GONGCTL_OLLAMA_URL`
  environment variables and `gongctl://guide` MCP resource remain supported
  during the transition.

### Changed

- The Go module, GitHub repository references, release archives, installer,
  Homebrew cask, MCP implementation identity, user agent, command help, and
  current documentation now use `github.com/JungHoonGhae/opendatactl` and the
  `OpenDataCTL` brand.
- Fresh installations store state in the operating system's standard
  `opendatactl` configuration directory. Existing users continue using their
  `gongctl` directory in place, so login cookies, API keys, catalogues, and
  semantic indexes are neither copied nor lost.

### Migration

- Replace `gongctl` with `opendatactl` in shell scripts and MCP configuration.
  The old command remains functional in v0.9, but new integrations should use
  `opendatactl`.
- Go installations must use
  `go install github.com/JungHoonGhae/opendatactl/cmd/opendatactl@latest` because
  the module path follows the renamed repository.

## [0.8.0]

### Added

- **Goal-based public-data discovery** with `catalog discover` and MCP
  `catalog_search`. Codex, Claude Code, Gemini CLI, or Cursor Agent can turn an
  everyday goal into distinct opportunity axes; deterministic catalogue search
  then explores all 11,902 OpenAPIs without sending the catalogue to the model.
  Optional Ollama/EmbeddingGemma vectors recover differently worded candidates
  without making a model server or vector database mandatory.
- **A progressive agent workflow that closes the access gap:**
  `catalog_search → describe_api → (apply when needed) → call_api`. The application
  tool is now presented as the explicit 2.5-stage bridge rather than a buried
  account helper, so an agent can inspect approval terms, submit the real portal
  form, confirm approval, reuse the account key, and make the first live call.
- **Reproducible search and competitor research.** Ten natural-language queries
  improved top-10 precision from 0.36 lexical to 0.78 with model-planned hybrid
  search, while opportunity-axis diversity rose from 1.2 to 3.7. A primary-source
  audit of public alternatives records the defensible positioning: search/detail/
  call MCPs exist, but the surveyed implementations leave utilization application
  and approval to the user.
- **Sliding session refresh.** Authenticated HTTP reads use a cookie jar across
  redirects and persist a rotated data.go.kr session only after the requested
  account page is verified as authenticated. Active use can therefore extend a
  portal session without leaving Chrome open; absolute SSO expiry still requires
  `gongctl login`.

### Changed

- **The 2026-08 data.go.kr redesign is supported end to end.** KRDS search results,
  account application lists, multi-operation detail pages, approval rows, and the
  application success response have fixture-backed parsers. Four new APIs were
  submitted, automatically approved, and called on a real account: Onbid bid
  results, Nara procurement notices, wholesale-market auctions, and SME support
  announcements.
- The README now leads with the four user bottlenecks gongctl removes: exact keyword
  guessing, manual specification triage, portal application/approval work, and
  hand-written authenticated calls. Integration instructions cover Codex, Claude,
  Gemini, Cursor, and optional Ollama without implying that consumer subscriptions
  are generic model API credentials.

### Fixed

- data.go.kr's broken TLS 1.3 path no longer prevents portal access. The verified
  TLS 1.2 workaround is scoped to data.go.kr transports and the dedicated browser;
  unrelated API endpoints retain normal TLS negotiation.
- `doctor` no longer reports a portal markup drift when an expired session sends
  the key URL to the generic public homepage. Both application and key checks now
  classify that state consistently as `gongctl login` required.
- Exact lexical matches are protected from semantic reranking, mandatory request
  variables are preserved across redesigned detail layouts, and application success
  is confirmed from both the JavaScript dialog and the account list fallback.
- Account-wide credentials now stay behind a stricter boundary: MCP no longer
  returns the service key, calls upgrade and restrict key injection to the official
  `https://apis.data.go.kr` gateway, response bodies and catalogue result counts are
  bounded, and untrusted dataset identifiers are validated before portal navigation.
- Agent auto-selection falls through installed but unavailable CLIs within one
  three-minute budget, sends goals over stdin rather than process arguments, and
  requires the real portal purpose category for every application.
- Headless application browsers use isolated ports/profiles, are reaped on every
  exit path and on signals, and leave a recoverable marker so `logout` can clean up
  a browser left by an ungraceful process exit. Application pagination now works in
  both cookie-HTTP and live-browser modes without returning duplicate pages.
- Session operations use a cross-process advisory lock, so separate Codex, Claude,
  Gemini, Cursor, CLI, and MCP processes cannot race while the portal rotates the
  shared cookie. Local Chrome debugger probes are time-bounded as well.
- Catalogue and semantic snapshots are fsynced and atomically replaced; release
  tags independently run module, vet, test, and build gates before publishing.

## [0.7.0]

### Added

- **`describe` hands over the publisher's address for LINK datasets** (`linkUrl`).
  It previously said the spec lives on the publisher's site and the portal cannot
  help, while the portal was displaying that very URL a row away — every LINK dataset
  sampled (70/70) carries it. An agent that cannot get there guesses parameters
  instead, so the address is now surfaced and named in the note.

  gongctl does not follow it, and the note says why: the publishers are a long tail —
  39 distinct hosts across 70 sampled datasets, the largest 13% — each with its own
  registration, key and spec format. There is no single portal to support that would
  unlock the LINK 40%. The note also states that gongctl's account key does not work
  there, because "the spec is at this URL" otherwise reads as "and your key will
  authorise it".

## [0.6.0]

### Added

- **`call --wait` (MCP `waitSeconds`) waits out the gateway propagation.** An
  application is granted the instant it is made and the API still answers 403 for
  minutes — 7 to 10 in every run measured here. Previously the only handling was an
  error message asking the caller to retry, which put a timed retry loop in every
  agent and script that uses this, over behaviour belonging to the portal. Now the
  request is repeated once a minute until it answers or the wait runs out.

  The 403 became its own sentinel, separate from a rejected key, because the two
  demand opposite responses: propagation is fixed by repeating the identical request,
  a rejected key never is. Only propagation is retried — a bad key or malformed
  request still returns on the first attempt rather than being buried under a
  ten-minute wait. Running out of time is reported as "not propagated yet", with the
  elapsed time and attempt count, so a caller knows to ask again rather than re-apply.
  Capped at an hour (the portal's own ceiling); MCP caps at five minutes, since a
  tool call that blocks for an hour reads as a hung agent.

## [0.5.0]

### Added

- **`describe` surfaces the portal's 심의유형** as `approval{dev, ops, raw}` — whether
  an application is granted automatically or waits for a person at the publishing
  agency. `dev` is the stage gongctl's applications go through and decides whether a
  key arrives immediately; `ops` says what a later move to production would face. A
  missing row reports nothing rather than "automatic", and wording the parser does
  not recognise is kept verbatim instead of being read as auto-approved — assuming
  automatic is the answer that leaves a caller waiting on approval that is not coming.

### Changed

- **A known limitation from 0.1.0 was overstated and is withdrawn.** It said only
  auto-approved applications work and 심의 datasets cannot be driven to approval,
  which reads as a class of datasets gongctl cannot handle. In a random sample of 90
  catalogue entries, none required review at 개발단계 — the stage gongctl applies
  through. 57 carried the row (all 자동승인 at 개발단계, 16 of those 심의승인 at
  운영단계) and 33 had no row, being LINK datasets with no application path here
  anyway. The review gate is real but sits at 운영단계, which gongctl does not do; as
  written, the limitation would have talked users out of datasets that apply fine.

## [0.4.0]

### Added

- **`call --pk` looks the endpoint up instead of you typing one** (MCP `call_api`
  takes `pk`/`op`). Endpoint paths are not guessable —
  `HeatWaveCasualtiesRegion/getHeatWaveCasualtiesRegionList` cannot be derived from
  "지역별 폭염 인명피해" — and a wrong one answers 500 or 404 rather than saying it does
  not exist, so it reads as a broken API instead of a typo.
- **Required request variables are checked before the request is spent.** Omitting
  one makes data.go.kr answer with an *empty result rather than an error*, which is
  indistinguishable from a dataset that genuinely has no matching rows. Required-ness
  is read by prefix, because the embedded Swagger document says 필수/옵션 while the
  rendered HTML table says the portal's own 필/옵 — a check written against one
  vocabulary silently passes everything described by the other. Parameters the portal
  never documented block nothing.

### Fixed

- **`describe` no longer double-counts operations.** Some pages render each 상세기능
  twice — a desktop table and a mobile one — so a one-operation dataset reported two
  identical operations, which surfaced as an ambiguous choice whose two alternatives
  were the same call. Byte-identical operations are collapsed; operations sharing an
  endpoint but documenting different variables are kept.

## [0.3.0]

### Added

- **The catalogue now records each dataset's service type, and two in five are a
  dead end.** 4,770 of 11,932 OpenAPI datasets are `LINK`: the portal publishes no
  spec for them and only links to the publisher's site, so `describe` cannot
  produce an endpoint and an application spent on one buys a key that cannot be
  used here. Nothing surfaced that before — searching 폭염 returned a LINK dataset
  second, ahead of three callable ones, purely on application count.

  `catalog search --rest-only` (MCP `restOnly: true`) keeps only what the portal
  reports as REST; every result carries its `svcType` either way, and `catalog
  info` reports the breakdown. Verified against `describe` from the opposite
  direction: the LINK-labelled dataset reports `apiType: LINK` with zero
  operations, the REST-labelled one reports two operations with a live endpoint.

  Labels come from the portal's own `svcType` filter, and the counts it returns
  (REST 7,156 / LINK 4,770) account for the catalogue exactly. `sync` still starts
  with an unfiltered sweep that defines the catalogue, then labels what it found,
  so a service type nobody here has heard of yet leaves an entry **unlabelled
  rather than missing or mislabelled** — the 6 datasets the portal reports as
  neither show up as 미확인. Sync now takes ~235s instead of ~160s.

## [0.2.0]

### Added

- **API catalogue** (`catalog sync|search|orgs|info`, MCP `catalog_search`) — the
  portal only answers keyword queries, so finding out whether a dataset exists
  meant inventing search terms one at a time and never knowing whether a miss
  meant "doesn't exist" or "wrong word". `sync` sweeps the whole OpenAPI list
  (11,932 datasets in ~160s, 60 requests) into a local snapshot; `search` answers
  from disk instantly, ranked by application count, because demand is the best
  available proxy for "this one is actually usable".

  The gap this closes is real and was measured, not assumed: while researching
  heatwave data, four separate keyword searches missed
  기상청_생활기상지수 (5,045 applications, carries the heat-index figures) because
  none of the guessed words matched its wording. One catalogue search finds it.

  Descriptions are stored but never returned — they are what makes matching work
  and also what wrecks an agent's context (ten of them is ~3,000 characters of
  prose nobody asked for). Results are compact rows plus the total match count, so
  a caller can tell it should narrow the query rather than page blindly.
- **Queries can be written as sentences.** `"폭염에 취약한 고령자 데이터"` used to
  return nothing, because every term had to appear and particles and the word
  데이터 never do. Terms are now reduced (trailing particles trimmed, fillers
  dropped) before matching. Entries matching *every* term still win; only when
  there are none are the terms ORed — and that widening is reported as
  `relaxed`/`matched` rather than hidden, because a widened search answers a
  different question than the one asked. A term in the dataset's name outranks the
  same term buried in its blurb.
- **`catalog sync --if-stale`** — syncs only when the snapshot is old, so a cron
  entry or CI step can refresh unconditionally without anyone having to remember
  the cadence.
- **`doctor` now checks catalogue freshness**, reporting a stale snapshot as
  drift. A stale catalogue keeps answering while silently omitting everything
  published since the sync, which is exactly the kind of quiet wrongness `doctor`
  exists to make loud.

### Fixed

- **`apply` no longer burns the login session.** The portal rotates its session
  cookie during the application flow, and the rotated cookie lived only inside the
  headless browser — so every `apply` left the saved session dead and the next
  command demanded a fresh login. The rotated session is now captured before the
  browser closes (and only if it actually works, so a failed apply cannot
  overwrite a good session with a broken one).
- **`login` no longer opens a second login screen.** Polling for completion
  navigated a fresh tab to an authenticated page each time, which the portal
  bounced back to its login wall — so the user watched a second login appear on
  top of the one they were using. Login is now detected by reading cookies without
  navigating at all.
- **A rejected serviceKey is retried once with a freshly read key.** The cached
  key is normally right, but goes stale when the key is reissued on the portal;
  previously that surfaced as an authentication failure the user had to diagnose.
- Gateway propagation guidance now states the measured wait (7~10 minutes; the
  portal's own ceiling is an hour) and says explicitly that `list_applications`
  showing 승인 does not yet mean callable.
- `catalog sync` progress reported every 1,000 datasets, which left the first
  stretch of a multi-minute command silent and indistinguishable from a hang; it
  now reports every page.

## [0.1.1]

### Fixed

- **`logout` now removes the Chrome profiles**, not only the stored cookies and
  cached serviceKey. The login profile accumulates the cookies of whatever the
  human logged in *with* — an SSO provider's session, some of them persistent —
  and the headless profile holds the session gongctl injected. Leaving those
  behind after an explicit logout was the wrong boundary. Verified: 4.2 GB → 0 B,
  no cookie database left under the config directory.
- README's security section now states what is actually written to disk (with
  permissions), what `logout` removes, and the risks that remain: the files are
  `0600` but not encrypted; the local CDP port is reachable by other processes on
  the same machine while `login`/`apply` runs; MCP `apply` creates real
  applications without a human confirmation, so prompt injection through portal
  text can produce unwanted ones (the CLI's y/N confirm is the safer path); and
  the serviceKey is account-wide.

## [0.1.0]

First release. A Go CLI + MCP server that automates data.go.kr (공공데이터포털)
so an AI agent can go from "I need this data" to actual API responses without a
human touching the portal.

**A person logs in once in a browser. Everything after that — searching,
applying, confirming approval, fetching the issued key, calling the API — the
agent does itself.** No key is ever copied and pasted; after login no browser
window stays on screen.

Verified end to end on a real data.go.kr account: search → apply (auto-approved)
→ approval confirmed → serviceKey fetched → API called, with zero visible
browsers throughout.

### Added

- **Dataset search** (`search`, MCP `search_datasets`) — keyword search over
  data.go.kr's file and OpenAPI datasets. Results carry each dataset's publisher,
  last-modified date, view count and application count, so a dataset can be
  judged without opening its page. `--per-page` sweeps large result sets in few
  requests (the full OpenAPI catalogue — 11,932 datasets — in 60).
- **활용신청 automation** (`apply`, MCP `apply`) — fills and submits the
  application form by driving the portal's own `fn_save()`, so the portal builds
  and validates the payload. Auto-approved dev accounts get a key immediately.
- **Application list** (`applications`, MCP `list_applications`) — status, account
  type, and expiry. Paginated, so accounts with more than ten applications are
  reported in full.
- **serviceKey retrieval** (`key`, MCP `get_api_key`) — reads the account key the
  portal issues on first approval. `call` uses it automatically when `--key` is
  omitted, which is what closes the loop for an agent.
- **OpenAPI describe** (`describe`, MCP `describe_api`) — surfaces operations,
  endpoints and request variables. Reads the Swagger 2.0 document the portal
  embeds in the page when present (the authoritative source) and falls back to
  the rendered tables. Reports the portal's `API 유형`: REST pages carry a spec,
  LINK pages only point at the publisher's site. Surface-only — it never invents
  a parameter, and says where the spec actually lives when the page has none.
- **Authenticated call** (`call`, MCP `call_api`) — injects the serviceKey,
  converts XML responses to JSON, and surfaces error codes rather than swallowing
  them. A 403 right after approval is explained as gateway propagation (minutes,
  not a key problem) instead of being left to guesswork.
- **`doctor`** — drives each scraping seam live and reports ok/drift/skipped,
  exiting non-zero on drift. This project scrapes fragile HTML by necessity, so
  drift is made loud rather than absorbed.
- **MCP server** (`mcp`) — six tools plus a `gongctl://guide` resource, over
  stdio.
- Output as `json` / `jsonl` / `table`; goreleaser + Homebrew tap +
  `install.sh` / `install.ps1` distribution.

### Security

- The login browser is closed once its session cookies have been copied out, so
  no window and no browser process is left running. Reads then go over plain
  HTTP; only 활용신청 submission needs a browser, and that one runs **headless**
  with the session injected.
- Chrome is launched **without** `--remote-allow-origins=*`, so a malicious local
  web page cannot open the CDP port and drive the authenticated session (Chrome
  rejects foreign-Origin WebSocket upgrades; verified by spike). chromedp still
  attaches, as it sends no Origin header.
- The serviceKey is never logged or persisted in the clear: it is redacted from
  transport-error messages, and the cached copy is written `0600`. `logout`
  clears both the cookies and the cached key.

### Known limitations

- Only auto-approved applications work; 심의(manual-review) datasets are listed
  with their status but not driven to approval.
- A freshly approved API answers 403 at the gateway for minutes (about 8 in
  testing, up to an hour by the portal's own guidance) before it can be called.
- LINK-type datasets have no spec on the portal, so `describe` can only say so
  and point at the publisher.
- The portal's HTML can change at any time; `doctor` exists to detect that.
