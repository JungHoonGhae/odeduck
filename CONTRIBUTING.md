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

