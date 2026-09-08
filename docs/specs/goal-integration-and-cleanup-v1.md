# 공통 탐색·원천 읽기·목표 실행 통합

상태: 검색 통합 검증 완료; 원천 읽기·목표 실행 통합 대기.
[교체·삭제 순서 결정](https://github.com/JungHoonGhae/odeduck/issues/43).
사용자가 설계와 구현 판단을 위임했다. 이는 기존 worktree 중 목표와 관련된 변경의 통합이며,
전체 INTENT의 완료나 모든 현재 동작의 의미 승인을 뜻하지 않는다.

## Problem Statement

동일한 검색 정책과 실행 규칙이 CLI/MCP에 분산되면 한쪽 수정이 다른 쪽에 반영되지 않는다.
기존 실원천 reader·목표 실행 변경과 최근 기능이 미커밋 상태로 얽혀 있어 재현 가능한 기준점도 부족하다.
작동 중인 후보 탐색을 삭제하거나 근거 없는 전면 재작성으로 문제를 옮기지 않고 이를 정리해야 한다.

## Solution

검색 정책, 외부 획득 adapter, 목표 실행의 소유권을 고정하고 독립적으로 검증 가능한 세 단계로
통합한다. 각 단계는 깨끗한 worktree에서도 테스트할 수 있는 커밋으로 남긴다. 대체된 구현과 사용되지
않는 공개 표면만 제거하며 기능과 실패 근거는 유지한다.

## User Stories

1. 사용자는 CLI/MCP에서 같은 의미 검색 사용·실패·명시적 저하를 확인한다.
2. 사용자는 기존 검색 목록과 연결 후보 카드를 계속 사용할 수 있다.
3. 사용자는 후보 목록을 실제 검증된 연결이나 최종 목표 결과로 오인하지 않는다.
4. 사용자는 standalone discovery의 초기·확장·선택 계획 진단과 실제 선택 카드를 JSON으로 받는다.
5. 사용자는 검색 도중 필수 의미 검색이 실패하면 후보가 성공 결과로 출력되지 않는다고 확인한다.
6. 사용자는 검색에서 발견한 제공형의 실제 스키마·행을 같은 공식 획득 경로로 읽는다.
7. 사용자는 FILE/STD의 원본 위치·정밀도·한계가 목표 산출물까지 유지된다고 확인한다.
8. 사용자는 일반 조회·분석·결합과 선택 근거 읽기를 같은 목표 실행기로 사용한다.
9. 사용자는 CLI/MCP에서 동일한 상세 행동 계약을 읽는다.
10. 사용자는 지원하지 않는 형식·의미·coverage를 명시적인 실패나 미완료로 구분한다.
11. 유지보수자는 새 연산이나 제한을 추가할 때 동일한 상세 안내를 여러 곳에서 수정하지 않는다.
12. 유지보수자는 불변 목표 계약·원천 lineage·시간·정확 수치·자격증명 회귀를 유지한다.
13. 유지보수자는 커밋마다 현재 작업물과 분리해 빌드·테스트하고 이전 기준점으로 비교할 수 있다.
14. 사용자는 과거 실패가 삭제되거나 현재 후보가 과거 성공으로 다시 기록되지 않는다고 확인한다.

## Implementation Decisions

- Catalog Searcher가 선택형 의미 인덱스 loading/stale/embedding/명시적 저하/필수 의미 검색 정책을
  소유한다. CLI, MCP, 목표 획득이 같은 interface를 사용한다. 복사된 호출부의 검색 로직과 이제
  외부 caller가 없는 내부 helper의 export를 제거한다.
- standalone Discovery Runner는 설치된 planner adapter와 scripted test adapter를 교체하는 실제
  seam이다. 검색 후보를 만드는 Generate→Search→Expand→Search→Compose→Search를 유지한다.
  MCP는 host가 계획하므로 이 Runner나 중첩 모델을 호출하지 않는다. 후보 탐색은 Goal Result 실행과
  다른 제품 계약이다. 연결 수 제한은 QueryPlan 한 곳에서 전달한다.
- 포털 계약/키/HTTPS/typed operation은 기존 획득 module이 소유한다. Goal Engine은 불변 목표,
  revision/replay/budget, 관측, typed 실행, 평가, 선택 근거를 소유한다. CLI/MCP는 전달·표시와 신뢰된
  시작 설정만 맡는다. 정책을 adapter와 Engine에서 검증하는 것은 각 신뢰 seam의 방어이며 지우지 않는다.
- 목표 행동의 상세 계획 지침은 Goal Engine이 소유하는 하나의 정적 원본을 CLI prompt와 MCP
  resource에 전달한다. CLI의 JSON 반환/외부 도구 금지와 MCP의 session/tool framing은 caller별로
  유지한다. 전체 Artifact 비공개와 MCP 사용자 결과 반환 차이를 숨기지 않는다. 복제한 상세 지침은
  대체 검증 후 삭제하며 새 문서 생성 framework나 schema registry는 만들지 않는다.
- 목표 실행 상태와 의미 검증은 별개다. 이번 통합은 자동 의미 승인이나 review_required를 성공으로
  바꾸지 않는다. 결과 기반 주장·검토 후 재계획/완료 상태와 장부 재사용은 후속 구현이 필요하다.

### 이행과 rollback

| 단계 | 독립적으로 검증할 동작 | 선행 | 커밋·삭제 경계 |
|---|---|---|---|
| 검색 | CLI/MCP 검색 정책, standalone 다단계 계획과 최종 JSON | 교체 결정 | Searcher/Runner와 호출부 교체, 전역 planner hook·CLI helper 사본 제거, 연결 수 설정 통일 |
| 원천 읽기 | API 기본 포털, STD 검사, CSV/ZIP/XLSX 실제 행·위치 | 교체 결정 | 공식 reader와 metadata 및 CLI/MCP 검사 surface·공개 seam 회귀를 함께 통합 |
| 목표 실행 | 공통 Engine/CLI/MCP 결과·선택 근거·단일 행동 안내 | 검색과 원천 읽기 | 관련 foundation과 최근 두 결과 기능, 의도/설계/평가 문서를 연결하고 통합 미완료 이슈를 정리 |

각 커밋은 이전 커밋에서의 diff와 테스트 근거를 갖는다. 임시 detached worktree로 그 커밋만 검사한다.
실패하면 현재 작업물을 reset하거나 삭제하지 않고 관련 변경을 수정한다. 배포 뒤 문제가 있으면
별도 revert commit을 검토하며 원문 장부·기존 진단을 지우지 않는다. 이 작업은 push/배포를 포함하지 않는다.

### 검토한 대안

- CLI/MCP 각각에 실행기를 두는 전면 교체: 공식 호출·평가·원본 추적이 다시 분산되므로 채택하지 않는다.
- 새 graph DB/workflow server로 이전: 현재 고정 질문은 bounded 검색·join·시간/공간 비교로 실행할 수
  있으며, 지금의 실패는 저장소 부족보다 의미·coverage·결과 계약의 문제다. 측정 근거 없이 도입하지 않는다.
- 현재 기능 위에 새 facade만 추가: 옛 helper·중복 지침이 그대로 남으므로 채택하지 않는다.

다섯 고정 질문은 G1–G5 benchmark를 유지한다. 원본 `goal-completion-benchmark-v1.md`는 현재
worktree에 있으며 목표 실행 통합 커밋에 포함한다. G1 지역/인구→쉼터,
G2 시설→접근성→교통, G3 측정→관측소, G4 연령 인구→학교, G5 인증→리콜에서 Source/Record/Claim을
별도로 추적하고 필드 namespace·기간·원본 위치 없는 동일시를 허용하지 않는다. 이 구조로 lookup과
bounded join을 공유할 수 있지만, 그 사실이 질문별 정답이나 고비용 행동의 승인 근거는 아니다.

## Testing Decisions

사용자 위임에 따라 기존 public seams를 유지한다: CLI command와 MCP JSON-RPC/resource,
Searcher/Discovery Runner, Unified Inspector/공식 caller, Goal Engine Start/Advance/View/PlanningView.
외부 planner/HTTP만 fixture로 대체하고 실제 공통 실행기는 mock하지 않는다. 이전 다단계 CLI 테스트가
Runner 테스트로 옮겨진 뒤 빠진 JSON wiring 검증을 복원한다. 제약 이동은 실패 테스트로 먼저 확인한다.
기존 public 원천/lineage/정확 수치/hard-negative 테스트를 통합하며 같은 의미를 이미 검증하는 내부
wrapper 테스트만 대체 여부를 확인하고 제거한다. 새 가짜 성공 oracle을 만들지 않는다.

각 단계: tidy/vet/전체 test/build, 위험에 맞춘 race, 독립 Spec/Standards review와 깨끗한 커밋 검증.
외부 모델을 설치한 환경에서도 단위 테스트는 실제 provider를 호출하지 않는다.

검색 단계의 공개 seam 테스트에서 QueryPlan에 연결 수 1을 지정해도 확장 검색에서 설정이 사라져
두 후보가 반환되는 실패를 재현했다. 중복 Request 필드를 제거하고 QueryPlan의 제한을 유지한다.
CLI의 실제 카탈로그/검색 경로로 세 계획 진단·최종 선택 카드·명시적 수 제한의 JSON 전달도 검증한다.

2026-09-08 검색 통합 검증: 기존 미통합 변경이 없는 detached staged-tree에서 tidy/브랜드 일치/vet/
전체 test/build와 catalog·discovery·MCP·CLI race 검사 통과. 같은 전체 검사는 원래 worktree에서도
통과했다. 독립 Spec/Standards 검토 지적 사항은 각각 0건이다. 이 검증은 fixture 기반 검색 경로이며
실제 모델의 자율 탐색이나 I1–I10 완료 증거가 아니다.

## Out of Scope

이번 통합 단계에는 추가 저장소·임의 코드 실행·새 portal·원천 scope 확대·자동 신청·배포가 없다.
I1–I10과 실제 G1–G5 완주, 원천 의미 승인·장부 재사용·가설/인용 산출물은 제품 목표에 남아 있으며
통합이 끝났다는 이유로 완료하지 않는다. 무관한 worktree 변경은 통합 대상이 아니다.

## Further Notes

의도는 INTENT, 용어는 CONTEXT, 어려운 결정은 ADR, 현재 typed 계약은 spec, 당시 원천/실패는
research 기록, 추적은 GitHub issue가 소유한다. 같은 상세 규칙을 README·prompt·tool 설명에 복제하지
않고 각 문서의 독자에게 필요한 요약과 원본 연결만 둔다. 현재 ADR-0002/0004/0006/0007 범위 안의
이행이므로 중복 ADR은 추가하지 않는다.
