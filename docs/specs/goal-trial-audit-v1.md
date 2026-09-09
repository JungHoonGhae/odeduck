# Goal Trial Audit v1

Status: implemented locally, 2026-09-08; not a passing goal benchmark.
Parent: [evaluation decision](https://github.com/JungHoonGhae/odeduck/issues/44).
Implementation: [common trial audit](https://github.com/JungHoonGhae/odeduck/issues/50).
Policy: [goal benchmark](goal-completion-benchmark-v1.md#evaluation-contract-2026-09-08).

## Problem statement

Current diagnostics preserve useful failures but have no shared report across the frozen goals. Test success,
produced rows and runtime completion are easy to conflate. Missing cases must remain visible while approval
and independent goal oracles are implemented.

## Solution

A read-only developer command reads a small manifest of cases and trial files, emits counts without raw source
values, and fails a minimum readiness check when any declared positive case lacks an engine-ready candidate.
This check is necessary, not sufficient: the report explicitly leaves correctness, safety, autonomy, held-out
status and business utility ungraded. It never writes to the engine, ledger or retained evidence.

## User stories

1. As a maintainer, I can audit all declared cases without dropping an unrun case.
2. As a maintainer, I can count every scheduled trial, including missing or invalid artifacts.
3. As a reviewer, I can distinguish review, abstention, partial execution, command failure and ready candidates.
4. As a reviewer, I can see semantic-search use and acquisition/execution counts without treating use as benefit.
5. As a reviewer, I can identify the exact input artifact by hash without exposing source row values.
6. As an evaluator, I can retain old diagnostic envelopes and inspect new raw execution views through one seam.
7. As a maintainer, I get a failing readiness check for all-review/all-abstain, even if tables were produced.
8. As a maintainer, I can add a new domain to the manifest without changing the reporter.

## Implementation decisions

- One audit module owns manifest validation, bounded artifact reading, view summarization and aggregation.
  The command only selects the local manifest and renders the report/exit code. No new public product command,
  server, persistent state, model call, graph store or general-purpose grader framework.
- Manifest v1 contains named cases with the exact goal, positive/negative/dependency expectation and trial
  references. A trial has a unique ID, relative file path and explicit raw-view or retained-diagnostic format.
  Zero references means no trials were scheduled, not three retrospectively invented attempts.
- Missing files remain missing trial rows. Invalid JSON, inconsistent run/view status, unknown status, mismatched
  goal, or malformed view remain invalid trial rows. Duplicate case/trial IDs and repeated artifact paths are
  manifest errors. Unknown manifest fields, duplicate JSON members and case-folded schema/control-field
  collisions are errors; source row keys remain case-sensitive. Bound file size and suppress parser contents.
- Summaries use actual view searches, observations, acquisition attempts, execution steps and artifact rows.
  `output_ready` additionally requires requirements-met, no semantic review and a nonempty artifact before it is
  a ready candidate; a diagnostic reporting command failure cannot count as ready. Other statuses never qualify.
  Raw views cannot establish process exit status; preserve that as unknown, not success.
- A missing activity field is null, not zero; it also makes that metric's aggregate unknown. A present empty
  array is a measured zero. Historical artifact summaries remain separate from actual retained rows and cannot
  establish readiness. Candidate counts are not inferred from remaining budget or prose summaries.
- Report per-trial and per-case counts, planned/loaded/missing/invalid trial denominators and unrun cases.
  A positive case needs at least one ready candidate to meet the command's **readiness floor**. No positive case,
  any missing/invalid artifact, or a positive case with none fails that floor. This is not the three-trial gate.
- Always label the report as an execution audit, with independent grading not performed. Do not return an
  autonomous-completion or precision percentage. An artifact hash identifies input bytes, not an audited trace.
- Retained diagnostics remain byte-for-byte unchanged. Add a retrospective manifest for their actual two G2
  trials and the other four unrun cases. New future trials must be predeclared in their own manifest.

## Testing decisions

Test at the audit module's public interface using real filesystem adapters and bounded fixture files. Exercise
the real GoalWork engine for the all-review counterexample, and the two retained diagnostic artifacts for actual
historical counts. Independently specified synthetic ready views only test reporting semantics, never production
completion. Test command output and exit status at its entry seam; default tests perform no network/model calls.

## Out of scope

Production semantic approval, oracle authorship/grading, claims of strict autonomy or held-out status, automatic
trial scheduling, source acquisition, usage instrumentation, business/customer validation and release gates.
Those remain governed by the benchmark; this audit does not satisfy them.

## Use and retained-record result

From the repository root:

```sh
go run ./scripts/goal-audit internal/goalwork/testdata/goalbench-v1/trial-audit.json
```

The command reads only artifacts beneath the manifest directory. Manifest input is capped at 1 MiB, each
artifact at 16 MiB, with at most 64 cases and 1,024 trial references. JSON nesting is bounded at 64 and individual
audit control objects at 128 fields; source row keys keep their original case. JSON output contains no raw rows.
Exit 0 means only the readiness floor was met, 1 means it was not, and 2 means manifest/output failure.
The report always says independent grading was not performed. The Go launcher itself reports any nonzero
child exit as its own failure; use a built binary when distinguishing exit 1 from 2 in shell automation.

The retrospective manifest declares five cases and the two retained G2 diagnostics only. At 2026-09-08 the
audit shows two abstentions, four cases without a scheduled retained trial, 12 semantic searches, 14 observed
sources, 15 acquisition attempts and three executions. Neither record retains candidate arrays or actual
artifact rows, so those counts are unknown. One preserves a six-row summary, separately reported. The readiness
floor fails with zero ready candidates. This is a reread of old records, not a new live trial, an oracle grade,
or proof that the other four cases were never investigated outside this manifest.

The next architecture decision is [claim-specific approval and replanning](https://github.com/JungHoonGhae/odeduck/issues/51).
