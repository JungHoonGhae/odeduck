# Security policy

## Supported version

Security fixes are made against the latest release and `main`. Upgrade to the newest release before reporting a
problem that may already be fixed.

## Report a vulnerability privately

Do not open a public issue for a vulnerability or include a live credential in any report.

Use GitHub's private vulnerability reporting from the repository's **Security** tab. If that option is unavailable,
email `lucas.ghae@remodule.dev` with the subject `oddsock security report`.

Include the affected version, operating system, impact, minimal reproduction, and whether the report involves a
data.go.kr session or a provider credential. Redact cookies, keys, account identifiers, and downloaded personal data.
You should receive an acknowledgement within five business days.

## Important boundaries

- oddsock does not bypass government SSO or provider approval.
- Session cookies and provider keys are stored in the operating system's user configuration directory with restricted
  file permissions, but they are not encrypted at rest.
- Credentials must never appear in MCP input/output, command logs, fixtures, screenshots, or issue attachments.
- Only test accounts and data you are authorized to access. Do not probe government or third-party systems beyond
  the documented workflow in order to demonstrate a report.
- This project does not currently operate a bug-bounty program.

The maintainer will coordinate validation and disclosure timing with the reporter. Confirmed fixes will be noted in
the changelog without publishing reusable credentials or unsafe exploitation details.

