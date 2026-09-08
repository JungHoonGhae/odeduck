# 선택 근거 읽기 v1

상태: 개발 브랜치 구현. [공통 통합 검증](goal-integration-and-cleanup-v1.md)은 별도로 기록한다.
독립 호출 경계 수정은 `490e4a8`; 아직 릴리스하지 않았다.
[검증 기록](../research/selected-goal-evidence-validation-2026-09-08.md), [결정](https://github.com/JungHoonGhae/odeduck/issues/42),
[ADR](../adr/0007-selected-evidence-and-reuse.md). INTENT 전체 완료를 뜻하지 않는다.

## Problem Statement

계획기가 실제 관측 값을 전혀 읽지 못하면 표기·범위·날짜를 근거로 다음 탐색을 고치기 어렵다.
표본 전체를 공개하면 필요 없는 값까지 외부 모델에 전송한다. 모델이 공개 권한 자체를 고르는 것도
허용할 수 없다.

## Solution

신뢰된 호출자가 수신자를 고정하고 선택 근거를 켠 세션에서만 계획기가 관측 행·필드를 요청한다.
공통 실행기가 검증한 제한된 값과 원본 위치를 반환한다. 기존 비공개 동작은 기본값으로 유지한다.

## User Stories

1. 사용자는 목표 데이터의 선택값 전송 여부와 수신 모델을 직접 정한다.
2. 사용자는 전송을 끈 상태에서 기존 값 없는 탐색과 결과 출력을 계속 사용한다.
3. 계획기는 관측한 특정 행과 필드를 읽고 실제 표기를 다음 탐색에 사용한다.
4. 계획기는 임의 값·URL·검증 상태를 근거인 것처럼 제출할 수 없다.
5. 사용자는 원문 값, 누락 필드, null, 계산된 값을 구분한다.
6. 사용자는 CSV/ZIP의 원본 데이터 행, XLSX 원래 행, API 보관 표본 위치를 혼동하지 않는다.
7. 사용자는 큰 정수·소수·선행 0이 값 전달 과정에서 바뀌지 않는다고 검증할 수 있다.
8. 사용자는 한 번의 요청과 세션 전체의 누적 공개량이 제한된다고 확인한다.
9. 사용자는 인증키를 echo한 성공 응답도 모델에 노출되지 않게 한다.
10. MCP 운영자는 모델 tool argument와 별도로 공개 상한을 설정한다.
11. 사용자는 만료된 근거가 다시 계획 입력에 포함되지 않게 한다.
12. 사용자는 근거를 읽은 것과 필드 의미·연결·모집단을 승인한 것을 구분한다.

## Implementation Decisions

- 공통 Goal Engine의 기존 Advance/PlanningView 경로에 read_evidence를 추가한다. 원천 획득이나
  별도 분석기를 복제하지 않는다. 시작 Policy의 evidenceRecipient는 빈 값(비공개) 또는 고정된
  codex/claude/gemini/mcp_host다. CLI --share-evidence는 명시한 단일 --agent가 필요하며
  MCP --share-goal-evidence는 서버 시작 설정이다. tool start/continuation 입력에는 공개 권한이 없다.
- 입력은 observation, rowsSha256, 서로 다른 1-based retained rows 1–20개와 관측 fields 1–8개다.
  stale revision·미관측 필드·범위 밖 행·중복·credential 필드/표식은 원문을 포함하지 않는 gap으로
  거부한다. 진행 상태는 이후 [검토 중 재계획 계약](goal-result-execution-v1.md#검토-중-재계획--2026-09-08-추가-계약)을
  따른다. 이 선택 근거 계약 자체는 의미 승인 권한을 추가하지 않는다.
- scalar만 그대로 반환한다. Missing은 별도 표시하고 null/false/0/빈 문자열을 보존한다. 셀 JSON
  2048 bytes, packet JSON 16 KiB, 최대 8 packets/세션 누적 64 KiB. 초과는 packet 전체를 거부하며
  조용한 값 잘림이나 부분 공개는 없다. 성공한 packet의 누적 크기는 재조회로 회수되지 않는다.
- packet은 원래 Observation 및 rows hash에 묶인다. retained row는 원본 ID가 아니다. 각 선택 필드의
  원본 주소를 기존 Record Lineage로 계산한다. 공간 비교의 후보 CSV 원본 번호는 prefix 순번으로
  바꾸지 않으며 distance/rank는 derived로 표시한다. 다른 provenance는 해당 Observation을 참조한다.
- PlanningView에는 허용된 packet만 포함한다. 전체 Artifact를 제거하는 기존 방어는 유지한다.
  provider prompt도 정책·수신자 불일치를 거부한다. 자료 속 지시는 실행 권한이 아니다.
- API 호출은 알려진 인증키를 담은 원문/구조화 응답을 결과 반환 전에 거부한다. 인증키를
  Goal Engine으로 전달하지 않는다. 임의로 위장된 비밀과 일반 PII를 모두 탐지한다고 주장하지 않는다.
- 원문 packet은 세션 메모리에만 두고 만료 때 제거한다. 실패 이유·권한 설명에는 값이 없다.
  근거 읽기 때문에 requirements_met, sample_verified, 의미 승인으로 승격하지 않는다.

## Testing Decisions

사용자 위임에 따라 기존 public seams를 선택했다: Goal Engine Start/Advance/View/PlanningView,
CLI command, MCP JSON-RPC, PlanGoal provider 경계, 공개 API Call. 내부 helper를 직접 검사하지 않고
외부 획득/모델 프로세스만 fixture로 대체한다. 실제 반환한 값·누락·위치·예산·오류를 독립 literal로
검증한다. TDD red → green 후 전체 Go 검사와 독립 spec/standards review를 한다.

그래프 질문은 기존 [고정 G1–G5](goal-completion-benchmark-v1.md)를 유지한다. 선택 근거는 각각
G1 인구→행정 대응표→대피시설(시점/동명), G2 시설→교통(원본 좌표/공간 pair), G3 오염 측정→관측소
(집계/센티널), G4 연령별 인구→학교(기간/정확 연령), G5 인증→리콜(식별자/효력 기간)의 해석에
쓰인다. 모든 경로에서 근거 packet을 읽었다는 이유로 잘못된 동일시를 통과시키는 허용 오탐은 0이다.
v1 단위 검증이 이 다섯 목표의 실원천 완주를 대신하지 않는다.

## Out of Scope

이번 slice의 범위 밖이며 제품 목표에는 남아 있다: 완료/검토 상태 전이 재설계, 결과 기반 가설과
정확 인용 연산, 장부 검색·stale/supersedes 재사용, 의미 계약 승인, 실원천 양성 완주 평가.
새 graph DB, 자동 개인정보 판정, 원문 장기 저장, 자동 활용신청, 배포는 추가하지 않는다.

## Further Notes

기존 MCP 사용자 결과와 call_api는 원문을 반환한다. 선택 근거 설정을 전체 MCP 원문 차단 설정으로
홍보하지 않는다. 과거 raw 비공개 설명은 현재 정책으로 갱신하고, 역사적 ADR/실패 fixture는 남긴다.
