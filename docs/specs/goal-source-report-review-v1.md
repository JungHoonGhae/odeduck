# 원천 보고의 분리 검토 v1

상태: 제한된 원천 보고 경로 구현·개발 검증 완료. 실제 모델 응답과 실패를 포함한 분모는
[검증 결과](../research/source-report-review-validation-2026-09-08.md)에 보존한다.
[주장별 근거·재계획 결정](https://github.com/JungHoonGhae/odeduck/issues/51)의 원천 보고 부분이며,
G1–G5의 범위나 전체 완료 기준을 바꾸지 않는다.

## Problem Statement

실제로 실행한 원천 보고도 영구적인 단일 의미 검토 값 때문에 완료될 수 없다. 반대로 계획기의
자기 승인이나 원천 인용의 존재만으로 질문에 맞는 답이라고 판정해서는 안 된다.

## Solution

신뢰된 시작 설정으로 선택한 검토자가 계획 이력 없는 별도 요청에서 원래 질문과 산출물을 검토한다.
원천 지지와 원래 목표 충족을 따로 기록한다. 기본은 꺼짐이며, 승인 결과도 모델 판단이지 현장 검증이나
진실 보증이 아니다. 검토 미충족·실패는 같은 목표의 추가 탐색으로 돌아간다.

## User Stories

1. As a 사용자, I want 자료에 실린 사실을 출처와 함께 받아, 불필요한 현실 효과 검증을 기다리지 않는다.
2. As a 사용자, I want 원래 질문의 범위를 검토해, 계획기가 더 쉬운 질문으로 바꾸지 못하게 한다.
3. As a 사용자, I want 필드별 지지 근거를 받아, 일부 맞는 값이 답 전체를 승인하지 않게 한다.
4. As a 사용자, I want 원천 보고와 현재 상태·계산·가설을 구분해, 더 강한 결론을 오해하지 않는다.
5. As a 사용자, I want 검토자와 한계를 확인해, 모델 검토를 사람/제공기관 승인으로 오해하지 않는다.
6. As a 사용자, I want 외부 전송을 직접 설정해, 검토 도입이 원문 공개 범위를 넓히지 않게 한다.
7. As a 사용자, I want 미지원 연산도 기존대로 실행·재계획해, 새 검토가 기능을 없애지 않게 한다.
8. As a 운영자, I want 검토 비용·실패·근거 revision을 남겨, 무한 재검토와 오래된 승인을 막는다.
9. As a CLI/MCP 사용자, I want 같은 실행·검토 판정을 받아, 인터페이스에 따라 신뢰가 달라지지 않는다.

## Implementation Decisions

- 기존 Engine에 `review_result` 행동을 추가한다. 모델 입력은 현재 composition ID뿐이며 판정·원문·
  검토자 설정을 입력하지 못한다. Review는 신뢰된 dependency를 통해 실행하고 Engine이 결과에 묶는다.
- 첫 지원은 한 관측의 원천 필드 보고다. join/aggregate/measure/spatial 결과의 의미 승인, 인과/현재
  안전성·모집단/사업 가설은 이 검토 경로에서 승인하지 않는다. 기존 실행 기능은 유지한다.
- 실행의 구조 검사가 모두 통과해야 한다. 원래 Goal와 불변 Contract를 함께 검토한다. 검토자는
  원천 보고만으로 원래 질문이 충족되는지 따로 판단한다. 더 강한 요청을 보고로 낮추어 승인하지 않는다.
- 입력은 최대 20개 retained record, 8개 관측 필드의 이미 공개 허용된 단일 Evidence Packet과 실제
  원천 메타데이터·요청·recipe·실행 결과다. 모든 retained row와 선택/시간/선택조건 필드를 포함해야 한다.
  결과 필드의 값은 이 packet으로 재현 가능해야 하며, 전체 표본을 새로 공개하거나 잘라 승인하지 않는다.
- 별도 tool-free 요청은 기존 coding-agent CLI adapter를 재사용한다. 계획 이력·oracle·예전 검토 답을
  보내지 않는다. 같은 provider도 별도 context일 뿐 통계적 독립·정답 보증은 아니다. 자동 fallback 없음.
- 검토는 `source_report`의 출력별 지지와 원래 목표 적합성에 supported/unsupported/insufficient,
  이유와 실제 packet ID를 반환한다. 빠진/중복/가짜 ID, 잘못된 형식, 원문 credential material, 실패는
  승인하지 않는다. 모든 필수 출력과 목표가 supported여야 `output_ready`가 된다.
- Engine이 검토 입력 hash, 실행 revision, 시도 revision, provider와 검토 계약 버전을 기록한다.
  같은 실행+같은 근거로 재시도하지 못하며 행·필드 순서만 바꾸어도 같은 근거로 취급한다.
  세션당 최대 세 번 호출한다. 만료/취소/오류를 승인으로 바꾸지 않는다.
  새 실행은 이전 검토를 현재 결과에서 제거하고 시도 이력은 보존한다. 만료 시 검토 원문도 폐기한다.
- CLI는 `--review-source-reports`와 명시적인 `--agent`/`--share-evidence`가 필요하다. MCP는 서버 시작
  시 `--review-goals-with=<provider>` 및 `--share-goal-evidence`가 필요하다. MCP host 공개 허용을
  다른 provider 전송 허용으로 재해석하지 않는다. 검토 provider를 모델 tool argument로 바꿀 수 없다.
- `NeedsSemanticReview=false`는 이 제한된 검토를 통과했다는 호환 요약이다. Review의 method/범위/한계가
  함께 출력되며 외부 현실 검증·sample_verified·장부 재사용 승인으로 승격하지 않는다.

## Testing Decisions

사용자 위임으로 기존 Start/Advance/Run, CLI command, MCP JSON-RPC와 외부 모델 adapter의 공개 함수를
seam으로 선택한다. 원천 보고 양성부터 TDD로 구현하고, 입력 공개·revision·구조/목표/주장별 음성을 보강한다.
모델 fixture로 상태 전이만 검증한 것은 실제 양성 검토 증거로 세지 않는다. 독립 원천에 고정된 최소 두 분야의
양성/강한 결론 음성을 실제 격리 reviewer로 각각 세 번 실행하고 원 응답·전체 분모를 보존한다. 이는 개발
calibration이지 held-out 평가/오류율 보증이 아니다. 실패 시 해당 경로를 유효하게 검증했다고 보고하지 않는다.
G1–G5 원문 목표와 기존 oracle은 변경하지 않는다. 별도 단순 양성은 추가 개발 사례로 명시한다.

개발 calibration 명령 handler도 사용자 위임에 따른 검증 seam이다. 외부 inspection/model만 대체하고
실제 Engine으로 성공·모델 오류·오탐·원천 누락의 예정 12개 기록 및 기존 출력 파일 비덮어쓰기를 검사한다.
검토 adapter는 calibration이 명시적으로 저장할 수 있는 원 응답을 별도로 반환하지만 CLI/MCP 목표에는
구조화된 Assessment만 전달한다. 원 응답 보존 상한은 1 MiB이며 초과 시 잘림을 표시하고 승인하지 않는다.

## Out of Scope

이 slice 밖: 계산·가설의 자동 승인, 인물/시설 identity 승인, population 완전성 승인, 장부 재사용,
외부 현장 진실 보증, 모든 G1–G5의 자율 완주. 제품 목표에서는 계속 미완료다.

## Further Notes

[원천 지지의 1차 자료 검토](../research/claim-scoped-evidence-primary-sources-2026-09-08.md)는 출처 지지와
질문 적합성이 별개임을 뒷받침한다. 모델 검토의 실제 정확도는 그 문헌이 아니라 위 calibration으로 점검한다.

## 관계·계산 검토 — 2026-09-08 후속 계약

상태: 경로 구현. 최초 개발 calibration은 9/12로 미통과였고, 같은 질문·정답의 실제 취득 후속은
12/12 기대 일치다. scanner의 선택/전체 검사/보관 근거로 환경 양성의 누락을 해결했다.
[실패 포함 검증](../research/source-report-review-validation-2026-09-08.md#후속-관계계산-검토의-개발-검증)을
보존하며 범용 의미 정확도·G1–G5 완료로 확대하지 않는다. 원천 보고 v1의 권한·검토 기록은 유지한다. 기존 제외 범위 중
typed 관계·계산을 별도 opt-in으로 확장한다. 이는 I6/I7/M4의 후속이며 G1–G5와 모집단 통과선을 낮추지 않는다.

- 같은 `review_result`와 검토 dependency를 사용한다. 신뢰된 시작 정책의 `ReviewAnalyses`가 켜져야
  관계·계산 검토를 허용한다. CLI `--review-analyses`는 명시적 agent와 `--share-evidence`가 필요하고,
  MCP `--review-goal-analyses`는 기존 수신 provider와 공개 설정에 추가한다. 원천 보고 설정만으로
  계산 검토가 켜지지 않으며 두 종류가 세 번의 검토·같은 만료·실행 revision 예산을 공유한다.
- 지원은 관측 원본과 Source Reduction에 대한 기존 typed 결합·수치 변환·집계·시간 비교다.
  공간 결과와 그 후보 stream 검토, 인과·현재 안전성·사업 가설은 후속이다. 기존 연산은 계속 실행 가능하다.
- Engine은 실제 보유 원본으로 각 Source Reduction과 현재 composition을 재실행해 결과를 대조한다.
  이는 원천 revision에 대한 로컬 재현 검사이지 독립 검산이나 의미 승인이 아니다. 정답의 독립성은
  별도의 원천 oracle 테스트가 담당한다. 원본 행을 검토자로 보내기 위한 우회로로 사용하지 않는다.
- 실제 결과의 값을 이미 공개된 Evidence Packet들로 다시 계산할 수 있어야 한다. 직접 결합되는
  관측은 실제 결합·시간 선택을 통과해 산술/집계에 기여한 모든 행의 필요한 필드를 공개해야 한다.
  Engine이 원본 lineage에서 이 위치를 기록하며, 0이나 서로 상쇄되는 항도 포함한다. 기여하지 않은
  출력 값을 추가 전송할 필요는 없고 전체 취득·결합 범위 지표는 유지한다. 다만 미대응 기록의 비교
  필드와 지표 재현은 [미대응 추적 계약](goal-result-execution-v1.md#미대응-기록-추적--2026-09-08-추가-계약)을 따른다.
  명시적으로 요청한 [미대응 결과 표](goal-result-execution-v1.md#미대응-원천-값의-결과-표)도
  모든 보고 필드가 공개돼야 하며 보유 원본과 공개 근거로 각각 재현한다. 표는 계산 입력이 아니고
  기존 검토 입력 hash에 함께 묶인다. `analysis.outputSha256`은 종전처럼 계산 행의 hash다.
  계산된 그룹을 공개했다면 원본 구성원 전체를
  추가 공개하지 않는다. 원본 구성원은 로컬에서 집계·시간·lineage 검사에만 사용한다. 계산에 쓰인
  원본 필드마다 선택 근거가 있어야 하며 일부 예시 값은 전체 행을 검토했다는 뜻이 아니다.
- 출력은 명시적 Select를 사용한다. 원천별 필드 의미·join key·단위·시간·선택 조건을 검토할 수 있도록
  관련 필드 metadata, 원래 요청·선집계 recipe·모든 구성원 참조·실제 결합 지표를 보존한다. 무관한
  필드 metadata는 검토 입력에서만 투영하고 관측 원본은 변경하지 않는다. 기존 8 packet/64 KiB와
  검토 입력 96 KiB 한도를 유지하며 원문이나 결과를 자동으로 잘라 승인하지 않는다.
- 별도 tool-free 검토는 원래 Goal와 불변 Contract, 출력별 지지 외에 `relations`, `periods`,
  `measurements`, `coverage`를 각각 판정한다. 각 판정은 실제 공개 packet ID들을 인용한다.
  모든 항목과 목표 적합성이 supported일 때만 `output_ready`가 된다. 모델의 판정은 필드 의미와
  근거에 대한 판단이며 source truth, canonical identity, 현장 검증 또는 모집단 인증이 아니다.
- 부족한 헤더 문맥, 미대응 지역, 불분명한 단위·시점은 이유와 함께 남기고 같은 목표에서 추가 근거를
  찾는다. 원래 목표보다 약한 Contract나 면책 문구만으로 완료하지 않는다. 기존 population 구조
  검사를 검토자가 우회하지 못한다. 새 실행에는 이전 검토를 이월하지 않는다.

검증 seam은 위임된 기존 Start/Advance/Run·CLI·MCP·ReviewGoal이다. 공개된 계산 그룹과 비공개 원본을
분리하는 양성부터 구현하고, 근거 누락·가짜 인용·단위/관계/기간/범위 거절·예산/만료를 회귀로 고정한다.
개발에 쓴 실제 원천은 기존 독립 reference와 대조하고, 실제 모델 판정은 원 응답과 전체 예정 분모를 남긴다.
fixture 승인만으로 모델 정확도나 원래 G1–G5의 자율 완주를 주장하지 않는다.

### 같은 파일의 별도 문맥 근거

계산에 쓰인 관측 밖의 헤더·주석은 실제로 읽었어도 기존 검토 입력에서 빠졌다. `ReviewAnalyses`가
켜진 경우, 이미 공개한 다른 원본 FILE 관측의 packet을 `analysis.sourceContext`로 전달한다.
새 행동·설정·임의 문맥 입력은 추가하지 않는다. 원천 보고 v1은 그대로 두되, 연관 문맥이 있으면
별도 관계·계산 검토 계약을 선택해 네 의미 차원과 문맥 인용을 함께 검사한다.

- 연관 조건은 계산 원천의 원본 FILE과 같은 PK, 비어 있지 않은 asset, 같은 ZIP member와 유효한
  SHA256 형식의 content/contract hash다. 다른 파일·revision, hash 누락, 파생/공간 관측은 제외한다.
- 문맥마다 기존 packet, 선택 필드에 투영한 원천 metadata, 실제 sample request와 연결 대상
  observation ID(`targets`)를 보존한다. 원문 선택·주소·수신자는 기존 근거 공개 계약을 따른다.
  계산의 `artifact.sources`/`analysis.sources`와 문맥은 구분하며 원래 관측을 변경하지 않는다.
- 같은 파일이라는 연관은 특정 표/시트에 대한 헤더 적용, 필드 의미, entity identity의 증명이 아니다.
  검토자는 실제 위치·주변 문구로 해석하며, 빠진 적용 근거는 insufficient로 남긴다. 문맥은 계산에
  참여하거나 역할·출력·행 기반 시간 조건·모집단 검사를 충족하지 않는다.
- 문맥 packet ID는 판정에서 인용할 수 있다. 새 문맥 셀은 새 검토 근거가 될 수 있지만, 같은 관측
  revision의 같은 셀을 순서/packet만 바꿔 다시 보내면 추가 검토를 얻지 못한다. 세 번의 호출과
  8 packet/64 KiB 공개·96 KiB 검토 입력 한도, 만료·변조 차단은 유지한다.

기존 G4 독립 reference와 실제 재취득을 대조해 헤더 셀·원본 주소가 계산을 바꾸지 않고 검토에
전달되는 것을 검사한다. 이것은 입력 경계 검증이며 실제 모델의 문맥 해석 calibration은 아니다.
별도 파일의 법령/대응표 문맥, 전체 지역 결과와 의미 승인은 이 slice에서 완료하지 않는다.

### 다른 관측의 선택 근거 연결 — 2026-09-08 후속 계약

같은 파일 자동 연관만으로는 별도 대응표·정의 자료의 이미 취득한 기록을 검토할 수 없다. 기존
Composition의 `support:[{packetId,targets,purpose}]`로 실제 공개한 Evidence Packet과 계산 원천
관측 사이의 **제안된 적용 관계**를 명시한다. 새 저장소·임의 문서 원문 입력·URL 취득은 추가하지 않는다.

- 기존 Start/Advance·CLI command·MCP JSON-RPC·ReviewGoal seam을 재사용한다. `ReviewAnalyses`와
  기존 선택 공개가 켜진 목표에서만 사용한다. packet은 이 목표의 불변 관측 revision에 속해야 한다.
- 최대 8개 서로 다른 packet, 각 1–8개 중복 없는 target 관측과 1–1000 UTF-8 byte purpose를 받는다.
  target은 계산의 직접 원천 또는 그 원본 lineage에 속해야 한다. 근거 자체는 계산에 참여하지 않는
  원본 관측이며 파생/공간 기록이나 다른 목표의 packet은 거부한다. credential material은 받지 않는다.
- `analysis.sourceContext`에 실제 packet·투영 metadata·요청·target과 `proposed:true`, purpose를
  전달한다. purpose와 target은 계획기의 가설이지 출처의 진술이나 적용 승인 값이 아니다.
  같은 파일 자동 문맥은 종전대로 유지하고, 명시한 packet은 그 제안 한 번만 전달한다.
- 문맥은 원래 계산·행 수·원본 lineage·역할·출력·시간·모집단 검사를 변경하지 않는다. 적용되는
  필드·기간·대상은 원문과 원본 위치로 검토해야 한다. 독립 승인 없이 명칭 대응이나 정의를 사실로
  사용하지 않는다. 검토 불가·누락 근거는 같은 목표의 추가 취득·재계획으로 남긴다.
- 같은 실행과 같은 셀 근거의 재포장으로 검토를 충전하지 않는다. 새로운 적용 제안은 새 composition과
  실행을 필요로 하며 기존 6조합·3검토·8 packet/64 KiB·96 KiB 입력·만료·수신자 상한을 유지한다.

공개 seam에서 별도 파일 근거 전달과 무관한 값 비공개, 가짜/중복/비참여 target·권한·예산·변조·
역할 대체 차단을 검증한다. fixture 판정은 전달·상태 검증일 뿐 의미 정확도나 G4 완주가 아니다.
외부 웹의 공식 정의를 원천 revision에 연결하는 취득과 전체 범위 수용 계약은 후속으로 남는다.

### 원래 G4의 실제 검토 진단

위임된 Start/Advance·ReviewGoal seam에서 원래 G4 질문과 독립 citywide reference를 재사용한다.
명시적 provider와 새 출력 경로가 있어야 실행하며, 실제 취득은 원천 hash가 바뀌면 멈춘다.
`historical` 모드는 실제 과거 목록에서 기존 publication 이름의 유일한 버전을 선택해 취득하고 원래
hash·행 위치·합계와 대조한다. 인구는 기존 전체 CSV scanner로 검사/보관 범위를 추가 확인한다.
기본 `live`의 최신 파일 hash 불일치를 자동 복구하거나 과거 실패를 숨기지 않는다.
보존 reference 재생은 별도 명시 모드로만 선택하고 새 취득·자율 발견으로 세지 않는다. 원 질문과
기존 기대값은 유지하며 baseline과 분교 출력 추가의 진단 recipe를 구분한다. 모델에 기대 verdict나
oracle 해석을 보내지 않는다. 각 시도의 입력·원 응답·Engine 결과와 준비 실패를 보존한다.
결과와 한계는 [실행 검증](../research/goal-result-execution-validation-2026-09-08.md#원래-g4의-실제-모델-진단)에
둔다. 이 진단의 예상 보류나 단일 전후 관측은 양성 목표 완주·모델 정확도 통과선이 아니다.
