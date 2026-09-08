# 선택 근거 읽기 검증 — 2026-09-08

상태: 선택 근거 기능의 구현·두 축 리뷰 기록. 독립 호출 경계 수정은 `490e4a8`에 커밋했다.
후속 [공통 통합 검증](../specs/goal-integration-and-cleanup-v1.md)과 구분한다. INTENT 전체는 미완료다.

## 범위와 실제 변화

- [결정](https://github.com/JungHoonGhae/odeduck/issues/42)을 해결하고
  [구현 추적](https://github.com/JungHoonGhae/odeduck/issues/46)으로 연결했다.
- ADR-0007에 기본 비공개·고정 수신자·원문 수명·장부 재사용의 신뢰 경계를 기록했다.
  별도 graph DB, 전역 entity merge, LLM 자가 의미 승인은 추가하지 않았다.
- 공통 Engine의 read_evidence가 실제 관측 hash/행/필드를 대조해 선택값·missing/null·원본 주소를
  반환한다. 공간 비교의 원본 CSV 위치와 계산 pair를 구분한다. 셀/packet/세션 누적 예산과 만료를
  적용한다. CLI/MCP는 공개 상한을 정하는 adapter이며 별도 선택 실행기를 갖지 않는다.
- CLI --share-evidence는 명시한 단일 agent만 허용한다. MCP --share-goal-evidence는 서버 시작
  설정이며 모델 tool 입력에 권한 필드를 추가하지 않았다. 전체 Artifact는 CLI 계획 입력에서 제외한다.
- 기존 절대 원문 비노출 안내를 기본 비공개+선택 공개 계약으로 갱신했다. 외부 제공기관 응답에
  대체 문자열을 만들어 원천 관측으로 반환하던 경로를 제거했다. 역사적 실패 fixture/ADR은 유지했다.

## 검증

| 검사 | 실제 결과 |
|---|---|
| TDD: public Call 응답 경계 | 기존 동작의 실패를 확인하고 공통 호출 경계에서 수정 |
| TDD: Engine 선택 근거 | unknown action 실패 확인 후 read_evidence 구현 |
| TDD: PlanGoal/CLI 공개 정책 | auto 수신 제한 누락·비공개 prompt 누락·미등록 flag 실패 후 수정 |
| MCP JSON-RPC | 시작 권한 위조·다른 연결의 세션 접근 거부, 고정 서버 정책, 정확 수치 반환 통과 |
| 선택값/주소 | 미선택 행·필드 비공개, 큰 정수·0·false·빈 문자열·null·missing, CSV/worksheet 원본 위치 통과 |
| 공간 근거 | 후보 prefix 밖 원본 1002번 레코드와 anchor 원본·computed pair 구분 통과 |
| 안전 한계 | stale hash·중복 행·credential 필드/표식·셀/packet 수·누적 bytes, detached view·1시간 만료 통과 |
| 전체 기본 검사 | go mod tidy -diff, go vet ./..., go test ./... -count=1, go build ./... 통과 |
| 독립 커밋 검증 | 490e4a8만의 임시 detached worktree에서도 tidy/vet/전체 test/build 통과; 검증 후 깨끗한 임시 worktree 제거 |
| Race | 전체 go test -race ./... -count=1 통과; 마지막 호출 경계 수정 후 apicall/goalwork/agentplan/mcpserver/CLI 재검사 통과 |
| 공개 브랜드 일치 | sync-brand --check 통과 |
| 실제 adapter drift | doctor --adapters-only: 4 providers, 11 canaries 모두 정상 |

테스트 정답은 fixture의 독립 literal이며 실행 결과에서 역산하지 않았다. CLI/MCP 통합 검사는
동일 public Engine을 검증하며 의미 승인 gate를 낮추지 않는다. 실제 원천 목표의 성공 평가가 아니다.

초기 수신자 회귀 검사의 red 단계에서 provider 탐색을 fixture로 대체하기 전에 auto 경로가 한 차례
실제 계획기를 호출했다(검사 시간 11.25초). 입력은 원천 관측·값·인증키가 없는 최소 View였다.
이를 live 목표 성공으로 세지 않는다. 해당 검사를 즉시 모의 provider 검색으로 고쳤으며 현재 단위
검사는 설치된 실제 모델을 호출하지 않는다. 최종 검증의 명시적 live 호출은 위 adapter drift 점검이다.

## 독립 리뷰

- Spec: 최초 1건 — 공통 응답 보호의 LINK 경로 누락. 수정하고 재검토해 남은 지적 0건.
- Standards: 최초 2건 — 같은 LINK 경계와 CHANGELOG 누락. 수정하고 재검토해 남은 지적 0건.
- 리뷰는 read-only이며 서로 독립적으로 진행했다. 새로운 저장 abstraction을 추가할 필요는 없었다.

## 통합·정리 경계

시작 HEAD는 `450d17b4a8edb0927d7fccd30592b7211013e4fe`였다. 기존 목표 실행 foundation과 관련
원천 reader 상당수가 이미 untracked/dirty였다. 이를 새 변경으로 오인해 한꺼번에 커밋하지 않았다.
이번 독립 커밋 `490e4a8`은 호출 경계·회귀 테스트·provider 설명 및 보안 CHANGELOG hunk만 담는다.
당시 선택 근거 코드/문서는 worktree에 남겼다. 이후 교체·통합 결정에 따라 foundation과 함께
통합하며, 단계별 검증 결과는 위 공통 통합 계약에 둔다.
무관한 변경과 과거 오답·음성 회귀 근거는 삭제하지 않았다. push·배포·신규 활용신청은 하지 않았다.

## 남은 제품 작업

선택 근거를 읽어도 자동 의미 승인·목표 완료가 되지는 않는다. 결과를 읽고 가설/인용을 만들 수 있는
상태 전이, 원천 의미 매핑의 검토, 기존 장부의 revision/기간/supersedes 재검증과 실제 재사용,
원천 정답에 대한 G1–G5 자율 양성 완주가 남아 있다.
[공통 실행 경로와 이전 구현의 교체·삭제 순서](https://github.com/JungHoonGhae/odeduck/issues/43)는
이후 결정했으며, 그 통합이 위 제품 작업을 대신하지 않는다.
