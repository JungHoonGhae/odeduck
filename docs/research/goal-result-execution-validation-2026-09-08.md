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

## 원천별 선집계 후속 검증

2026-09-08, 원천 보고 검토 커밋 `eead795` 이후의 추가 구현이다. 기존 단일 원천 실행·검토 기록을
소급 변경하지 않는다. [추가 계약](../specs/goal-result-execution-v1.md#원천별-선집계--2026-09-08-추가-계약)은
공통 Engine의 선집계이며 별도 저장소·패키지 의존성·도메인별 분기를 만들지 않는다.

오데덕의 공식 inspection과 CSV/XLSX reader로 G4 원천을 다시 취득했다. 인구 162행을 11개 군·구로
집계한 만 6–17세 인구는 **304,280명**이다. 모든 군·구 값이 기존 독립
`education-citywide-reference.json`의 제공기관 대조값과 일치했다. 기대값은 이번 실행기로 만들지 않았다.

| 실제 취득 | 보유 행 | 확인한 원천 SHA256 |
| --- | ---: | --- |
| 인구 CSV, 인천 조건 | 162 | `6920fdafd269554d259e8498301f004c9799f499c38832de9352a0e7def916c5` |
| 학교 XLSX, `구·군별!A27:AM37` | 11 | `c8f57fc8e3bd7e70175ff0debd1529a17539e526e436dc244fcea39ed9a0f695` |

원천 날짜는 인구 2026-07-31, 학교 조사 2026-04-01, 학교 표의 행정구역 적용은 2026-07-01로 구분한다.
정확한 이름 결합은 5쌍이었다. 실제 학교 셀의 바깥 공백을 보고 명시적인 기존 `trim` 규칙으로 다시
실행하니 10쌍이 됐다. 원문은 바꾸지 않았고 두 실행을 이력에 유지했다. 일치한 지역의 학교·학생 수가
인구 원천 행 수만큼 곱해지지 않는 것을 독립 기대값으로 확인했다.

남은 한 지역은 인구의 `서해구`와 학교 표의 `서구`다. 이를 전역 이름 치환으로 해결하지 않았다.
결과는 `review_required`이며 원래 G4의 전체 비교·미확정 부분 설명·의미 승인은 미완료다.
이 검증은 PK·필드·학교 셀 범위를 사전에 지정했으므로 자율 탐색 평가도 아니다. 실제 데이터 취득과
계산 경로의 검증이라는 범위를 유지한다. 별도 모델 검토·로그인·활용신청은 실행하지 않았다.

```sh
# 외부 네트워크 없이 보존된 독립 원천과 실제 Engine을 대조
go test ./internal/goalwork -run TestCitywideSourceReduction -count=1

# 공식 메타데이터·원천 파일을 새로 읽고 같은 독립 기대값에 대조
ODEDUCK_LIVE_REDUCTION=1 go test ./internal/goalwork -run TestLiveCitywideSourceReduction -count=1 -v
```

실조회는 11.03초에 통과했다. 파일 hash가 바뀌면 실패하며 기존 reference를 새 결과에 맞춰 덮어쓰지 않는다.
추가 회귀는 계산 그룹의 원본 기여 위치, 비공개 값 격리, 같은 값의 별개 원본 보존, 오래된 revision,
null 항목, 혼합 선택/중첩, 그룹-원본 재결합, 모든 기여 기록의 시간 검사, 공간 원점 오인과
CLI/MCP의 `9007199254740993 + 1 = 9007199254740994`를 검사한다.

`go mod tidy -diff`, `go vet ./...`, `go test ./... -count=1`, `go build ./...`와 브랜드 동기화
검사를 통과했다. `internal/goalwork`, `internal/mcpserver`, `cmd/odeduck`의 race 검사도 통과했다.
이 결과는 구현 회귀 검증이며 위에 남긴 G4의 의미·범위 문제를 해결했다는 뜻은 아니다.

재발 방지 기준은 그래프 엔지니어링 스킬의 ‘결합 전에 관측 단위를 맞춘다’에 모았다. 특정 PK·지역명·
기대 숫자를 스킬에 넣지 않고, 원천의 행 단위·연결 배수·계산 그룹의 기여 기록을 검토하도록 했다.

## 미대응 기록 추적 후속 검증

2026-09-08 `a5082fc` 이후, 기존 G4 원천·질문·독립 정답을 유지했다. 원래 지역 전체 비교에서
누락된 행을 일반 문구로만 알리던 경로에 [단계별 원천 위치](../specs/goal-result-execution-v1.md#미대응-기록-추적--2026-09-08-추가-계약)를 추가했다.
10개 대응 행을 11개로 꾸미거나 서구/서해구를 자동 합치지 않는다.

같은 날 위 opt-in 실조회 명령이 10.81초에 통과했다. 두 파일의 hash는 위 선집계
검증과 같고, 인구 162행→11그룹/304,280명과 학교 11행을 다시 확인했다. 명시적 trim 뒤 10쌍과
왼쪽 미대응 그룹 1개(`서해구`), 오른쪽 미대응 학교 보유 행 9번(`구·군별!A35`의 ` 서구 `, 학교 111개)을
함께 추적했다. 후자의 값은 실행 지표로 원문을 공개한 것이 아니라 그 위치로 `read_evidence`를 호출해
확인했다. 인구 그룹의 원본 구성원은 기존 reduction 근거로 이어진다. 두 원천 날짜와 geography
해석은 앞선 기록을 유지한다. 새 모델 호출·로그인·활용신청·외부 쓰기는 하지 않았다.

회귀는 다단계 1:N의 왼쪽 tuple 보존, 양쪽 미대응, 빈 결합 후 근거 읽기, null/scope/time 제외,
PlanningView의 값 비공개와 반환 사본 격리를 포함한다. 대응된 행만으로 결과를 재생하면 미대응
기록의 비교 근거 없이도 검토에 도달하던 결함을 재현하고, 필요한 비교 필드를 선택해야 모델을
호출하도록 고쳤다. 제외된 행의 무관한 출력 값까지 요구하거나 자동 전송하지 않는다.

이는 I5/I6/I7의 추적·검토 입력 진전이다. 원래 G4의 전체 지역 산출물·명칭 대응의 공식 근거 연결·
학교 기준일/지리 적용일 해석·population 승인 및 무힌트 자율 완주는 아직 검증되지 않았다.
실제 취득 회귀와 모델 fixture 승인 경로를 새 모델 정확도나 G4 완료로 집계하지 않는다.

`go mod tidy -diff`, 브랜드 동기화 검사, `go vet ./...`, `go test ./...`, `go build ./...`와
`internal/goalwork`, `internal/mcpserver`, `cmd/odeduck` race 검사를 통과했다. CLI 실제 명령과 MCP
JSON-RPC가 같은 제외 위치를 보존한다. 그래프 스킬은 파일 검사·일치 행 보관·모집단·결합 후 제외를
구분하도록 갱신했고 제공된 `quick_validate.py`를 통과했다. 특정 지역 대응이나 정답은 넣지 않았다.

## 같은 파일의 헤더 문맥 전달 후속 검증

2026-09-08 `50474b2` 이후, [같은 파일 문맥 계약](../specs/goal-source-report-review-v1.md#같은-파일의-별도-문맥-근거)을
구현했다. 계산에 참여하지 않은 별도 헤더 관측의 공개 근거가 검토 입력에서 빠지던 경로를 고쳤다.
새 엔진·도메인 예외·의존성을 추가하지 않았고, 중복된 검토 원천 metadata 투영 로직은 공통화했다.
기존 원천 보고 v1은 유지하며 분석 검토 권한 아래에서만 문맥을 전달한다. 파일 삭제는 없고 독립
reference, 정확한 이름 결합의 실패·공백 정규화 결과와 과거 모델 응답은 보존했다.

위 opt-in G4 실제 취득 명령은 12.54초에 통과했다. 인구·학교 파일 hash는 이전과 같으며,
162행→11그룹/304,280명과 학교 10쌍을 유지했다. 같은 학교 파일의 `구·군별!A22:AM26`을
별도로 취득해 선택한 다섯 열의 실제 값·원본 행 주소를 기존 독립 `headerEvidence`에 대조했다.
행정개편 적용 2026-07-01과 작성 기준 2026-04-01 문구, 합계·학교수·학생수·분교 헤더가
계산 원천과 분리된 `analysis.sourceContext`로 전달됐다. 헤더를 join하거나 날짜 열을 만들지 않았다.

원래 미대응 packet을 보존한 채 그룹·학교 행·원본 연령 필드 예시·헤더로 기존 8 packet 한도에
맞췄다. 원본 인구 162행은 로컬 재계산에 사용되며 검토자로 전체 전송되지 않는다. 검토 callback은
입력 경계를 검사하는 fixture로서 원래 목표를 `insufficient`로 남긴다. 실제 모델을 호출하지 않았으므로
헤더 적용·명칭 대응·모집단/전체 G4 의미 승인 또는 새로운 calibration 정확도를 주장하지 않는다.
PK·셀 범위 사전 지정도 유지했으므로 자율 탐색 평가가 아니다. 로그인·활용신청·외부 쓰기는 없었다.

회귀는 새 헤더 근거 뒤 추가 검토, 다른 PK/asset/ZIP member/내용/계약 revision·누락/잘못된 hash
차단, 미공개 값/metadata 격리, 원천 보고 권한만 있을 때의 제외, 검토 입력 변조 차단과 같은 문맥
셀을 새 packet으로 합쳐 재검토하는 우회 차단을 검사한다. 공개 `ReviewGoal` adapter는 별도 분석
guide·선택 문맥 전달·문맥 packet 인용을 검사한다. fixture 승인은 경로 검증으로만 센다.

`go mod tidy -diff`, 브랜드 동기화 검사, `go vet ./...`, `go test ./...`, `go build ./...`와
`internal/goalwork`, `internal/mcpserver`, `cmd/odeduck`의 race 검사를 통과했다.

다음은 같은 원래 G4의 전체 지역 산출물과 공식 명칭 대응·인구/학교 집계 범위를 근거에 연결하고
실제 의미 검토를 검증하는 것이다. 이번 문맥 경로는 별도 파일의 법령·대응표 연관이나 원천 기록의
시간 필드를 대신하지 않는다. 모든 I1–I10/G1–G5 및 M5 상태는 종전대로 미완료다.

## 미대응 원천 값의 결과 표 검증

2026-09-08 `991d143` 이후, [미대응 결과 계약](../specs/goal-result-execution-v1.md#미대응-원천-값의-결과-표)을
구현했다. 계산된 대응 행만 결과 표에 있던 경로에 명시적 `reportUnmatched` 투영을 추가했다.
별도 엔진·저장소·지역명 치환 없이 원래 실행 지표의 tuple과 선택한 원천 값을 결과에 보존한다.
미대응 값은 inner join·시간·산술의 입력으로 넣지 않는다. 기존 실패·oracle·원천 보고 경로와
기본 비공개 정책은 유지했으며 파일 삭제는 없다.

### 오데덕을 통한 대응 자료 확인

`catalog search --require-semantic --limit 6`로 다음 두 검색을 실행했고 모두
`semantic.status=used`(`embeddinggemma:300m-qat-q4_0`)였다. 사람이 정한 조사 검색어이므로
PK 없는 자율 목표 완주 평가나 의미 검색의 인과 효과로 세지 않는다.

- `인천 행정구역 개편 서해구 서구 명칭 변경`: [서구청 연혁 15105269](https://www.data.go.kr/data/15105269/fileData.do)를
  `inspect --delivery file --observe`로 검사했다. 현재 제목·기관은 서해구지만 자산은 2025-07-15
  연혁 CSV이며 관측 열은 연번/연대/연혁이다. 내용 SHA256은
  `14887a3636b629db89458fe74c76d72f118f81bc16a9432677e5515178cf8af7`이다.
  제목의 현재 이름만으로 2026년 학교 표의 동일 영역 대응을 확정하지 않았다.
- `행정구역 변경 이력 신구 대응 코드`: [등록번호용 지역코드 15063993](https://www.data.go.kr/data/15063993/fileData.do)를
  같은 명령으로 검사했다. 신·구 기관 코드/명칭과 변동일자 열이 있지만 공식 선언은 비법인 기관의
  등기용 네 자리 코드다. 내용 SHA256은
  `f3ef89af1936bac94f455e1bbe00867026c2d4ee89bd78b460d1965b1dda4f00`이다.
  주민등록·교육 행정구역의 namespace나 경계 대응 근거로 자동 전용하지 않았다.

이 두 후보의 schema 관찰은 개별 mapping 행 검증이 아니다. 공식 과거 대응이 없다는 결론도 아니다.

### 결과와 공개 경계

기존 G4 실취득 명령은 12.00초에 통과했다. 인구·학교 내용 hash와 162행→11그룹/304,280명의
독립 대조를 유지했다. 정확한 이름 5쌍, 명시적 trim 10쌍의 기존 실행에 이어 세 번째 조합은
10쌍과 미대응 인구 그룹/학교 행 두 tuple을 별도 표로 반환했다. 원래 `서해구`/` 서구 `와
학교·학생·인구 값이 독립 reference와 일치했으며 강제 이름 대응·0 대입은 하지 않았다.
헤더 문맥과 실제 보고 필드 모두 기존 8 evidence packet 안에서 scripted reviewer에 전달됐다.

첫 공개 Engine 테스트는 결과/recipe 계약 부재로 실패했고 구현 뒤 통과했다. 후속 테스트에서
미대응 결과 값이 공개 근거 없이 reviewer로 전달되는 결함을 재현했다. 모든 보고 필드를 선택
공개하고 로컬 원본·공개 값으로 각각 재현해야 검토할 수 있도록 수정했다. 새로운 원문 자동 공개
경로를 남기지 않는다. 16개 필드 한도도 compose 저장 전에 실패시키고 회귀로 고정했다.

추가 검증은 null/빈 상대/0/missing의 구분, 다단계 1:N tuple 보존, 없는 필드·계산 alias·중복 필드,
계산 표와 미대응 표의 합산 행/byte 초과, 반환 사본 격리와 CLI 실제 명령/MCP JSON-RPC를 포함한다.
`go mod tidy -diff`, 브랜드 동기화 검사, `go vet ./...`, `go test ./...`, `go build ./...`와 핵심
세 패키지의 race 검사를 통과했다. 원래 G4 GoalContract와 독립 정답은 변경하지 않았다.

검토 callback은 입력·상태 전이 fixture이며 원래 G4를 계속 insufficient로 남긴다. 실제 모델
해석, 근거 있는 행정구역 대응·학교/인구 집계 범위와 전체 질문 충족은 다음 검증이다. 값 두 개를
결과에 추가한 것을 G4·I1–I10·M5 완료로 세지 않는다. SSO·활용신청·외부 쓰기는 하지 않았다.

## 원래 G4의 실제 모델 진단

2026-09-08 후속으로 실제 `ReviewGoal`을 실행했다. 원래 G4 질문과 July citywide reference는
그대로이며 PK·recipe를 지정한 개발 진단이다. 새 자율 검색이나 held-out 평가가 아니다.
공개 집계·선택 헤더만 명시한 Codex로 전송했다. 개인별 행, 인증키, 기대 판정과 oracle 해석은
보내지 않았다. 기존 scripted recipe의 완료 여부 문구도 실제 모델 입력에서 제거하고 원천별
revision·정규화·측정 가정만 명시했다. 실제 제품의 기본 off·수신자·예산 계약은 바꾸지 않았다.

### 취득 revision과 실패 분모

첫 실제 취득은 인구 내용 hash가 달라 모델 호출 전 멈췄다. 이어 오데덕
`inspect 15097972 --delivery file --observe --format json`으로 현재 계약을 확인했다.
[공식 원천](https://www.data.go.kr/data/15097972/fileData.do)은 `20260831.csv`, 수정일 선언
2026-09-02, 2,568,139 bytes이며 SHA256은
`0cbb5983a9efad7951548a89ae11c295ed08e95f8d222d15bc5e5e46d5701e24`이다.
이를 7월의 독립 합계 304,280명으로 채점하거나 기존 oracle을 덮어쓰지 않았다.

원천을 고정한 의미 진단은 이후 `reference` 모드를 명시해 수행했다. 이 모드는 보존된 162개
인구 record·11개 학교 record·헤더를 재생하고 synthetic contract hash와 재생 경고를 사용한다.
현재 metadata, 실제 scanner의 전체 검사 영수증이나 모집단 근거를 생성한 것처럼 꾸미지 않는다.
그래서 아래 coverage 보류에는 **진단 입력이 취득·metadata 근거를 생략한 한계**도 포함된다.

| 시도 | 실제 모델 호출 | 결과 |
| --- | --- | --- |
| live revision | 0 | 현재 파일이 8월로 바뀌어 oracle hash 불일치; 첫 기록은 최종 View 없이 준비 실패만 보존 |
| reference 준비 | 0 | 필수 assumptions까지 제거한 진단 구성 오류; compose 거부, 실패 View 보존 |
| reference baseline | 1 | 기간 supported; 세 출력·관계·측정·coverage 및 GoalFit insufficient |
| reference with-branches | 1 | 기간·학교 관련 네 출력 supported; 인구·관계·측정·coverage와 GoalFit insufficient |

예정은 네 시도, 실제 모델 호출은 두 번, 원래 목표 완료는 0회다. 두 번의 예상 보류를 목표 성공률이나
모델 정확도 100%로 보고하지 않는다. provider는 Codex CLI 0.153.4의 기본 모델이며 응답에서 실제
모델명은 확인되지 않았다. 각각 76.88초와 87.44초였다. 단일 전후 실행이므로 변동성을 분리한 효과
평가가 아니다. 원문에 skill 목록 축약 경고가 있어 전역 CLI 문맥까지 비어 있었다고 주장하지 않는다.

### 실제 지적을 반영한 결과 수정

첫 모델은 같은 학교 시트의 A22/A23을 데이터 표 바로 위 문맥으로 해석해 4월 조사 기준일과
7월 행정구역 적용일을 구분했다. 반면 AH/AL 분교 수치가 출력되지 않은 점을 지적했다.
기존 Engine 연산만으로 진단 recipe의 Select·ReportUnmatched·선택 근거에 AH/AL을 추가하고,
기존 출력 세 개를 유지한 채 분교 학교 수·학생 수 출력을 각각 추가했다. 새 연산이나 지역명
치환은 만들지 않았다. 같은 8 packet 안에서 10개 비교 행과 미대응 학교 행의 분교 값까지 전달된다.
공개 Engine 회귀는 먼저 분교 출력 부재로 실패했고, 수정 후 구별 독립 기대값 및 전체 **7교/63명**과
일치했다. 이 fixture의 승인 응답은 상태 전이 검사용이지 실제 의미 검증으로 세지 않는다.

두 번째 실제 모델은 학교 수·학생 수·분교 수·분교 학생 수 각각의 원천 지지를 인정했지만,
만 나이 정의·전체 지역 coverage·서해구/서구 대응 근거 부족으로 원래 질문은 계속 보류했다.
주민 인구와 학교 학생수를 같은 연령·거주 모집단으로 만들거나 미대응을 0으로 채우지 않았다.

다음은 추가 산술 기능이 아니라 원천 의미와 적용 범위를 검토 입력에 연결하는 일이다.
예를 들어 [행안부 공식 설명](https://jumin.mois.go.kr/ageStatMonth.do)은 통계의 전체 등록구분에
거주자·거주불명자·재외국민을 포함하고 외국인은 제외한다고 설명한다. 이 정의와 만 나이 정의는
별개이며, 이번 웹 확인을 모델에 제공된 근거로 세지 않는다. 같은 파일 헤더만 연결하는 현재 경로는
별도 공식 대응표·방법론의 적용 근거까지 자동으로 제공하지 않는다. 근거 없는 가정을 추가하거나
승인 기준을 낮추어 해결하지 않는다. G1–G5·I1–I10·M5는 여전히 미완료다.

### 보존과 재실행

[진단 archive](../../internal/goalwork/testdata/goalbench-v1/citywide-review-20260908/)는 네 JSON을
내용 변경 없이 gzip `-n`으로 압축했다. 원본 임시 파일과 압축 해제 bytes를 대조했다.

| 파일 | 압축 해제 SHA256 |
| --- | --- |
| `live-revision-failure.json.gz` | `7a8229cc08d050c045e04c3102deda26a78766844bc63ad4a27bd13c1bec443d` |
| `replay-assumption-failure.json.gz` | `5fc19e674d8d8d8b5f8e49ab7cb893367c3ab46d317467180ff1ddfb61ca78d0` |
| `baseline-codex.json.gz` | `62becb0057de0078bbdb68c323937fe4a00e26f7cf7d592ccb11067957a6868b` |
| `with-branches-codex.json.gz` | `8471d3ba30011fd271026cf4b142c5884e771edab9fea7d4887ef2da188767ae` |

두 실제 호출의 `analysis-review-guide.md` SHA256은
`e6bcee382b9b1fe7a048e2ff8ac7cc8453266be429f68a0aa4287b8ed8b6ffb9`다.
공개 adapter의 archive 재생 회귀는 실제 두 원 응답이 기록된 판정으로 다시 해석되는지 검사하며,
과거 세 calibration archive의 각 12회 분모도 그대로 검사한다.

```sh
# 외부 모델 호출 없이 기존/신규 응답 재생
go test ./internal/agentplan -run '^TestReviewGoalReplaysArchivedCodexCalibration$' -count=1

# 선택 공개 집계·헤더를 Codex로 전송하는 단일 reference 진단. 기존 출력 경로는 거부한다.
ODEDUCK_CITYWIDE_REVIEW=codex ODEDUCK_CITYWIDE_ACQUISITION=reference \
ODEDUCK_CITYWIDE_RESULT=with-branches ODEDUCK_CITYWIDE_REVIEW_OUTPUT=/tmp/odeduck-g4-new.json \
go test ./internal/goalwork -run '^TestLiveCitywideGoalAnalysisReview$' -count=1 -v
```

기존 baseline은 `ODEDUCK_CITYWIDE_RESULT=baseline`으로 남는다. `live` 취득 모드에서는 현재
원천 revision이 고정 reference와 다르면 계속 실패하는 것이 맞다. 새 월 자료의 독립 검증은 별도다.
`go mod tidy -diff`, `go vet ./...`, `go test ./...`, `go build ./...`, 브랜드 동기화 및 핵심 세
패키지 race 검사를 수행했다. API 신청·SSO·배포·외부 tracker 변경은 하지 않았다.

독립 코드 검토에서 새 진단 진입점의 결정론적 회귀가 부족한 점을 지적받아 보강했다. 실제 테스트
명령 진입점과 cleanup을 자식 프로세스로 실행하고 외부 CLI만 fixture로 대체한다. 기존 파일
비덮어쓰기, provider 오류 원문, 잘못된 승인 상태·실패 표시, 원천 reference 부재, 잘못된 provider와
취득 모드 거부를 검사한다. 자식 실행은 30초로 제한하며 실제 모델 호출 없이 일반/race 회귀를 수행한다.

## 과거 버전의 실취득 복구

2026-09-08 10:00:12 UTC에 원래 G4 질문·7월 oracle·분교 포함 recipe로 `historical` 진단을
실행했다. 포털의 최신 항목 교체와 원본 삭제를 구분한 후속이다. 실제 포털 목록에서 July publication을
선택하고 같은 Unified Inspector/LiveDependencies/Engine으로 읽었다. PK·publication 이름·recipe를
사람이 고정한 개발 진단이며 자율 발견이 아니다. 기본 `live`와 명시 `reference` 모드 및 이전 실패는 유지한다.

현재 포털의 과거 목록은 54개, 노출은 첫 32개다. July ID는
`uddi:95f114ef-87c9-4669-971d-aab9aff9d7d4/2`이며 2,568,212-byte CSV의 hash는 기존
`6920fdafd269554d259e8498301f004c9799f499c38832de9352a0e7def916c5`와 일치했다.
전체 3,619개 데이터 행을 검사하고 인천 조건의 162행을 모두 보관했다. 원본 행 위치와
11개 지역의 6–17세 남녀 합계 **304,280명**을 기존 독립 reference에 대조했다.
학교 11행과 헤더도 원래 hash `c8f57fc8e3bd7e70175ff0debd1529a17539e526e436dc244fcea39ed9a0f695`로
실취득했다. 최신 metadata나 합성 scanner 영수증을 과거 원본에 붙이지 않았다.

선택 집계·헤더와 실제 취득 범위/metadata를 같은 Codex CLI 0.153.4의 별도 tool-free 요청으로
전송했다. 전체 소요 102.64초, 예정 1회/실제 모델 1회, 결과 `review_required`다. 다섯 출력 및
periods/measurements는 supported, relations/coverage/GoalFit은 insufficient였다. 서해구 인구와
서구 학교 행은 미대응으로 유지하며 명칭 대응의 근거를 요구했다. 모델이 인정한 6–17세 필드의
의미는 독립적인 만 나이 정의 검증이 아니며, 전체 스캔도 모집단 포괄성 승인으로 세지 않는다.

이 한 번의 판정 변화는 취득 입력 차이를 동반한 관측이며 모델 변동성과 효과를 분리한 대조 실험이
아니다. 앞선 네 시도를 합치면 G4 진단은 **5시도/3모델 호출/목표 완료 0회**다. 보류를 정확도나
양성 완주 성공으로 세지 않는다. 다음은 원래 요청의 기준일·집계 대상·행정개편 대응 근거와 전체
설명을 연결하는 작업이다. 새 graph DB나 계산 연산을 추가할 이유가 확인된 것은 아니다.

원 입력·원 응답·Engine View는 기존 archive의 `historical-codex.json.gz`로 보존했다.
압축 해제 SHA256은 `3840b8cc0d4d59198f8036374631fa5526bbe722fa5651e51e3906b3f6665224`다.
검토 지침 hash는 앞선 두 실행과 동일하다. 이 원 응답도 공개 ReviewGoal adapter의 오프라인 재생
회귀에 추가했다. 제품 기본 전송 권한이나 검토 정책은 변경하지 않았다.

```sh
ODEDUCK_CITYWIDE_REVIEW=codex ODEDUCK_CITYWIDE_ACQUISITION=historical \
ODEDUCK_CITYWIDE_RESULT=with-branches ODEDUCK_CITYWIDE_REVIEW_OUTPUT=/tmp/odeduck-g4-historical-new.json \
go test ./internal/goalwork -run '^TestLiveCitywideGoalAnalysisReview$' -count=1 -v
```

공개 seam 회귀에서 JSON 변환 파일의 잘못된 CSV 표시, 원 요청과 다른 버전 응답, 파서 drift의 빈 목록
오인, popup 밖 metadata 혼입, 중복 asset 이름, 잘못된 attachment ID, 목록만 본 뒤 structural 장부
검증을 허용하던 실패를 먼저 재현했다. 공통 검사와 실제 CLI/MCP·Engine 경계에서 이를 차단했다.
한도/사라진 membership, 현재 재검사 후 과거 관측의 로컬 합계 보존도 fixture로 검사한다.
최신 파일 경로는 기본 동작으로 유지하되 과거 취득을 위해 최신 metadata를 재사용하는 경로는 두지 않는다.
SSO·신청·배포·외부 tracker 쓰기는 하지 않았다.

과거 FILE 구현의 독립 Spec 검토는 첫 candidate `036ea438`에서 같은 attachment/serial에 CSV와
JSON을 중복 선언하면 뒤의 형식이 무시되는 P2 한 건을 찾았다. 공개 Inspector seam에서 실패를
재현한 후 충돌 선언을 거부하도록 수정했다. 대소문자만 다른 동일 형식의 반복 버튼은 계속 허용한다.
기준점 `8e19bff` 대비 수정 candidate `01152035`(tree
`1d087abad717932fa9a37491aa9329b73d27fbc4`)의 후속 검토는 Standards 0건, Spec 미해결 0건이다.
분리된 깨끗한 worktree에서 tidy/vet/전체 test/build/브랜드 일치를 통과했고, 원래 worktree에서
dataset·goalwork·agentplan·MCP·CLI race 검사를 통과했다. 검토 이후 변경은 이 검증 기록뿐이다.
이는 구현 계약 검증이며 추가 모델 호출이나 G4 의미 승인 증거가 아니다.

### 공식 정의와 행정구역 적용 범위 재확인 — 2026-09-08

이번 후속은 공식 웹의 방법론·법령 조사다. 오데덕의 새 자율 발견이나 실취득·모델 검토로 세지
않으며, 기존 **5시도/3모델 호출/목표 완료 0회**와 원래 G4 질문·oracle은 그대로다.

- **연령**: KOSIS의 2024-06-28 답변은 주민등록인구현황 작성기관에 확인한 내용으로, 주민등록상
  출생월일과 통계 기준 월의 말일에 따라 만 나이를 산출한다고 설명한다. 이는 해당 통계계열의
  공식 방법론 근거지만 PK `15097972`의 July CSV revision을 직접 명시한 답변은 아니다.
  [KOSIS 작성기관 확인 답변](https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124)
- **모집단**: 행안부 화면은 등록구분 전체에 거주자·거주불명자·재외국민을 포함하고 외국인은
  제외한다고 명시한다. 별도 거주자 구분은 재외국민을 제외하므로 일상어의 거주 인구와 등록구분
  전체를 섞을 수 없다. 월별 통계의 기준일은 매월 말일이다. 이 화면의 선택 가능한 등록구분 중
  어느 것이 July CSV에 적용됐는지는 파일에 연결된 선언으로 추가 확인해야 한다.
  [행안부 연령별 주민등록인구 화면](https://jumin.mois.go.kr/ageStatMonth.do)
- **명칭과 영역은 다른 변경**: 명칭 변경 법률 제21734호(2026-06-02 공포)는 서구를 서해구로
  바꾸며 2026-07-01 시행한다. 부칙 제3조는 다른 법령의 인용 관계를 정할 뿐 학교 통계 행의
  대응표가 아니다. 부칙 제2조의 법원 관할구역 표 개정만 2028-03-01 시행인 예외와 구분한다.
  같은 2026-07-01 시행되는 설치법 제21247호의 제2조제3항은 검단구에 속하게 된 영역을 서구에서
  제외한다. 따라서 개편 전 4월 서구 전체와 개편 후 7월 서해구의 영토적 동일성을 명칭 변경만으로
  승인할 수 없다. 이 결론은 두 법률을 함께 읽은 적용 판단이며 학교 원자료의 집계 검증은 아니다.
  [명칭 변경 법률 본문](https://www.law.go.kr/LSW/lsInfoP.do?lsiSeq=286453&efYd=20260701&ancYnChk=0),
  [명칭 변경 법률 부칙](https://law.go.kr/LSW/lsRvsDocListP.do?chrClsCd=010202&lsId=015137&lsRvsGubun=all),
  [신설구 설치법 제2조](https://www.law.go.kr/LSW/lsInfoP.do?lsiSeq=281877&efYd=20260701&ancYnChk=0)

포털의 현재 August [인구 설명·컬럼 표](https://www.data.go.kr/data/15097972/fileData.do)는
행안부·KOSIS 통계 홈페이지를 안내하지만 만 나이나 등록구분의 상세 정의를 싣지 않는다.
`계`의 설명은 전체 주민등록인구, 개별 연령 열 설명은 해당 연령·성별 표기에 그친다.
July ID `uddi:95f114ef-87c9-4669-971d-aab9aff9d7d4/2`의 과거 popup은 공식
`/tcs/dss/selectDpkDetailInfo.do`에 해당 detail PK와 `publicDataHistSn=2`를 보내 재확인했다.
제공기관·등록일·수정일·3,619행·CSV/JSON 자산은 있지만 설명·연령 정의·등록구분·행정구역 대응은
없다. 메인 작업의 동일 ID에 대한 오데덕 CLI 검사도 이 경계를 확인했다. 이 HTTP 재확인은
파서 누락과 원천 부재를 구분하기 위한 별도 조사이며, 현재 설명을 과거 metadata로 승격하지 않는다.

다음 최소 작업은 새 저장 계층이 아니라 **정의의 원문 위치·revision·적용 필드/기간을 기존 검토
입력까지 연결하는 것**이다. 먼저 July 파일의 등록구분을 확정할 원천 선언과, 학교 시트의
7월 개편 표에 남은 `서구` 행이 검단구를 제외해 재집계됐다는 원천 근거를 확인해야 한다.
법령은 행정개편의 의미를 설명하지만 특정 학교 행의 실제 소속·재집계를 보증하지 않는다.
현재 컬럼 정의서만 추가로 읽거나 외부 설명을 assumptions에 넣는 것으로 이 두 적용 공백이
해결되지는 않는다. 근거를 확보하기 전에는 미대응 값을 보존하고 원래 질문의 비교 한계를 설명한다.
