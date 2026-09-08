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
