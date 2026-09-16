---
name: odeduck
description: Use odeduck to turn Korean public-data goals into sourced reports and comparisons, or find, inspect and call data.go.kr datasets. 공공데이터 목표 기반 분석·탐색·활용신청·호출에 사용한다.
license: MIT
---

# odeduck — Korean public data for agents

Turn the user's question into the requested report, comparison or data, with supporting sources and gaps.
Use odeduck's local execution engine for inspection, applications and authenticated calls.
The supported portal is data.go.kr, including its registered files and reviewed provider adapters.

## Choose the available interface

- **MCP connected:** read its current tool schemas and `odeduck://guide`. Tool names below may have a host prefix.
- **CLI available:** run `odeduck version` and read the relevant command's `--help` before using its options.
- **Neither available:** follow [setup](references/setup.md). A skill installation alone does not install the binary or connect MCP.

The running server's guide and installed CLI help are authoritative for supported actions, arguments,
limits and prerequisites. This skill describes intent and workflow; it does not pin a release or override
the runtime contract. If a needed capability is missing, identify that gap and offer an update within the
user's setup scope; do not assume repository main matches the installed runtime or silently replace it.

MCP and CLI use the same backend; either is sufficient. Ordinary catalogue search needs no portal login,
API key or Ollama. When already running inside an agent, plan searches directly rather than starting a
second agent through `catalog discover`.

## Route by the requested outcome

| User needs | Entry point |
| --- | --- |
| A sourced report, calculation or comparison requiring discovery and execution | `advance_goal` through MCP, or `odeduck solve` through CLI |
| Candidate datasets or a particular API's data | `catalog_search` → `inspect_dataset` → [access and API calls](references/access.md) |

### Execute a goal

`solve` / `advance_goal` is odeduck's goal-execution workflow. Start with the user's goal and preserve its
required output, geography and period. For MCP, use the running guide's session/revision and action
contract. Define required roles and outputs, then inspect actual sources, acquire observations, compose
and execute. Use returned gaps to seek missing evidence, another source or a justified crosswalk within
the same goal. A report request is not complete when only dataset links have been found.

Use the runtime's supported actions and completion status. In the current contract, `sample_executed`
means a bounded computation ran; `review_required` still needs evidence or authorized review.
Report the artifact, source support and unresolved requirements separately. Keep partial progress when
completion is blocked rather than relabeling it as success or abandoning the requested output early.

Check the runtime's semantic-search and model prerequisites. Read the installed `solve --help` before
launching a CLI planner; if that version provides a non-executing preflight, use it to diagnose setup.
Goal execution currently does not submit access applications itself: resolve a selected API's missing
access through the [ordinary access workflow](references/access.md) and follow the runtime's continuation guidance.
Do not enable optional source sharing or external model review without authorization for the recipient
and data involved. Normal MCP query results are still visible to the host AI.

For standalone dataset access, read [access and API calls](references/access.md). Read
[setup](references/setup.md) only for missing installation, connection or an authorized update.

## Deliver evidence at the level obtained

Return the requested data or candidate list with dataset IDs, official source URLs, relevant period and
geography, and any access or coverage gaps. Distinguish discovered, inspected, pending approval and
successfully called sources. Mark bounded samples as samples.

Cross-source comparisons require compatible identifiers, geographic scope, units and periods. Matching
names or nearby descriptions do not establish identity or causality. Keep missing records and unresolved
conditions visible. Do not equate a generated row with the user's goal being complete.

Goal execution is under active development; general autonomous completion is not established. For
setup and supported review modes, consult the running guide first, then the
[advanced guide](https://github.com/JungHoonGhae/odeduck/blob/main/docs/advanced-usage.md), checking it
against the installed runtime. A model's approval does not establish field verification or causality.
