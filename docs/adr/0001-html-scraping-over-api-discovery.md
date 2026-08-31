---
status: accepted
---

# HTML scraping with loud drift detection, not API discovery or self-healing selectors

gongctl reads data.go.kr by scraping HTML, which looks fragile enough that
"surely there's a better way" keeps coming up — an adaptive-selector library
(Scrapling), a crawler framework (goscrapy), an AI browser agent (Browser Use
et al), or discovering the JSON APIs beneath the pages (Unbrowse's approach). We
investigated the last one against the live portal and it settled the rest:
data.go.kr's read paths return **server-rendered HTML**, not a stable JSON data
API. We therefore keep deterministic HTML parsing, prefer narrow first-party
HTTP fragment endpoints when the portal exposes them, and manage markup
fragility with a *loud* drift signal (`gongctl doctor`) rather than a clever
parser.

## Evidence (2026-07-25, live authenticated session)

XHR/fetch captured via CDP while loading each page:

| Page | Data-bearing XHR/fetch |
|---|---|
| `selectDataSetList.do` (search) | none — only a `.hbs` template and Google Analytics |
| `openapi.do` (describe) | none — only `selectMberInfo.json` (member info) |
| `selectAcountList.do` (applications) | none — only `selectCommonCodeSelectboxList.json` (select-box codes) |
| apply submit | **`POST /iim/api/saveDevAcountRequest.do`** — a real endpoint |

## Revalidation (2026-08-31, KRDS redesign)

The portal replaced its search and account-list markup with KRDS components, so
we repeated API discovery before changing selectors. CDP captured request
metadata for public search, the authenticated application list, the API-key
page, and the apply form without recording cookies, payloads, or key values.

| Page | Data-bearing XHR/fetch after redesign |
|---|---|
| public search | none — rows are in the document HTML |
| OpenAPI detail | metadata and the initial operation are in the document HTML; remaining legacy operations come from **`POST /tcs/dss/selectApiDetailFunction.do`**, an HTML fragment endpoint |
| application list | none — `selectCommonCodeSelectboxList.json` only supplies common select-box codes |
| API-key page | none — the key field is in the authenticated document HTML |
| apply form | none — only a web-filter `.hbs` template |

The detail fragment endpoint is now used directly. It is a better boundary than
driving the operation selector UI even though its response still requires HTML
parsing. The apply submit endpoint still exists, but its form validation and
payload assembly remain browser-owned. The redesign therefore refines the
decision: use stable read endpoints when found, parse their factual HTML, and
keep submit form-driven.

This also establishes an operating rule: after a major portal redesign, rerun
network discovery first. HTML parsing is the fallback only after confirming that
the redesigned page did not introduce a stable data API.

## Considered options

- **JSON API discovery for reads** — rejected: search, account, and key data is
  only in server-rendered HTML. A targeted HTML fragment endpoint is used for
  multi-operation OpenAPI details because it is narrower than the page UI.
- **`POST /iim/api/saveDevAcountRequest.do` directly for apply** — rejected even
  though the endpoint exists. Driving the form's own `fn_save()` makes the portal
  build and validate the payload; hand-rolling the POST means reproducing every
  field (plus session/CSRF preconditions) and silently creating malformed
  applications when a field changes.
- **Adaptive/self-healing selectors (Scrapling)** — rejected: Python (breaks the
  single-binary distribution) and *probabilistic* — relocating an element by
  similarity can silently match the wrong thing. gongctl's contract is
  surface-only: fail loudly, never fabricate.
- **Crawler framework (goscrapy)** — rejected: gongctl hits three known
  endpoints; it is not a crawler, and a framework wouldn't make selectors any
  less brittle.
- **AI browser agents (Browser Use, Stagehand, agent-browser)** — rejected: an
  LLM deciding what to click is probabilistic, and `apply` creates a real
  application on the user's government account. That path must stay
  deterministic.
- **Swapping chromedp for another driver (Rod, Playwright-Go)** — not adopted:
  every Go option is CDP underneath, so this changes the API surface, not the
  substance, at the cost of re-validating hand-verified automation.

## Consequences

- Markup drift is inevitable, so it must be *detected*, not absorbed: parsers
  degrade to empty results, and `gongctl doctor` drives each seam live and exits
  non-zero on drift (CI-friendly).
- A zero-row authenticated application parse is ambiguous, not healthy. Doctor
  reports it as skipped, while non-empty lists report the parsed count. The
  API-key selector has a value-free probe so layout drift can be detected without
  exposing the credential.
- CDP stays. The one-time human SSO login plus a long-lived re-attachable session
  is the requirement that rules out managed/remote browser services anyway.
