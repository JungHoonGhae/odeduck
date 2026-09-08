# Goal Result execution v1 — 결합을 강제하지 않는 표본 실행

상태: 개발 브랜치 구현. [기능 검증](../research/goal-result-execution-validation-2026-09-08.md)과
[공통 통합 검증](goal-integration-and-cleanup-v1.md)을 구분해 기록한다. 아직 릴리스하지 않았다.
[INTENT 지도](https://github.com/JungHoonGhae/odeduck/issues/40)의
[산출물·완료 결정](https://github.com/JungHoonGhae/odeduck/issues/41)에서 확정된 첫 vertical slice.
전체 목표의 구현 명세를 대체하지 않으며 I1–I10, G1–G5, 자동 의미 승인·지식 재사용은 계속 미완료다.

## Problem Statement

사용자가 한 원천의 기록이나 집계를 요청해도 공개 목표 실행은 추가 원천 결합을 강제한다.
실행하지 않은 join의 이름·한계가 결과에 붙어 무엇을 계산했는지 혼동하게 한다.

## Solution

같은 목표 실행으로 한 원천의 필드 선택·수치 변환·집계와 여러 원천 결합을 처리한다.
산출물을 생성했는지와 요구/의미 검증을 통과했는지를 분리해 보고한다.

## User Stories

1. As a 사용자, I want 한 원천에서 필요한 기록을 받아, 불필요한 데이터 연결 없이 결과를 확인한다.
2. As a 분석 사용자, I want 관측된 값의 정확한 집계를 받아, 초기 가설의 근거를 살핀다.
3. As a 사용자, I want 실제 사용한 원천과 요청을 추적해, 결과를 다시 검토한다.
4. As a 사용자, I want 표본과 모집단을 구분해, 일부 관측의 합계를 전체로 오해하지 않는다.
5. As a 사용자, I want 필수 역할이 빠진 결과를 부분 결과로 받아, 누락 자료의 탐색이 이어지게 한다.
6. As a 사용자, I want 실행한 연산의 설명을 받아, 존재하지 않는 결합/동일시를 믿지 않는다.
7. As a 사용자, I want 날짜 조건과 미상 날짜 처리가 유지돼, 다른 시점의 기록이 섞이지 않게 한다.
8. As a CLI 사용자, I want 구조상 결과만 나왔을 때 실패 상태를 유지해, 미검증 결과를 완료로 자동 처리하지 않는다.
9. As a MCP 사용자, I want 같은 근거·정밀도·판정을 받아, 사용 도구에 따라 결과가 달라지지 않게 한다.
10. As a 운영자, I want 원문과 credential 격리가 유지돼, 새로운 연산이 모델 권한을 넓히지 않게 한다.
11. As a 유지관리자, I want 공간 연산만 허용하던 예외와 낡은 설명을 제거해, 하나의 실행 계약을 관리한다.
12. As a 감사자, I want 과거 실패 기록은 당시 상태로 남아, 현재 구현의 성공으로 소급 해석되지 않게 한다.

## Implementation Decisions

- 기존 공통 목표 실행 module과 typed Composition을 사용한다. 새 엔진/저장소/의존성/질문별 라우팅을 만들지 않는다.
- 실제 base 관측이 있으면 joins가 비어도 실행한다. 기존 source 참여·output lineage·시간·예산 검사는 그대로 적용한다.
- 현재 산출물 상태는 모든 실행 연산에 공통인 `sample_executed`로 정리한다. 이것은 실행 결과이며 승인 등급이 아니다.
  이전 experimental `sample_joined`를 현재 writer에서 제거한다. 과거 진단 fixture의 값은 변경하지 않는다.
- join이 있는 경우에만 불일치 행 제외를 설명한다. no-join의 시간 필터 오류를 공간 전용 오류로 표시하지 않는다.
- 결과 행/해시/원천 요청과 기존 정밀도를 보존한다. planner가 원문을 보는 정책은 이 slice에서 변경하지 않는다.
- CLI/MCP의 같은 계획 동작·부분/검토 필요 결과를 보존하고 도구 설명을 실제 계약에 맞춘다.

## Testing Decisions

- 사용자 위임에 따라 기존 public seam인 Start/Advance/Run, 실제 CLI command와 MCP JSON-RPC를 선택한다.
- 외부 검색/취득만 fixture로 대체하며 Engine의 성공 상태를 fake runner로 주입하지 않는다.
- 순서: projection 실패 재현 → 구현 → 독립 상수 합계/정밀도 → 누락 역할·population·시간/lineage 음성 → CLI/MCP 동등성.
- 기존 spatial lineage/identity/time, 불변 계약, 세션 격리와 과거 진단 보존 회귀를 함께 실행한다.
- 이 fixture 검증은 PK 없는 실제 자율 탐색이나 원천 의미 검증을 대신하지 않는다.

## Out of Scope

이 slice의 범위 밖: 자동 의미 승인, 모델용 근거 읽기, 영속 지식 재사용, 사업 가설 출력,
모집단 전체 연산 및 G1–G5 전체 완료. 이 항목들은 INTENT의 범위 밖이 아니라 지도에 남은 후속 작업이다.

## Further Notes

중단된 구현의 재개와 불필요한 변경 정리는 사용자 2026-09-08 위임에 따른다.
보존할 원천·실패 oracle을 임의로 삭제하지 않으며, 새 경로를 검증한 뒤 대체된 제약·설명만 정리한다.

## 검토 중 재계획 — 2026-09-08 추가 계약

[주장별 근거·재계획 결정](https://github.com/JungHoonGhae/odeduck/issues/51)의 재계획 부분이다.
아래 전이만으로 전체 결정을 완료하지 않는다. 원천 보고의 후속 부분은
[별도 검토 계약](goal-source-report-review-v1.md)에 있으며 계산·가설의 의미 승인은 남아 있다.
[주장 근거의 1차 자료 검토](../research/claim-scoped-evidence-primary-sources-2026-09-08.md)는
원천 지지와 목표 충족을 구분하는 후속 판단 자료이지 자동 승인 구현의 근거가 아니다.

사용자는 최초 실행 뒤에도 부족한 근거·대응표·대체 원천을 같은 목표 안에서 찾고, 수정 결과를
실행하며, 이전 결과와 새 실패를 구별할 수 있어야 한다. 기존 Start/Advance/Run, CLI command,
MCP JSON-RPC를 사용자 위임에 따른 검증 seam으로 재사용한다. 새 엔진이나 resume 저장소는 만들지 않는다.

- `review_required`는 `exploring`과 함께 진행 가능한 상태다. Run도 여기서 다음 계획을 요청한다.
  같은 목표·불변 GoalContract·수신자 공개 정책·원천·누적 예산·고정 만료 시간을 유지한다.
- 허용된 다음 행동을 실제 소비하면 `exploring`으로 돌아간다. 새 실행의 판정이 다시 검토 필요인지
  부분 결과인지를 정한다. stale/replay/취소 등 행동 전 거부는 revision·기존 결과·상태를 바꾸지 않는다.
- 재계획은 기존 행동으로만 한다. 불변 Composition을 같은 ID로 재실행하지 않는다. 대안은 새 ID와
  남은 조합 예산을 사용한다. 새 근거를 읽었다고 이전 결과의 의미 승인을 해제하지 않는다.
- Evaluation의 `executionRevision`과 `compositionId`가 어느 실행의 판정인지 고정한다. 검색·추가 관측
  중 마지막 Artifact/Evaluation을 보존하되 세션의 최신 revision에서 실행됐다고 표현하지 않는다.
- 알려진 새 Composition의 실행을 시작하면 이전 Artifact/Evaluation을 현재 결과에서 제거한다.
  실패는 해당 ExecutionRecord에 남고, 성공/부분 결과는 새 실행의 판정과 산출물로 교체한다.
  원래 관측·recipe·실행 이력은 보존한다. 이전에 반환한 detached 결과 사본은 바꾸지 않는다.
- 총 단계 한도는 두 진행 상태에 동일하게 적용한다. 마지막 단계가 검토 필요여도 `budget_exhausted`로
  종료하며 산출물과 검토 판정은 보존한다. output_ready/abstained/blocked/expired 등은 재개하지 않는다.
  Run의 한 번뿐인 replay 보정도 같은 Engine에서 다시 Run한다고 충전되지 않는다.
- 원문 없는 PlanningView, 허용된 선택 Evidence Packet, 한 시간 만료와 MCP 소유권은 그대로다.
  추가 근거가 없으면 이유를 남겨 abstain할 수 있으며 CLI는 이를 성공 종료로 처리하지 않는다.

TDD 검증은 실제 Engine의 검토 도달 → 근거 읽기/대안 탐색 → 수정 실행을 사용한다. 검토 후
부분·실패·예산·만료·재실행·계약 약화 음성을 포함하고 CLI/MCP에서 같은 전이를 관측한다.
외부 취득·모델만 fixture로 대체한다. 이 상태 전이 검증은 독립 실원천 정답이나 G1–G5의 자율 완주
증거가 아니다. 자동 승인, 프로세스 간 복원, 장부 재사용은 후속 범위로 남는다.

## 원천별 선집계 — 2026-09-08 추가 계약

고정 G4의 인구는 행정기관별 기록이고 학교 표는 군·구별 집계다. 결합 후에만 합산하면 학교 수가
원천 인구 행 수만큼 반복된다. I6/M3의 이 차이를 해결하기 위해 같은 Engine에서 원천을 먼저 집계하고
그 결과를 다른 관측과 연결한다. G1의 지역별 수요·공급 비교에도 같은 연산을 사용할 수 있으며 PK·분야별
분기를 두지 않는다. 사용자 위임으로 Start/Advance/Run, CLI command, MCP JSON-RPC를 검증 seam으로 유지한다.

- 기존 `sample`의 `reduce`로 원본 observation ID·정확한 rowsSha256과 GroupBy/Measures/Aggregates를
  지정한다. 필드는 원본의 비한정 열 이름이며 aggregate.field만 앞서 지정한 measure alias를 쓸 수 있다.
  원래 PK·요청 delivery를 유지한다. 새로운 asset/where/params/operation/rowPath/layout/scan/nearest/XLSX/ZIP
  선택과 혼합하지 않는다. 네트워크 취득을 다시 하지 않는다.
- 원본 1–1000개 보유 행 전체를 입력으로 사용한다. groupBy는 0–8개, measures는 최대 16개,
  aggregates는 1–8개다. 기존 정확한 십진 변환·sum_fields·sum·count를 재사용한다. null 그룹은 보존하고
  누락된 합산 항목은 부분 합이나 0으로 바꾸지 않는다. 원본 열과 alias를 혼동하거나 덮어쓰지 않는다.
- 계산 결과는 별도의 불변 `DERIVED` 관측이다. 원본은 그대로 두며 원천 내용/요청/행 revision, recipe와
  각 그룹에 기여한 모든 보유 행의 1-based 위치를 `reduction`에 기록한다. 위치는 물리 CSV 행 번호가
  아니며 원본 관측의 CSV/XLSX provenance로 이어진다. 원천의 선언을 새로 생성된 필드의 선언으로 복사하지 않는다.
- 결과는 기존 sample/bytes/단계/만료 예산을 사용한다. 별도 무제한 계산·실행·저장 경로는 만들지 않는다.
  실패 시 부분 그룹 관측을 남기지 않는다. 원천 요청과 실패는 기존 sampleAttempts/Gaps에 보존한다.
- 결과 필드의 역할과 artifact.sources는 원본까지 추적한다. Evidence Packet의 `computed_group` 주소는
  한 원본 행이 아니라 그룹을 가리키며 기존 선택 공개 범위를 유지한다. 원천 보고 검토로 승인하지 않는다.
- 동일 원천에서 나온 그룹끼리, 그룹과 원본/동일 원본의 공간 결과를 다시 결합하는 혼합 단위 연산은
  명시적인 배분 계약이 없으므로 거부한다. 다른 원천에 1:N으로 붙인 그룹 값을 같은 합계에 반복 기여시키는
  경우도 기존 sum의 원본 반복 guard를 유지한다. 중첩 선집계와 집계 그룹의 공간 원점 사용은 지원하지 않는다.
- 시간 검사는 그룹에 기여한 원본 기록 각각의 기간을 사용한다. 그룹을 만든 뒤 일부 기여 기록만
  날짜로 제외하면 합계가 틀리므로 모든 기여 기간의 교집합으로 그룹 전체를 검사한다. 필요한 날짜 선택은
  원천 취득 조건 또는 날짜를 포함한 그룹 단위에서 먼저 처리한다. 기준일·유효 기간 의미 승인은 별도다.

검증은 단순 합계/연결, 같은 값의 별개 원본, 큰 정수, null·잘못된 필드·revision·혼합 선택·중첩·시간·
공간·출처 혼동, CLI/MCP 공통 결과를 포함한다. 기존 G4 독립 원천 162행을 11그룹으로 집계한 값은
군·구별 기대값 및 304,280명 합계에 대조한다. opt-in 실조회도 오데덕의 공식 검사·CSV/XLSX 읽기를 사용한다.
서해구/서구의 미확정 대응은 그대로 남기며, PK를 제공한 실행 검증을 자율 발견이나 G4 완료로 세지 않는다.
수치와 재현 명령은 [실행 검증 기록](../research/goal-result-execution-validation-2026-09-08.md#원천별-선집계-후속-검증)에 둔다.

## 미대응 기록 추적 — 2026-09-08 추가 계약

G4의 전체 비교에서 일부 지역만 대응돼도, 제외된 지역을 일반 면책 문구로 숨기지 않는다. 기존
Engine의 ordered inner join과 선택 근거 읽기를 사용한다. nullable 출력/outer join이나 임의 명칭
대응을 추가하지 않으며, 원래 G4 전체 비교·기준일 해석·population 검증을 완료한 것으로 세지 않는다.

- 각 `JoinMetric.unmatchedLeft`는 그 단계에서 대응하지 않은 왼쪽 입력 tuple마다
  `{observation ID: 1-based retained row}`를 기록한다. 이미 결합한 여러 원천의 위치를 함께 보존한다.
  `unmatchedRight`는 해당 `right` 관측에서 어떤 왼쪽 입력과도 통과하지 못한 보유 행 위치다.
- key 일치뿐 아니라 lineage·scope·시간 조건까지 통과해야 matched다. null/누락 키도 미대응으로
  남긴다. 빈 결합 실패에도 실행 이력의 위치를 보존하고 같은 목표에서 근거를 읽거나 재탐색한다.
  행 수 초과 등 단계 계산 자체가 실패하면 그 단계의 부분 지표를 완성된 진단처럼 반환하지 않는다.
- 위치는 CSV/XLSX 물리 위치가 아니다. 관측 revision 및 기존 provenance를 거쳐 원문/계산 그룹과
  연결한다. 그룹의 원본 구성원은 기존 reduction 근거를 사용한다. 값·원문·새 식별자는 기록하지 않는다.
- 범위 설명은 단계별 제외 수와 위치의 해석을 제공한다. 중간 tuple 수는 고유 개체 수가 아니며,
  단계별 미대응의 합은 최종 미대응 집합이 아니다. 후속 결합에서 제외될 수 있다. 사건 부재나
  자동 canonical 동일성을 주장하지 않고, 행 값은 기존 권한·예산의 `read_evidence`로 확인한다.
- 관계 검토는 대응된 출력의 재현뿐 아니라 직접 관측의 미대응 행에 대한 key/scope/time 비교 필드도
  공개된 packet에서 확인하고 결합 지표를 재현한다. 무관한 미대응 출력 값이나 집계의 비공개 원본
  전체를 추가 전송하지 않는다. 단일 원천 시간 선택은 이 결합 진단과 구분한다.

검증 seam은 위임된 Start/Advance/Run·CLI command·MCP JSON-RPC다. 양쪽 미대응, 다단계 1:N,
빈 결합, null/scope/time 제외, 값 없는 PlanningView, 반환 사본 격리 및 검토 전 공개를 검사한다.
기존 G4 독립 원천과 실제 오데덕 취득을 재사용해 10개 대응 쌍과 미대응 인구 그룹/학교 행을 대조한다.
이는 원천 지정 실행 검증이지 무힌트 자율 발견·별도 모델 정확도·원래 목표 전체 완료가 아니다.

### 미대응 원천 값의 결과 표

원래 질문이 비교할 수 없는 부분까지 요구하면 위치 지표만으로 결과를 대신하지 않는다.
`composition.reportUnmatched`에 1–16개의 서로 다른 `observation.field`를 지정하면 성공한 결합
실행의 `artifact.unmatched`에 각 단계의 미대응 tuple을 별도로 투영한다. 기존 Start/Advance/Run,
CLI command, MCP JSON-RPC를 위임된 검증 seam으로 유지한다.

- 필드는 해당 조합의 직접 원천/계산 그룹에서 관측된 열이어야 한다. 각 항목은 1-based joinIndex,
  left/right, 원천별 보유 행 위치, 선택한 실제 값과 missing을 보존한다. 그 tuple에 없는 원천의
  값은 만들지 않으며 실제 null/빈 문자열/0은 원문대로 둔다.
- 모든 단계의 미대응 tuple을 보존한다. 이 표는 결합/시간/집계의 입력이 아니며 값의 합계나
  고유 개체 집합이 아니다. 미대응은 실제 부재·지역 비동일성·학교 수 0을 뜻하지 않는다.
- 기본은 기존 결과 그대로다. 명시한 경우 대응 결과와 미대응 표가 합쳐 기존 1000행/2 MiB
  결과 예산을 공유한다. 초과 시 새 산출물을 반환하지 않으며 조용히 잘라내지 않는다. 빈 결합이나
  실패한 실행을 성공으로 바꾸지 않으며, 그 경우 기존 실행 지표로 재계획한다.
- PlanningView에는 값이 들어가지 않는다. 분석 검토 전에 미대응 표의 실제 선택 필드도 기존
  `read_evidence`로 공개돼야 하며, 로컬 원본과 공개 근거로 표를 재현한다. 비교 지표만으로 값을
  전송하지 않는다. 입력/근거 hash·검토 권한·만료·원래 목표 적합성 검사는 유지한다.

양쪽 미대응·다단계 tuple·결측/0·예산·반환 사본 격리·공개 전 검토 차단을 회귀로 고정하고, 원래
G4의 10쌍과 미대응 인구/학교 기록을 기존 독립 reference에 대조한다. 원천 지정 재생은 자율 완주나
공식 명칭 대응의 의미 승인이 아니며 기존 GoalContract/Goal Oracle은 낮추지 않는다.
