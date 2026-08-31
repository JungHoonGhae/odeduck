---
status: accepted
---

# Treat LINK as a first-class external-provider handoff, not a broken REST API

The synced catalogue currently contains 4,766 LINK datasets out of 11,902
OpenAPI-labelled entries (40.0%). Ignoring LINK would therefore discard too much
of the public-data surface. But LINK is not one protocol: sampled targets include
an OpenAPI hub, a product-specific documentation tab, a provider dataset detail,
and an API-service detail page. A URL is evidence of an official starting point;
it is not evidence that the URL itself is callable or documents an API.

The KRDS redesign made this distinction easier to miss. It removed the legacy
labelled URL row and renders only a button whose JavaScript resolves the target
through `GET /tcs/dss/selectApiLinkUrl.do?publicDataPk=...`. The rest of a LINK
detail still parses successfully, so losing this lookup would otherwise become a
silent dead end.

## Decision

Keep the compact MCP surface and make `describe_api` branch by contract:

- REST returns the portal-published operations, endpoints and parameters that
  `call_api` can validate and invoke with the data.go.kr credential.
- LINK resolves the first-party portal lookup and returns `linkUrl` plus a
  structured `handoff` with the official host,
  `trust=publisher_supplied_untrusted`, `fetchPolicy=safe_fetcher_required`,
  `state=inspection_required`, and `nextAction=inspect_provider_contract`.
  Obvious local, private, link-local and numeric-literal targets are rejected as
  defence in depth, but this is not a complete fetch-time SSRF boundary: the
  consumer must validate resolved addresses and every redirect hop or stop.
- A generic LINK handoff is never passed to `call_api`. The agent or a future
  provider adapter must first establish the external documentation, application,
  authentication and invocation contract. The data.go.kr credential is never
  assumed to apply to an external host.

`describe_api` remains the only detail-stage tool. We will not create one MCP
tool per publisher or add a generic "call arbitrary URL" escape hatch. Reusable
provider support will live behind the detail boundary and enrich the handoff.
A provider adapter may report a stronger state only when its official contract
is versioned, its host-and-path match scope is explicit, and the scope is covered
by fixtures. Provider credentials must use separate namespaces and may be sent
only to that adapter's validated hosts.

The `doctor` command has a dedicated LINK canary (`15116894`) so the KRDS lookup
can fail independently of the REST describe canary and still be detected.

## Provider coverage strategy

Coverage is measured by resolved LINK datasets and their normalized destination
hosts, not by the raw count of individual endpoints. We will prioritize adapters
by three factors:

1. how many catalogue entries and high-value searches the provider unlocks;
2. whether the provider publishes a stable machine-readable or versioned spec;
3. whether application and credentials can be handled safely without imitating a
   human or guessing a contract.

A live resolution of the 50 LINK datasets with the highest portal application
counts found 16 VWorld, 6 FoodSafetyKorea and 6 Seoul Open Data Plaza targets.
Those three hosts covered 56% of the high-demand sample, which supports host-level
adapters even though the full LINK population remains a long tail.

The generic handoff remains the honest fallback for the long tail. A provider
page can always change or require manual approval, so "unsupported" and
"inspection required" are valid outcomes rather than parser failures.

## Considered options

- **Treat LINK as non-callable noise** — rejected because it is 40.0% of the
  catalogue and includes high-value data such as product certification/recall.
- **Assume every link is documentation and ask the model to browse it** —
  rejected because observed targets have different page roles. This overstates
  evidence and can turn a normal detail page into a fabricated endpoint.
- **Automatically fetch and classify every external URL in `describe_api`** —
  deferred. It adds open-world network/SSRF, redirect, size, JavaScript and prompt
  injection boundaries to a deterministic portal read. Provider adapters give a
  narrower and testable route. Any future fetcher must resolve DNS, reject
  non-public addresses and revalidate every redirect target.
- **One MCP tool per external provider** — rejected because tool count and model
  context would grow with the publisher long tail. The existing
  `catalog_search → describe_api → call_api` progression must remain compact.
- **Reuse the data.go.kr key on external hosts** — rejected. Credential scope is
  a provider contract and must never be inferred from a LINK URL.

## Consequences

- Agents receive an actionable official address after portal redesigns without
  being told that all LINK targets are directly callable.
- Existing clients retain `linkUrl`; newer clients can branch deterministically
  on `handoff.state` and `handoff.nextAction`.
- High-value providers can be integrated incrementally without changing the MCP
  entry points. SafetyKorea is the first documented contract because it exposes
  KC certification and recall data under a versioned external specification.
  Its handoff reports the separate manual application and `AuthKey` header but
  remains `invocationState=not_implemented` until a scoped credential and caller
  are added.
- Full LINK coverage is not promised. Coverage and adapter state must be surfaced
  explicitly, and unsupported providers remain discoverable rather than being
  silently filtered from broad research.
