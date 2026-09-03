# Repository rulesets

GitHub does not apply files in this directory automatically. They are the reviewed source for live repository rules.

The repository is currently private on a GitHub Free personal account, where GitHub rejects branch protection and
Rulesets. After the repository becomes public, apply the default-branch ruleset from a clean `main` checkout:

```sh
gh api --method POST repos/JungHoonGhae/oddsock/rulesets \
  --input .github/rulesets/main.json
```

Then verify the live rule and the two required CI checks:

```sh
gh api repos/JungHoonGhae/oddsock/rulesets
gh api repos/JungHoonGhae/oddsock/rules/branches/main
```

The ruleset requires pull requests, a linear history, resolved review conversations, and the `test-and-build` and
`windows-installer` checks. It blocks deletion and force pushes. It intentionally requires zero approvals because
this is a solo-maintained repository; requiring another person's approval would make routine maintenance impossible.
