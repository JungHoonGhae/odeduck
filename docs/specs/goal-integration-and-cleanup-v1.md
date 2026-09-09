# 공통 탐색·원천 읽기·목표 실행 통합

상태: 검색·원천 읽기·목표 실행의 공통 통합 검증 완료. 목표 실행은 v0.19.0에 실험적 기능으로 포함된다.
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

다섯 고정 질문은 [G1–G5 benchmark](goal-completion-benchmark-v1.md)를 유지하며 목표 실행과 함께
버전 관리한다. G1 지역/인구→쉼터,
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

원천 읽기 단계는 CSV의 빈 문자열을 null로 치환하지 않는다. STD는 private inspection handle의
원래 포털 응답만 사용하며, bounded no-redirect streaming과 모호하지 않은 JSON 원문을 요구한다.
기존 buffered STD fallback은 실제 사용자가 없어 제거한다. XLSX/ZIP 구조와 원본 위치 회귀는
보존하고, 위 보완은 공개 원천 읽기 seam의 실패 재현 후 적용한다. 상세 STD 계약은 portal-catalog가
소유하며, 이 통합은 새로운 원천 의미 승인이나 형식 자동 추측을 추가하지 않는다.

2026-09-08 원천 통합 검증: 다른 목표 실행 변경이 없는 detached staged-tree에서 tidy/브랜드 일치/
vet/전체 test/build와 dataset·fetch·apicall·catalog·MCP·CLI race 통과. 원래 worktree의 전체 test 및
tidy/vet/build도 통과했다. Spec 지적 0건; Standards 규칙 위반 0건, XLSX 구조 순회 중복에 대한
비차단 의견 1건. 공통 좌표 형식 검사는 공유하되 구조만 읽는 경로와 선택 값/수식 cache를 읽는
경로의 제한은 유지한다. 전체 parser framework로 합치는 것은 이번 통합에 포함하지 않는다.
실제 CLI의 STD 계약/5행 스키마 검사는 portal-catalog에 기록한다. 원천 검사 통과를 자율 목표 완주로
세지 않는다. 원천 값이 파생 결과까지 유지되는 검증은 다음 목표 실행 통합에도 필요하다.

목표 실행 단계는 `goalwork/planning-guide.md`를 단일 상세 행동 원본으로 둔다. CLI의 긴 prompt
본문과 MCP의 별도 목표 장·연산별 tool 설명을 삭제하고, 같은 원본이 실제 provider 입력과 MCP
resource에 한 번씩 전달되는지 public seam에서 확인한다. 전달/출력/고정 수신자 설정은 adapter에
남긴다. 용어집의 구현 세부 반복은 줄이고, 현재 lineage/원문 공개 설명을 실제 계약에 맞춘다.

`TestNearestPreservesOriginalScalarStatesInEvidenceAndResult`는 원본 1002번 CSV 레코드의 빈 문자열이
파생 선택 근거에서 null이 되는 실패를 재현했다. 복사 중 치환을 제거한 뒤 선택 근거와 zero-join
산출물에서 빈 문자열·null·0·false와 원본 주소를 그대로 확인한다. 의미 승인 상태는 바꾸지 않는다.
수정 후 원래 worktree의 tidy/브랜드 일치/vet/전체 test/build와 diff 검사를 통과했다.

2026-09-08 목표 통합 검증: `7d4666d`를 기준으로 고정한 detached staged-tree `0c32985`에서
tidy/브랜드 일치/vet/전체 test/build와 goalwork·agentplan·MCP·CLI race 검사를 통과했다.
독립 Spec 지적 0건, Standards 규칙 위반·중요 smell 의견 0건이다. 실제 provider 입력과 MCP resource의
단일 안내 전달, 단일 원천/공간 결과, 수신자·예산·원본 주소와 과거 진단 보존을 검토했다.
검토 뒤에는 이 검증 결과와 완료 계획의 부분 진전 연결만 문서에 추가했다. 테스트는 외부 모델이나
새 원천을 호출하지 않는 fixture 검증이며, 이전 dirty foundation을 포함한 통합 기준점을 남긴 것이다.
이 통합으로 최근 두 기능의 커밋 대기는 해소하지만 I1–I10/G1–G5의 미완료 상태는 유지한다.

### 과거 FILE 버전 선택 — 2026-09-08 추가 계약

G4의 최신 파일 교체가 과거 파일 삭제를 뜻하지 않았다. 기존 Unified Inspector에 포털이 실제
나열한 과거 버전의 발견·선택을 추가한다. 최신 검사는 기본 그대로이며 provider/인증 계약을 바꾸지 않는다.
외부 요청·파서·범위의 단일 소스는 [portal catalog](../reverse-engineering/portal-catalog.md#historical-file-editions-verified-2026-09-08)다.

- CLI `inspect --file-history`, MCP `inspect_dataset(fileHistory:true)`는 파일 취득 없이 제한된 목록을
  반환한다. FILE 제공형을 선택하며 API/STD 지정이나 버전 없는 observe/asset 조합은 거부한다.
- `--file-version` / `fileVersion`은 실제 목록의 정확한 ID만 받는다. 선택 시 membership를 다시 확인하고
  그 버전만 검사한다. 사라진 버전·범위 밖 ID·깨진 포털 계약에서 최신 파일로 전환하지 않는다.
  목록은 구조 검사나 `sample_verified` 영수증이 아니다. 실제 asset 검사 이후의 기존 장부 규칙은 유지한다.
- Goal은 기존 `inspect` 행동의 `fileHistory` / `fileVersion`으로 같은 경로를 쓴다. 선택하려면 이 목표에서
  먼저 해당 ID를 발견해야 한다. 선택 검사도 갱신된 목록을 유지해 다른 버전을 비교·재계획할 수 있다.
  반환 PK·선택 버전을 요청과 대조하고 실패 시 이전 Inspection을 보존한다.
- FILE `sample`과 `layout`은 선택 검사와 같은 `fileVersion`을 명시한다. 최신 파일은 생략한다.
  요청 hash·중복 방지·layout pin/refresh에도 버전이 포함된다. Live adapter는 private contract와 다시 대조한다.
  기존 관측은 후속 검사로 덮어쓰지 않는다. `sample.reduce`는 fileVersion을 생략하고 불변 원본 관측을
  가리키며 새 취득이나 최신 원천 metadata를 상속하지 않는다.
- 선택 버전의 원천 선언과 실제 content/contract hash를 결과·검토까지 유지한다. 목록의 이름·등록일이나
  전체 스캔만으로 행의 기간·모집단·동일성을 승인하지 않는다. 예산·공개 상한·승인 규칙을 높이지 않는다.

검증은 위의 동일 public seam에서 최소 익명 HTTP fixture와 실제 공통 Engine으로 수행한다.
CLI/MCP 목록→선택→관찰 및 잘못된 조합 거부, 형식/식별 drift, 목록 한도·fresh membership,
재검사 후 과거 관측의 선집계·결과를 검사한다. 원래 G4 질문·7월 oracle을 보존한 실제 역사 버전 취득과
별도 모델 진단은 [실행 기록](../research/goal-result-execution-validation-2026-09-08.md#과거-버전의-실취득-복구)에 둔다.
이는 I4/I9/M3의 부분 진전이며 I1–I10·G1–G5 완주를 뜻하지 않는다.

## Out of Scope

이번 통합 단계에는 추가 저장소·임의 코드 실행·새 portal·원천 scope 확대·자동 신청·배포가 없다.
I1–I10과 실제 G1–G5 완주, 원천 의미 승인·장부 재사용·가설/인용 산출물은 제품 목표에 남아 있으며
통합이 끝났다는 이유로 완료하지 않는다. 무관한 worktree 변경은 통합 대상이 아니다.

## Further Notes

의도는 INTENT, 용어는 CONTEXT, 어려운 결정은 ADR, 현재 typed 계약은 spec, 당시 원천/실패는
research 기록, 추적은 GitHub issue가 소유한다. 같은 상세 규칙을 README·prompt·tool 설명에 복제하지
않고 각 문서의 독자에게 필요한 요약과 원본 연결만 둔다. 현재 ADR-0002/0004/0006/0007 범위 안의
이행이므로 중복 ADR은 추가하지 않는다.
