# Contributing to odeduck

odeduck welcomes fixes, reproducible public-data questions, documentation improvements, and carefully scoped
provider adapters. A contribution is most useful when another person can verify it without access to your account.

## Before opening a pull request

- Use a GitHub issue for a new workflow, provider adapter, or behavior change.
- Small documentation and test fixes can go directly to a pull request.
- Security reports do not belong in public issues; follow [SECURITY.md](SECURITY.md).
- Never commit a portal session, cookie, API key, downloaded personal data, or an unsanitized HTML fixture.

## Development

odeduck uses Go 1.26.6 or newer.

```sh
go mod tidy -diff
go run ./scripts/sync-brand.go --check
go vet ./...
go test ./...
go build ./...
```

Unit tests must run without network access. Tests that need a real browser must skip under `go test -short ./...`.
Live provider drift belongs in the scheduled canary workflow, not in ordinary pull-request tests.

## Design and validation references

The root README is for people installing and using odeduck. Keep implementation details, review contracts,
test commands and asset-production instructions in the linked documents below.

| Reference | Contents |
| --- | --- |
| [Product intent](INTENT.md) · [Completion plan](docs/specs/goal-driven-completion-plan.md) | Expected outcomes, requirements and remaining validation |
| [Architecture](ARCHITECTURE.md) · [Domain language](CONTEXT.md) | Component responsibilities, execution flow and shared terminology |
| [Validation summary](docs/validation-summary.md) | Evidence, tested scope, open limits and competitor research |
| [Discovery specification](docs/specs/cross-domain-connection-discovery-v1.md) · [Evaluation](docs/research/connection-discovery-evaluation.md) | Candidate generation and selection contracts |
| [Connection evidence ledger](docs/specs/connection-evidence-ledger-v1.md) · [ADRs](docs/adr/) | Provenance, assessment rules and design decisions |

Goal-execution contributions should demonstrate both reaching the requested output and rejecting unsupported
connections. Keep the tested scope clear: a sample calculation or a model review does not establish general
autonomous completion or field verification.

## README and visual assets

Keep the introduction animation, application-to-query workflow and overall architecture visible in the README.
Explain what users can do and the conditions that affect them. Put optional CLI flags and experimental settings
in [advanced usage](docs/advanced-usage.md).

The public name, tagline and logo paths come from [brand.json](docs/brand/brand.json).
After changing them, run `go run ./scripts/sync-brand.go`; CI checks that the generated README block matches.

The [visual asset guide](docs/assets/README.md) lists editable diagram sources, Pretendard fonts, style decisions
and HTML/PNG export commands. Commit source changes and generated assets together.

## User-facing Agent Skill

[`skills/odeduck/`](skills/odeduck/) is the installable product skill. Keep it self-contained and route report,
comparison and calculation requests to goal execution. `.agents/skills/` contains maintainer guidance and
is not a runtime dependency. The installed CLI help and MCP guide own version-specific execution contracts;
skill setup uses the latest stable release URL, while validation records retain their tested revisions.

After changing the skill, check discovery with `npx skills add ./skills/odeduck --list` and install it into a
temporary project to verify that its referenced files travel with it. See [the skill guide](docs/agent-skills.md).

## Maintainer skills

Keep `.agents/skills/*/SKILL.md` as short task routers. Add specialized procedures to the relevant
`references/` file, with a new skill only for an independent trigger and workflow. Runtime capabilities
belong to the CLI/MCP contracts. See [extension guidance](docs/agent-skills.md#개발용-skills를-확장할-때).

## Provider adapters

A provider adapter is an executable trust boundary, not a collection of guessed endpoints. New or changed adapters
must include:

- an official HTTPS documentation source;
- an exact hostname, path, credential placement, and credential scope;
- typed operations and bounded parameters;
- fixture-backed contract tests with obvious dummy credentials;
- a reproducible canary that never writes or mutates third-party data; and
- a documented stop or handoff when the provider contract cannot be verified.

Read [the provider adapter guide](docs/provider-adapters.md) before implementation.

## Portal fixtures

When data.go.kr markup changes, keep the smallest fixture that reproduces the parser behavior. Remove names, account
identifiers, cookies, keys, application history, and unrelated page content before committing it. Explain which
selector or contract changed in the test name or pull-request body.

## Pull-request checklist

- The change has a focused purpose and an issue when the scope is larger than a small fix.
- User-visible behavior and limitations are documented.
- New logic has a deterministic test.
- No test requires a contributor's live account or credential.
- `go test ./...`, `go vet ./...`, and `go build ./...` pass.
- `CHANGELOG.md` is updated for a user-visible change.

By contributing, you agree that your contribution is licensed under the repository's [MIT License](LICENSE).
