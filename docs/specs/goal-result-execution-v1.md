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
