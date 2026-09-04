# Issue tracker: GitHub Issues

이 저장소의 canonical issue tracker는 [JungHoonGhae/oddsock GitHub Issues](https://github.com/JungHoonGhae/oddsock/issues)다. 이전 local-markdown tracker의 `.scratch/` 문서는 Git 이력에만 남아 있다. 과거 노력을 다시 시작할 때 필요한 항목만 GitHub로 이관한다.

구현 spec과 장기 문서는 저장소에 versioned Markdown으로 두고, 이를 추적하는 GitHub issue에서 링크한다.

## When a skill says "publish to the issue tracker"

`gh issue create -R JungHoonGhae/oddsock`로 GitHub issue를 만든다. 상태는 GitHub 자체 상태와 assignee를 사용하고, 본문에 별도 `Status:` 줄을 만들지 않는다.

## When a skill says "fetch the relevant ticket"

사용자가 준 issue URL이나 번호를 `gh issue view -R JungHoonGhae/oddsock`로 읽는다. sub-issue와 dependency는 GitHub의 Relationships에서 확인한다.

## Wayfinding operations

- **Map**: label `wayfinder:map`인 하나의 GitHub issue.
- **Child ticket**: map의 native sub-issue. 유형은 `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling`, `wayfinder:task` 중 하나의 label로 기록한다.
- **Blocking**: GitHub의 native `blocked by` dependency. 본문 체크리스트나 번호 관례로 대체하지 않는다.
- **Frontier**: map의 open sub-issue 중 dependency가 모두 closed이고 assignee가 없는 issue. GitHub UI의 Relationships 또는 REST `sub_issues`와 `dependencies/blocked_by`로 조회한다.
- **Claim**: 작업하기 전에 issue를 자신에게 assign한다. open + unassigned가 unclaimed다.
- **Resolve**: 결정을 issue comment로 남기고 close한다. 그 다음 map의 `Decisions so far`에 issue 제목 링크와 한 줄 gist만 추가한다.

GitHub REST 예시:

```sh
# child 연결
gh api -X POST repos/JungHoonGhae/oddsock/issues/<MAP>/sub_issues \
  -H "X-GitHub-Api-Version: 2026-03-10" -F sub_issue_id=<REST_ISSUE_ID>

# <ISSUE>가 <BLOCKER>에 막히도록 연결
gh api -X POST repos/JungHoonGhae/oddsock/issues/<ISSUE>/dependencies/blocked_by \
  -H "X-GitHub-Api-Version: 2026-03-10" -F issue_id=<BLOCKER_REST_ISSUE_ID>
```

REST integer id는 `gh api repos/JungHoonGhae/oddsock/issues/<NUMBER> --jq .id`로 얻는다.

## Triage

triage role과 label 이름의 대응은 [triage-labels.md](triage-labels.md)를 따른다. GitHub에 없는 label은 사용 전에 생성한다.
