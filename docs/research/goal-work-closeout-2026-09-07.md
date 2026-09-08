# 목표 기반 탐색·연결 작업 마무리 — 2026-09-07

상태: 안전 마무리 완료, 후속 구현 중단. **제품 목표는 미완료다.**
새 사업 사례·연산·그래프 저장소·의미 승인 기능을 추가하지 않았다. 다음 구현은 별도 요청 후 시작한다.
의도는 [INTENT.md](../../INTENT.md), 통과선은 [완료 계획](../specs/goal-driven-completion-plan.md),
과거 실측의 단일 소스는 [실행 평가](goal-composition-evaluation-2026-09-07.md)다.

## 1. 변경 보존 경계

- 시작 HEAD: `450d17b4a8edb0927d7fccd30592b7211013e4fe`.
- 시작 시 이미 변경된 파일 122개를 2026-09-07T07:56:34.047Z에
  [경로·SHA-256 목록](goal-work-closeout-baseline-2026-09-07.json)으로 고정했다.
  이는 **이번 마무리 전 상태**이지 과거 작성자 증명이나 원본 백업이 아니다.
- 122개 중 111개는 시작 해시와 동일하게 보존했다. 나머지 11개에만 아래 수정·문서 정리를 했다.
  파일 삭제, reset/checkout, 기존 수정의 일괄 재작성은 하지 않았다. 출처가 불확실한 변경도 보존했다.
- 기존 tracked/untracked 여부는 작성자를 증명하지 않는다. 이전 개발의 대규모 dirty diff를 이번
  마무리가 전부 새로 작성한 것으로 보고하지 않는다. Git index·HEAD는 변경하지 않았다.
- 시작/종료 프로세스 확인에서 작업용 `odeduck`, `go test`, `go build`, `codex exec`는 남지 않았다.
  이번에 실행한 테스트와 읽기 전용 검토 작업도 종료했다. 로그인·신청·키 변경·canary·새 live solve는
  실행하지 않았으며, 커밋·푸시·릴리스·이슈 변경도 하지 않았다.

이번 마무리에서 바꾼 기존 11개:

| 파일 | 변경 목적 |
| --- | --- |
| `internal/dataset/standard.go`, `standard_test.go` | STD JSON 문자 손실 차단과 public-seam 회귀 테스트 |
| `internal/goalwork/explanation.go`, `explanation_test.go` | 실행하지 않은 키 결합 설명 제거 |
| `internal/goalwork/lineage_test.go` | scope 분모에서 lineage 거부 쌍이 제외됨을 고정 |
| 두 `mobility-unseeded-*-20260907.json` 진단 | 기존 원본 로그와 대조해 누락 메타데이터 보강; 당시 결과 보존 |
| `INTENT.md` | 제품 미완료와 이번 개발 중단 상태 구분; 마무리 기록 연결 |
| `goal-driven-completion-plan.md`, `goal-driven-composition-v1.md`, `goal-completion-benchmark-v1.md` | 현재 근거·지원 범위·metric 의미 정합성 |

별도로 `internal/goalwork/diagnostic_record_test.go`와 이 문서·시작 해시 목록을 추가했다.
시작에 clean이던 `CHANGELOG.md`에는 **Unreleased** 실험 기능/수정만 기록했다. 버전을 올리지 않았다.

## 2. 두 축 검토와 해결한 문제

### Standards

범위: 변경된 acquisition·catalog·API 경계와 관련 미추적 파일, 저장소의 자격증명·fixture·원문 보존
규칙. 발견 1건: STD의 `encoding/json`이 잘못된 UTF-8와 단독 UTF-16 surrogate를 U+FFFD로 바꿔,
서로 다른 잘못된 식별자를 같은 문자열로 보존할 수 있었다.

`TestStandardJSONStringsAreLossless`로 header/row 양쪽의 실패를 먼저 확인했다. 디코딩 전에 유효
UTF-8 JSON 및 surrogate 쌍을 검사하도록 수정했다. 서로 다른 invalid bytes, high/low 단독 escape,
중간에 문자가 있는 잘못된 쌍은 거부한다. 실제 U+FFFD, 정상 supplementary 문자, 문자 그대로의
`\ud800` 표기는 보존한다. 숫자 정밀도·요청 scope·다운로드 제한은 바꾸지 않았다.

### Spec

범위: `INTENT.md`, composition/lineage 계약과 현재 실행·설명·평가. 발견 3건:

1. 키 결합 없는 공간 projection에도 exact/trim 결합을 계산했다고 설명했다. public engine 테스트를
   먼저 실패시킨 뒤 실제 `joins` 유무로 설명을 분리했다. 실세계 동일성 미검증 상태는 유지한다.
2. scope 문서가 모든 key-equal 쌍을 분모로 표현했지만 실제로는 lineage 충돌이 먼저 제외된다.
   안전한 실행 순서는 유지하고 문서·테스트를 고쳤다. 같은 키 8쌍 중 lineage 거부 4쌍,
   scope 후보/일치 각 4쌍을 public engine으로 확인한다.
3. G5의 28.9 MB를 무조건 용량 초과로, G4 학교 셀 production 읽기를 미검증으로 설명하던 문구를
   수정했다. 명시적 CSV scan은 64 MiB까지이나 G5 실재생을 증명하지 않는다. G4 11개 학교 요약 행
   읽기 재생은 기록되어 있으나 도시 전체의 인구·학교 조합/의미 승인/자율 완주는 여전히 미검증이다.

검토 합계: Standards 1건, Spec 3건을 해결했다. 발견된 최상위 위험은 각각 식별자 손실과 잘못된
실행 설명이었다. 검사한 범위에서 추가 확정 회귀는 발견하지 못했으며, 완전한 보안 증명은 아니다.

## 3. 실제 검증 기록과 재현 경계

| 기록 | 실제로 확인한 것 | 확인하지 않은 것 |
| --- | --- | --- |
| G2 무힌트 07:06–07:12 UTC | 의미 검색 7회, 7관측, 주소 토큰 불일치 9쌍, `abstained`, 산출물 없음 | 목표 완주/자동 동일성 승인 |
| 같은 7검색어의 사후 검색 비교 | 무장애 원천이 hybrid 상위에 있고 lexical 상위 8개에는 없었음 | lexical-only 자율 완주와의 인과 대조 |
| 명칭 사전의 seeded 공개 원천 재생 | 같은 원천 해시에서 literal 거부 9쌍을 후보 9행으로 복구 | 무힌트 탐색, population, 현재 접근성 |
| G2 무힌트 07:37–07:43 UTC | 사전 자율 선택, 12행 후 교통 포함 6행; `abstained`/`partial` | 교통망 전체·일상적 근접성·휠체어 이동 가능성 |
| G2/G4 등 고정 reference와 reader 재생 | 원본 위치·독립 수치/행 대조와 선택한 reader의 동작 | 다섯 목표 전체의 독립 oracle·held-out 통과 |

최신 6행의 교통 후보는 인천2호선 27개 기록에 한정되었고 계산 거리는 약 7.2–8.95 km였다.
이는 실행기가 낸 값이며 새로운 독립 거리 oracle이 아니다. 구조상 `nearby_transit` 값이 존재한다는
검사 통과는 목적 적합성을 증명하지 않는다. 질문에 명시되지 않은 `population` 해석도 미확정이다.

두 [무힌트 기록](../../internal/goalwork/testdata/goalbench-v1/)은 원래 run/revision·PK·내용/요청 해시를
로컬 원본 `view.json`과 대조한 뒤 다음을 보강했다: `observedAt`, 계약/행 해시, CSV/ZIP/XLSX 원본
위치 메타데이터, layout 및 source revision pin. 원문 셀이나 새로운 원격 데이터를 추가하지 않았다.
당시 잘못된 설명·중단 사유도 역사적 결과로 보존했다. 현재 수정된 설명으로 소급 대체하지 않았다.

오프라인 기록 검사는 다음 명령으로 재현한다. 과거 상태·요청 해시·원천 binding·layout pin·사전
해시·partial 판정의 보존을 검사하며, 원천 재호출이나 독립적인 정답 계산을 실행하지 않는다.

```sh
go test ./internal/goalwork -run '^TestRetainedGoalDiagnosticsDoNotBecomeCompletionEvidence$' -count=1 -v
go test ./internal/goalwork -run 'TestIndependentReference|TestCitywideEducationReference|TestEducationReference|TestMobility|TestEnvironmentReference|TestProductReference' -count=1
```

기존 live 재생 진입점은 아래와 같다. **이번 마무리에서는 실행하지 않았다.** 공개 원천 재생은
fixture의 원천 해시/선택자와 대조한다. 원격 revision이 바뀌면 기존 기대값을 수정하지 말고 drift로
기록해야 한다. 마지막 `solve`는 설치된 agent 사용량이 발생하며 계정 상태에 따른 호출도 가능하므로
재개 요청과 환경 확인 후에만 실행한다.

```sh
ODEDUCK_LIVE_SCOPE=1 go test ./internal/goalwork -run '^TestLivePublishedScopeVocabularyRecoversRecordedMobilityPairs$' -count=1 -v
ODEDUCK_LIVE_ZIP=1 go test ./internal/goalwork -run '^TestLiveZIPCSVReaderMatchesIndependentMobilityRecords$' -count=1 -v
ODEDUCK_LIVE_CSV_SCAN=1 go test ./internal/goalwork -run '^TestLiveFullCSVScanMatchesIndependentTransitReference$' -count=1 -v
ODEDUCK_LIVE_NEAREST=1 go test ./internal/goalwork -run '^TestLiveNearestMatchesIndependentMobilityDistances$' -count=1 -v
ODEDUCK_LIVE_XLSX=1 go test ./internal/goalwork -run '^TestLiveXLSXSchoolReaderMatchesIndependentCitywideReference$' -count=1 -v
ODEDUCK_LIVE_SOLVE=1 go test ./cmd/odeduck -run '^TestLiveSolveUnseededMobilityDiagnostic$' -count=1 -v -timeout=65m
```

재현 한계: 전체 원천 bytes와 과거 dirty 코드/모델 revision을 영속 보관한 실행 bundle은 없다.
해시는 원천을 식별할 뿐 복원하지 못한다. 로컬 임시 원본 로그는 사라질 수 있으며, 기록 보강은
이를 완전한 실행 아카이브로 만들지 않는다. 새 모델·카탈로그·원천 실행이 동일 행동/행을 낸다는
보장은 없다. 과거 결과의 감사, 고정 reader 재생, 새 무힌트 실행을 서로 구분해야 한다.

## 4. INTENT 요구사항별 감사

아래 상태는 **요구사항 전체**의 상태다. 테스트된 부분 기능이 있다는 이유로 전체 완료로 바꾸지 않는다.
경로는 저장소 루트 기준이며, 디렉터리 없는 테스트명은 `internal/goalwork/` 안에 있다.
상세 계약과 마일스톤의 단일 소스는 완료 계획에 둔다.

| ID / 상태 | 확인한 구현·검증 근거 | 남은 요구 / 다음 선행 조건 |
| --- | --- | --- |
| I1 부분 | `requirements_test.go`, `temporal_test.go`, CLI/MCP의 불변 목표·역할·출력 전달 | 실제 범위/기간 해석 검증. G2 population 추정과 '주변' 해석을 먼저 확정 |
| I2 부분 | `engine_test.go`의 중간 crosswalk/실패 이력, 두 무힌트 G2 기록 | 다른 역할/대안 발견이 유용한 최종 결과로 이어지는 독립 판정 필요 |
| I3 부분 | `internal/catalog/search_test.go`, 실제 semantic.used, G2 사후 검색 대비 | 동일 snapshot/질문의 lexical-only 대조와 완주 영향은 미검증 |
| I4 부분 | `internal/dataset/*test.go`, `goalwork/live_test.go`, 기존 공개 CSV/ZIP/XLSX 재생 | 형식별 계약과 제한은 구현. 실제 로그인/승인 후 API·접근 복구는 이번에 검증하지 않음 |
| I5 부분 | `scope*_test.go`, `temporal_test.go`, `lineage*_test.go`, 사전 고정 원천·오탐 | 명칭 대응은 identity 아님. 지리/기간/단위/coverage 의미 승인과 정정 증거 필요 |
| I6 부분 | `compose_test.go`, `measure_test.go`, `row_sum_test.go`, `nearest*_test.go` | bounded 후보 실행은 구현. 목적 적합성, 일반 공간 변환·원천 선집계는 미완료 |
| I7 부분 | `requirements_test.go`, `cmd/odeduck/solve_test.go`의 성공 코드 차단 | sample_joined/partial/review_required 구분 구현. 양성 자동 승인 경로 없음 |
| I8 부분 | 세션 관측·실패·hash/lineage와 기존 `connectionledger`는 각각 존재 | 목표에서 장부 retrieval·freshness·supersession·영속 재사용 통합은 미구현 |
| I9 부분 | `internal/mcpserver/goal*_test.go`, `retry_test.go`, `attempt_test.go`, CLI tests | 공통 엔진·세션 격리·예산·원문/키 차단 fixture 통과. 프로세스 재개·실계정 복구는 미검증 |
| I10 미완료 | `oracle_reference_test.go`, `oracle_mobility_test.go`, historical 진단 | G1–G5 완전한 양성/음성/정정 oracle·held-out·자율 완주 gate 미통과 |

M1 reference slice, M2 상태/재시도, M3 reader/연산, M4 표기 대응은 부분 구현이다.
M5 통과 평가에는 착수하지 않았다. all-review/all-abstain은 안전한 실패 처리이지만 제품 완성은 아니다.

## 5. 이번 검증 결과

환경: Go 1.26.6, darwin/arm64. `ODEDUCK_LIVE_*=1`인 환경 변수는 없었다.
실제 로그인·provider canary·과금 agent를 호출하지 않고 fixture/로컬 테스트로 확인했다.

| 검증 | 결과 / 범위 |
| --- | --- |
| `go mod tidy -diff` | 통과, module 변경 없음 |
| `go run ./scripts/sync-brand.go --check` | 통과, 공개 브랜드 블록 일치 |
| `go vet ./...` | 통과 |
| `go test ./... -count=1` | 전체 패키지 통과, opt-in live는 skip |
| `go test -race ./... -count=1` | 전체 패키지 통과 |
| `go build ./...` | 통과 |
| 새 진단 보존 검사·문서 링크/해시/format 검사 | 통과; 과거 실행의 성공 재판정 아님 |

보안·무결성 검증은 원문/credential 격리, JSON 수치 정밀도, stale revision/세션 소유권, 다운로드
계약·상한, scope/시간/lineage·중복 집계 차단을 포함한다. 이 결과는 실서비스 전체 보안 감사나
현재 포털 계약 drift·인증 수명·공개 데이터의 사실 정확성을 증명하지 않는다.

## 6. 남은 위험과 다음 작업의 정확한 시작점

1. **재개 승인 전에는 구현/실조회를 시작하지 않는다.** 이 마무리 완료는 제품 완료와 별개다.
2. 재개 시 먼저 이 문서 → 완료 계획 I1/I5/I7 → benchmark G2 질문 및 최신 무힌트 진단을 읽는다.
   `internal/goalwork/requirements.go`의 구조 평가와 `internal/agentplan/goal.go`의 계약 제안이 코드 진입점이다.
3. 사용자 범위(전체/일부), ‘주변 교통’의 후보망/거리 의미, 자료별 날짜의 역할을 먼저 명시한다.
   임의의 거리 임계값이나 population→sample 축소로 성공을 만들지 않는다. 기존 6행은 승인 정답이 아니다.
4. 그 해석에 대한 독립 양성 출력·오탐 기준을 고정한 뒤에만 부족한 기존 원천/대응·승인 규칙의 작업
   범위를 정한다. 현재 해시와 다르면 새 revision으로 기록한다. 새 사업 사례나 대규모 설계는 별도 계획이다.
5. 영속 장부/재개·정정, 전체 oracle/held-out, 자율 검색 기여 평가, 추가 reader/연산은 이번 범위 밖의
   미완료 backlog다. 실계정 API 복구가 필요하면 사용자에게 로그인 필요를 알리고 권한을 확인한다.
   인증키를 채팅이나 fixture로 받지 않는다.

중단 시 미해결 확정 회귀는 없지만, 목적 적합성·실세계 동일성·현재 유효성의 검증 공백은 남아 있다.
마무리 이후 테스트 PASS나 부분 행 수를 근거로 이 공백을 자동 완료로 표시하지 않는다.
