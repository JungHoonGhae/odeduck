# Access and API calls

1. **Discover from the goal.** Identify the needed measures, geography, period and output. Search with
   `catalog_search`, varying Korean domain terms and looking for complementary sources where useful.
   Use returned dataset IDs; catalogue descriptions identify candidates, not verified relationships.
2. **Inspect before access.** Call `inspect_dataset` for selected IDs; use `observe=true` when file columns
   or a bounded standard-data sample are needed. Read delivery type, operations, required parameters,
   coverage and approval conditions. A search result is not an executable API contract.
3. **Acquire access within the user's request.** For a selected data.go.kr REST API, use `list_applications`
   to check existing access. If access is missing and the task authorizes applying, call `apply` with a
   purpose derived from that task and the matching category. Follow the host's approval policy. A
   search-only request does not authorize applications. Apply only to selected APIs needed for the task.
4. **Verify approval.** Reuse an existing approved application. If the session is absent or expired,
   have the user complete `odeduck login` in the browser. Government SSO is manual. Report pending
   agency review as pending; do not resubmit to bypass it. For a newly approved API, `call_api` can wait
   for gateway propagation with `waitSeconds=300`.
5. **Call the inspected contract.** Pass the dataset ID, observed operation and required parameters to
   `call_api`. The runtime resolves the endpoint and injects the stored credential. Check the provider's
   result code as well as HTTP status. Never ask the user to paste an API key or cookie into the conversation.

### Delivery-specific paths

| Inspection result | Next action |
| --- | --- |
| data.go.kr REST API | Check access, apply if needed and authorized, then call the inspected operation. |
| FILE / standard data | Use supported inspection and bounded observations. Report what was actually read; `call_api` does not download files. |
| LINK to a provider | Use `call_api` only when inspection reports an implemented invocation contract. Follow its typed operations and provider-scoped key setup. Otherwise return the official handoff and missing requirement. |

Do not send LINK or FILE datasets to `apply`, guess provider endpoints, or bypass an insecure-transport block.
Provider key setup belongs in the local CLI, never in chat. Read the installed command's help when needed.

## CLI equivalents

Replace placeholders with values returned by search and inspection. Use JSON output for agent processing.

```sh
odeduck catalog search "건축물" --limit 5 --semantic=false -f json
odeduck inspect <PK> --observe -f json
odeduck applications -f json
odeduck apply <PK> --purpose "<actual user purpose>" --category research
odeduck call --pk <PK> --op <OPERATION> --param <NAME>=<VALUE> -f json
```

`research` is an example category; use the actual purpose (`research`, `web`, `app`, `ref`, or `etc`).
CLI application asks for confirmation. In a noninteractive agent session, use `--yes` only when that
application is already authorized; it is not a reason to ask again. Call with `--pk` to retain contract
validation and automatic credential handling. `--wait 5m` can wait for propagation after approval.
