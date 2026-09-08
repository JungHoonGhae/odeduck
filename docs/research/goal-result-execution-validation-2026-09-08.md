# Goal Result 첫 실행 경로 검증 — 2026-09-08

상태: 첫 slice 구현·검토 기록. 후속 [공통 통합 검증](../specs/goal-integration-and-cleanup-v1.md)과 구분한다.
제품 목표는 미완료다.
[결정 지도](https://github.com/JungHoonGhae/odeduck/issues/40) →
[산출물 결정](https://github.com/JungHoonGhae/odeduck/issues/41) →
[구현 티켓](https://github.com/JungHoonGhae/odeduck/issues/45).
계약은 [Goal Result execution v1](../specs/goal-result-execution-v1.md)에 둔다.

## 변경 및 삭제

- 공개 목표 실행에서 공간 관측에만 허용하던 zero-join gate를 삭제했다. 기존 연산으로 단일 원천 필드 선택과 집계를 실행한다.
- 현재 writer의 `sample_joined` 이름을 `sample_executed`로 교체했다. 추가 호환 분기나 별도 엔진은 만들지 않았다.
- 실행하지 않은 join 설명과 일반 날짜 필터의 중복 nearest 경고를 제거했다. 실제 공간 원천의 한계 설명은 남겼다.
- CLI/MCP tool·초기 instructions·guide와 현재 문서를 갱신했다. ADR에는 날짜가 있는 amendment를 추가했다.
- `INTENT.md`의 목표·제약·성공 조건은 유지하고 사용자 위임에 따른 재개 상태와 지도 링크만 갱신했다.
- 파일 전체 삭제는 없다. 과거 진단·독립 oracle·오탐 회귀와 무관한 이전 변경은 보존했다.

## Red → green

1. `TestSingleSourceResultUsesObservedFieldsWithoutInventingAJoin`: 공간 base가 아니라는 공개 compose 오류로 실패 → gate 제거와 공통 실행 상태 적용 후 통과.
2. `TestSingleSourceResultExplainsOnlyExecutedOperations`: 실행하지 않은 inner join의 설명으로 실패 → 실제 join 유무에 따른 설명으로 통과.
3. `TestSingleSourceEmptyTimeResultDoesNotInventSpatialWork`: 일반 시점 불일치를 spatial 결과 오류로 표시해 실패 → 중립적 오류로 통과.
4. `TestSingleSourceResultRetainsRequestedTimeWindow`: 리뷰에서 지적한 nearest 경고 assertion이 실패 → 중복 경고 제거 후 통과.

추가 public 회귀는 정확한 0.10+0.20=0.3, 큰 정수 합 9007199254740993+1=9007199254740994,
원천/요청 보존, 날짜 필터, 필수 역할/population 미충족, planner 원문 비노출,
실제 CLI Run과 MCP JSON-RPC의 동일 실행·검토 상태를 확인한다.

## 검증

- 시작 `go test ./...` 통과(일부 cache).
- 수정 후 `go mod tidy -diff`, `go vet ./...`, `go test ./... -count=1`, `go build ./...`, `git diff --check` 통과.
- 첫 전체 `go test -race ./... -count=1` 통과. 마지막 안내/시간 설명 수정 이후 `go test -race ./internal/goalwork ./internal/mcpserver ./cmd/odeduck -count=1`도 통과했다.
- opt-in 실제 포털/agent 실행·로그인·활용신청·provider key 변경은 하지 않았다. 위 결과는 fixture와 로컬 실행 근거다.

### Standards

독립 검토 1건: 활성 MCP 안내가 새 상태명/동작과 불일치. 수정 후 재검토에서 해결, 남은 지적 0건.

### Spec

독립 검토 2건: 위 MCP 안내 불일치와 일반 날짜 필터의 nearest 경고. 수정 후 단일 원천·공간·시간·MCP 검증과 재검토에서 해결, 남은 지적 0건.
이 검토는 첫 slice 계약에 한정되며 전체 INTENT 구현 검토가 아니다.

## 복구 및 비교 기준

시작 HEAD는 `450d17b4a8edb0927d7fccd30592b7211013e4fe`이지만 기존 큰 dirty 변경이 있어
HEAD 대비 전체 diff를 이번 변경으로 간주하지 않는다. 수정 직전 다음 내용을 Git blob으로 보존했다.
`git show <blob>`으로 확인할 수 있다. 아직 ref로 보호된 전체 스냅샷/통합 커밋은 아니므로 Git GC 이후까지
보장되는 장기 백업은 아니다. 이번 코드를 통합할 때 기존 foundation 변경과 새 delta를 구분해 검토한다.

| 파일 | 수정 전 blob |
| --- | --- |
| internal/goalwork/engine.go | f9151027c4b8484242f6fa3a2e075be9cd6bbd33 |
| internal/goalwork/compose.go | b32ec010308423c42f6230020b5c48b01990c810 |
| internal/goalwork/explanation.go | e4a306c9056d58b2b84c5103b7c1335a865145f0 |
| internal/goalwork/temporal.go | a5c2bdcd866cc11c30d82621c254b46cfabed81c |
| internal/agentplan/goal.go | 08174771a416a5217af51c3490bf34270f6e5eba |
| internal/mcpserver/goal.go | 8272044f089227bdbb6e252a1c7e90c65db99bda |
| internal/mcpserver/guide.go | f4ac6a8739e91b7a5aa0ecfd51c42c98cd53adec |
| cmd/odeduck/solve.go | 22434f554041bb644c1b18c92cd4db726fc8d3da |

## 잔여 범위 / 다음 행동

현재 결과는 여전히 의미 미검증이면 `review_required`다. 단일 원천 실행을 허용한 것만으로 자동 완료,
사업 가설 생성, 지식 재사용, G1–G5 또는 I1–I10을 달성하지 않았다. 이전 고정 평가의 질문/정답을 낮추지 않았다.
당시 후속이던 근거 공개 결정과 구현은 [선택 근거 검증](selected-goal-evidence-validation-2026-09-08.md)에
이어 기록했다. 현재 통합은 [공통 실행 계약](../specs/goal-integration-and-cleanup-v1.md)을 따른다.
[과적합 없는 원천 정답·자율 완주·사업 효용 평가](https://github.com/JungHoonGhae/odeduck/issues/44)와
전체 목표 감사는 계속 남아 있다.
