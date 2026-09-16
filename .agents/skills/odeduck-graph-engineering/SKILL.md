---
name: odeduck-graph-engineering
description: 오데덕의 목표 실행·검색·데이터 연결을 설계하거나, 그 결과의 정확도와 완주 여부를 검증할 때 사용합니다.
---

# 오데덕 데이터·목표 실행 개발

`INTENT.md`의 요청 산출물을 기준으로 설계합니다. 검색 후보, 실제 연결 근거, 목표 충족은 서로 다른
결과입니다. 현재 구현의 완료 여부를 판단할 때는 `docs/specs/goal-driven-completion-plan.md`에서
해당 요구사항의 검증 근거를 확인합니다.

## 작업에 맞는 안내만 읽습니다

| 변경하거나 검토할 대상 | 상세 안내 |
| --- | --- |
| 후보 탐색·재계획, CLI/MCP에 전달되는 실행 지침, 실패 원인 | [탐색과 런타임](references/planning-and-runtime.md) |
| 식별자·연결·근거 재사용·시간 범위·선집계 | [데이터 계약](references/evidence-model.md) |
| 검색 개선, 연결 정확도, 목표 완주, 성능·비용 주장 | [평가](references/evaluation.md) |
| 새 실행 계층·adapter·저장 기술 또는 기존 구조 축소 | [아키텍처](references/architecture.md) |

일반 사용자 데이터 조회는 `skills/odeduck/`을 사용합니다. 이 스킬은 개발 작업용이며 실행 중인
제품의 승인이나 완료 상태를 바꾸지 않습니다. 계획기의 실제 입력은
`internal/goalwork/planning-guide.md`, 버전별 지원 기능은 실행 중인 CLI/MCP에서 확인합니다.

새 작업은 관련 상세 안내에 추가하고, 별도 절차가 필요한 경우에만 reference와 위 표의 행을
추가합니다. 새 스킬은 호출 시점과 필요한 전문 절차가 독립적일 때 만듭니다. 원천별 정답·버전·수치와
실패 기록은 실행 계약·평가 자료에 보존합니다. 지침 구성과 확장 기준은
[공통 안내](../../../docs/agent-skills.md#개발용-skills를-확장할-때)를 따릅니다.
