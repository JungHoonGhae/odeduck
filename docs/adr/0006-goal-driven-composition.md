---
status: accepted
date: 2026-09-07
---

# Goal-driven discovery with bounded composition execution

사용자는 키워드와 endpoint를 모른 채 서로 먼 분야의 데이터를 조합하여 목표를 이루려 한다.
첫 Anchor 하나와 세 번의 검색에 고정된 v1은 중간 변환 자료 탐색과 검사 실패 후 재계획을
표현하지 못한다. Discovery Goal을 유지하는 공통 module에 후보, 관측, 여러 Composition,
Discovery Gap, 예산을 모으고 CLI planner와 MCP host가 동일한 진행 interface를 사용한다.

ADR-0002의 MCP host planning과 ADR-0004의 bounded·candidate/evidence 구분은 유지한다.
v1의 검색·카드 계약은 호환 유지하지만 목표 진행에는 고정 Anchor나 세 단계 제한을 적용하지 않는다.
목표 진행용 MCP 도구 하나와 CLI 명령 하나를 추가한다. 검색·검사·호출별 도구를 재조합하도록
지시문에만 의존하는 대안은 진행 상태와 실행 근거가 두 caller로 흩어져 채택하지 않았다.

실행은 bounded rows에 대한 typed 결합·집계이며 임의 코드나 endpoint 실행을 허용하지 않는다.
표본 실행 성공은 sample_joined이고 기존 장부의 sample_verified 승격과 별개다. namespace·coverage의
해석과 실세계 목표 효과는 명시적 검토 대상으로 남긴다. 외부 CLI planner에는 원문 행을 보내지 않는다.
원문은 세션 메모리에만 두고 최종 사용자에게 요청된 표본 산출물로 반환한다.

저장 기술은 기존 카탈로그와 선택형 의미 인덱스를 유지한다. 목표 내의 작은 결합 그래프를
실행하는 데 전역 graph DB나 의미 유사도를 사실 edge로 저장할 필요가 없다.

## 2026-09-08 amendment

[Goal Result 실행 계약](../specs/goal-result-execution-v1.md)에 따라 결합을 선택 연산으로 확장한다.
한 원천의 조회·집계와 다중 원천 결합은 같은 실행기를 쓰며 현재 산출물 이름은 `sample_executed`다.
위의 `sample_joined`는 최초 결정 당시 이름으로 보존한다. 의미 승인·원문/키 격리와 예산 계약은
이 변경으로 완화하지 않는다. 계획기가 읽을 근거의 범위는 별도 결정 전까지 현행 정책을 유지한다.

선택 근거의 공개·수신자·보관 정책은 이후 [ADR-0007](0007-selected-evidence-and-reuse.md)이 대체한다.
