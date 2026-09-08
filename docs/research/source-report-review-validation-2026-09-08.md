# 원천 보고 검토: 개발 검증 결과

2026-09-08. [원천 보고의 분리 검토](../specs/goal-source-report-review-v1.md)를 실제 Engine과
별도 모델 요청으로 실행했다. 응답 원문을 보존한 마지막 12회에서 원천 보고 6회는 `output_ready`,
자료보다 강한 결론을 요구한 6회는 `review_required`였다. 모두 사전에 정한 기대 판정과 일치했다.
**두 개발 사례의 반복 검증이지, 범용 정확도·자율 탐색·G1–G5 완료의 증거는 아니다.**

## 무엇을 검증했나

한 원천의 제한된 기록을 그대로 보고하는 경로다. 검토자는 계획 이력 없이 원래 Goal, 불변 Contract,
실제 실행 결과와 공개를 허용한 Evidence Packet을 받는다. 출력별 원천 지지와 원래 목표 적합성을
따로 판단한다. 모델 판단을 제공기관 승인이나 현장 검증으로 승격하지 않는다.

| 개발 사례 | 승인 기대: 원천에 적힌 보고 | 보류 기대: 같은 자료로 답할 수 없는 요청 |
| --- | --- | --- |
| 환경: 2024년 12월 | 서울·인천 도시별 PM25 통계 두 행의 원문 필드 | 각 개별 측정소의 PM25 월평균 |
| 인구: 2026년 5월 31일 | 부평1동 만 6세 남녀 주민 인구 두 행 | 그중 초등학교에 재학 중인 남녀 학생 수 |

음성 사례는 원래 Goal만 더 강하게 바꾸고 Contract와 결과는 약한 보고로 유지했다. 마지막 6회 모두
필드 자체는 `supported`였지만 Goal은 `unsupported`였다. 도시별 값을 개별 측정소 값으로,
연령별 주민 인구를 재학생 수로 해석하지 않았다. 항상 보류하는 구현도 양성 6회를 통과할 수 없다.

### 자료와 실행의 경계

- 환경: [공식 월별 통계](https://www.data.go.kr/data/15150551/fileData.do)의 기존 독립
  [환경 원천 기록](../../internal/goalwork/testdata/goalbench-v1/environment-reference.json)을 재생했다.
  reference의 `csvRecord` 12460·12463, 원 관측 시각은 `2026-09-07T04:06:30.759848+00:00`이다.
  선택한 8개 필드에는 도시별 구분, 측정소수 25·25와 `1시간 월평균` 19·19가 포함된다.
- 인구: [공식 동별 연령·성별 자료](https://www.data.go.kr/data/15085585/fileData.do)의 기존 독립
  [교육·인구 원천 기록](../../internal/goalwork/testdata/goalbench-v1/education-reference.json)을 재생했다.
  reference의 `csvRecord` 14·15, 원 관측 시각은 `2026-09-07T04:04:54.160580+00:00`이다.
  선택한 3개 필드는 `연령(세)` 6, `성별` 남·여, `부평1동인구수` 64·62다. 합계나 재학 여부를 만들지 않았다.
- 당시 파일의 SHA256과 record 위치는 기존 reference 및 새
  [사례 manifest](../../internal/goalwork/testdata/goalbench-v1/source-report-cases.json)에 고정했다.
  이번 실행에서 파일 전체를 다시 내려받았다고 주장하지 않는다.
- 현재 메타데이터는 오데덕 `LiveDependencies.Inspect`로 각 공식 페이지에서 읽었다.
  검색과 sample 취득은 위 기록을 재생했다. 계획·PK·선택 필드를 미리 정했으므로 키워드 없는
  자율 발견 실험이 아니다. 상태 전이와 실행·근거·검토 결합은 실제 Engine을 사용했다.
- 공개 집계 기록만 명시한 provider로 전송했다. 공공데이터 로그인이나 활용신청은 필요 없었다.
  모델에는 기대 판정, 음성 사례 설명, 이전 검토 답, 전체 reference 파일을 보내지 않았다.

후속 실제 취득 대조에서 번호 설명을 바로잡았다. 위 reference의 `csvRecord`는 헤더를 1로 센다.
Engine의 헤더 제외 data record는 환경 12459·12462, 인구 13·14다. 기존 원천 값·reference 번호·
과거 실행 기록은 바꾸지 않았으며, 물리 줄 번호와도 구분한다.

## 실패를 포함한 전체 분모

각 배치는 두 분야 × 양성/음성 × 세 번으로 12회다. 앞선 실패나 응답 보존의 결함을 없애고
마지막 결과만 전체 실험인 것처럼 기록하지 않는다. 아래 archive에 모든 예정 시도가 남아 있다.

| 배치 | 최초 시작 시각 UTC | 예정 / 기대 판정 일치 | 실제 결과와 보존 수준 |
| --- | --- | --- | --- |
| `preflight-failure` | 04:45:16 | 12 / 0 | 선택 필드 재실행의 lineage hash 검사 실패. 모델 호출 전 중단, 의미 판정 없음 |
| `claude-access-failure` | 04:47:24 | 12 / 0 | provider 실패. Engine 실패 이력·입력만 보존, 원 응답 없음 |
| `codex-typed-only` | 04:48:42 | 12 / 12 | 양성 6 승인·음성 6 보류. 구조화된 판정만 있고 원 응답은 없음 |
| `codex-raw` | 05:01:10 | 12 / 12 | 양성 6 승인·음성 6 보류. 입력·전체 View·provider 응답 원문 12개 보존 |

총 예정 48회 중 Codex 판정은 24회다. 서로 다른 개발 단계의 결과를 합쳐 정확도 비율로 제시하지 않는다.
Claude 실패 당시 별도 직접 호출은 조직의 구독 접근 제한 HTTP 403을 반환했다. 이것은 운영 진단이며
해당 배치 archive에는 원문 오류가 없어 재검증할 수 없다. 인증 설정을 바꾸거나 자동 fallback하지 않고
명시적으로 Codex를 선택해 새 배치를 실행했다. 과거의 누락된 원문은 나중 응답으로 메우지 않았다.

첫 실패는 선택한 열로 재생한 데이터에 전체 원천 행 hash를 그대로 적용한 문제였다. 실제 검토 입력의
원천 hash는 보존하고, 제한된 열로 재현하는 내부 실행의 hash를 별도로 계산하도록 고쳤다.
후속 수정은 원 응답을 별도 반환해 명시적인 개발 검증에서만 보존하게 했다. 제품 CLI/MCP의 목표 상태에는
구조화된 판정만 들어간다. 원천 누락·모델 실패도 예정 시도의 분모에서 빼지 않는 회귀 테스트를 추가했다.

### 모델·시간·비용 관측

실행 환경은 Go 1.26.6, darwin/arm64였다. Codex CLI는 `0.153.4`, Claude CLI는 `2.1.261`이었다.
Codex의 실제 모델명은 응답에 없어 확인하지 못했다. CLI 기본 선택을 사용했으며 모델명을 추정하지 않는다.
현재 기록으로 달러 비용도 계산할 수 없다.

마지막 배치의 `turn.completed` 사용량 합계는 input 262,196 tokens, cached input 61,440 tokens,
output 10,024 tokens다. 캐시 수치를 임의로 빼서 청구량으로 간주하지 않는다. 환경 사례는 회당
36.63–42.13초, 인구 사례는 23.06–25.43초였다. 단순 보고에 비해 추가 비용·지연이 크므로 기본 off를 유지한다.

각 요청은 새 임시 작업공간과 tool-free 설정을 사용했다. 원문 이벤트에는 도구 실행이 없고 최종
agent message와 사용량이 있다. 다만 Codex의 skill 목록 축약 경고도 있어, 전역 CLI 문맥까지 전혀 없었다고
주장하지 않는다. 같은 provider의 반복 요청은 통계적으로 독립된 검토자나 독립 정답 근거가 아니다.

## 보존·재실행

원문은 [검증 archive 디렉터리](../../internal/goalwork/testdata/goalbench-v1/source-report-review-20260908/)의
동명 `.jsonl.gz` 파일 네 개에 보존한다. gzip은 내용 변경 없이 `-n`으로 압축했다. 아래는 **압축 해제한
JSONL bytes**의 SHA256이다. 원본 임시 파일과 압축 해제 결과의 hash를 대조했다.

| Archive | 압축 해제 SHA256 |
| --- | --- |
| `preflight-failure.jsonl.gz` | `fb225c623f9653353d6d1dd3cb07de983976fa1d350194a59fcbc98419d3b0f8` |
| `claude-access-failure.jsonl.gz` | `77c7e17b9f9fafa358629090295d6e4c3d87243dc4ebdf697a292b4d4e906cd7` |
| `codex-typed-only.jsonl.gz` | `de5fd65f6c78af66250fe71e20a3a598c068064651b7df4b9d06c1eda51ce584` |
| `codex-raw.jsonl.gz` | `dfe7bfa65bca54f1c8cee8060ec1a1a0f86aebd3cc198cbabdaee894431e36ca` |

검토 prompt `internal/agentplan/source-review-guide.md`의 SHA256은
`910b18bd6a11b15ce5cf25a311c6cc7373b2722989386b80a77f890df8e9a107`,
사례 manifest의 SHA256은 `1fa470b7e8f1ef788b7a91b2d6a0e1141e0dfa06212ba982e63ebcf4d504339e`다.

```sh
# 외부 요청 없이 보존된 실제 응답을 현재 공개 adapter로 재생
go test ./internal/agentplan -run TestReviewGoalReplaysArchivedCodexCalibration -count=1

# 새 실제 모델 검증: 공개 필드가 지정 provider로 전송됨. 기존 출력 파일은 덮어쓰지 않음
go run ./scripts/goal-review-check --agent codex --output /tmp/odeduck-source-review-new.jsonl
```

실제 실행은 개발 중 바이너리로 진행했다. 마지막 모델 배치 시작 후 JSON 오류 처리와 검증 명령의
dependency 구성을 보완했다. prompt와 사례는 바꾸지 않았다. 최종 adapter가 보존된 12개 원문에서
같은 구조화 판정을 얻는지는 위 offline 회귀 테스트로 확인한다. 이를 최종 바이너리의 새 모델 실행이나
추가 12회 검증으로 세지 않는다. 시간·모델 비결정성 때문에 새 실행의 byte 단위 재현도 보장하지 않는다.

## 판정과 다음 범위

제한된 원천 보고에 대해 실제로 도달 가능한 완료 경로를 만들었다. 기본 off, 고정된 수신자,
선택한 근거의 범위, 실행 revision, 최대 세 번의 검토, 같은 근거 반복 차단을 유지한다.
전체 테스트·vet·build와 Spec/Standards 검토는 이 계약의 구현 검사이며 모델 정확도 근거와 구분한다.

[INTENT.md](../../INTENT.md)는 여전히 미완료다. 기존 G1–G5 원문과 oracle을 수정하지 않았고,
두 도시의 한 달을 전국 연간 분석으로, 한 동의 두 행을 전체 교육 수요 분석으로 계산하지 않는다.
계산·가설·entity identity의 의미 승인, 실제 교차 자료 연결, 근거 재사용, 사전에 자료를 고르지 않은
목표의 완주와 사업적 유용성은 다음 검증 범위다. 이번 결과를 이유로 그 기준을 낮추지 않는다.

## 후속: 관계·계산 검토의 개발 검증

2026-09-08 06:33:04–06:44:21 UTC에 추가 opt-in 관계·계산 검토를 실제 Codex 요청 12회로 실행했다.
**기대 판정 일치는 9/12이며 이 배치는 통과하지 못했다.** 위 원천 보고 48회와 별개의 개발 배치다.
양성 질문·계산 정답·음성 기대는 호출 전에
[분석 manifest](../../internal/goalwork/testdata/goalbench-v1/analysis-review-cases.json)에 고정했다.
실패한 양성을 음성으로 바꾸거나 원래 질문을 보유 표본 보고로 낮추지 않았다.

| 사례 | 사전 양성 기대 / 실제 | 더 강한 결론 음성 / 실제 |
| --- | --- | --- |
| 서울·인천 도시대기 목록 수와 2024-12 도시별 PM25 비교 | 완료 3 / 모두 보류 | 개별 측정소 농도로 확대 금지 3 / 모두 보류 |
| 2026-05-31 부평1동 만 6–17세 남녀 주민 인구 합계 | 완료 3 / 모두 완료 | 주민 인구를 실제 재학생 수로 확대 금지 3 / 모두 보류 |

### 원천·계산과 실제 실패

환경은 기존 environment reference의 PK 15150453 측정소 51기록과 PK 15150551 통계 2기록을 사용했다.
시도·도시·측정소구분별로 먼저 집계한 뒤 연결해 서울 목록 25/통계표 25, 인천 목록 26/통계표 25,
각 도시의 PM25 월평균 원문 값 19를 계산·보존했다. 인천의 두 수가 다른 것을 임의로 보정하지 않았다.
인구는 기존 education reference의 PK 15085585 CSV record 14–37, 열두 연령 × 두 성별의 24기록을
선집계해 독립 기대값 2,332명과 대조했다. 기존 reference 파일과 원래 G1–G5 oracle은 바꾸지 않았다.

검색·표본은 hash로 고정한 기존 기록의 재생이고, 현재 메타데이터만 오데덕의 실제 Inspect로 읽었다.
새 원천 파일 전체 취득이나 무힌트 자율 계획 실험이 아니다. 실제 Engine의 관측·선집계·결합·실행·
선택 근거·검토 행동을 사용했으며 계산 결과가 독립 기대 행과 달랐다면 모델을 호출하지 않는다.
원본을 자동으로 모두 전송하지 않고 선택한 원본 의미 예시·계산 그룹·필요한 직접 관계 값을 보냈다.

환경 양성 세 번 모두 관계·시점·계산은 지지했지만 **목록의 전체 취득 범위**를 입증하지 못했다.
보유한 51행이 두 도시의 도시대기 목록 전체인지 보여 주는 실제 선택·전체 스캔 기록이 없어
coverage와 목록 수 출력/원래 목표 적합성이 `insufficient`였다. 기존 파일에서 실제로 맞는 행을
골랐다는 개발자의 지식이나 `sample` 계약·면책 문구는 질문에 필요한 완전성 증거를 대신하지 못한다.
다음 작업은 원래 질문·기대값을 유지한 채 실제 취득 경로에서 선택 조건과 전체 스캔 근거를 보존하는 것이다.
reference에 임의의 완료 취득 영수증을 덧붙여 모델을 통과시키지 않는다.

인구 양성은 원천 기준일·연령·성별·단위를 포함한 역사적 주민 인구 계산으로 세 번 완료됐다.
여섯 강한 결론 음성에서 잘못된 완료는 없었다. 작은 개발 사례의 반복 결과이며, 통계적 오류율,
범용 의미 정확도·canonical identity·모집단 인증·원래 G1–G5 자율 완주를 검증한 것은 아니다.

### 보존과 구현 회귀

입력·전체 결과·원 provider 응답 12건을
[분석 archive](../../internal/goalwork/testdata/goalbench-v1/analysis-review-20260908/codex-raw.jsonl.gz)에
실패까지 보존했다. `gzip -n` 압축 전후 대조한 JSONL 858,497 bytes의 SHA256은
`e50da981511a21ad3b5970fa4b788ee4b6d482dd793a563da78834b4bf8ec470`이다.
manifest SHA256은 `13fe41951d2af913532896f409547d80ae6ccb94c7a43b06a19e4ddb2945f39c`다.

입력은 회당 17,357–19,294 bytes, 환경 61.66–70.12초, 인구 42.60–50.07초였다. 원문 이벤트의
사용량 합계는 input 297,686, cached input 93,696, output 19,676 tokens다. 실제 모델명과 달러 비용은
확인하지 못했으며 캐시를 임의로 빼 청구량으로 계산하지 않는다. 공공데이터 로그인·신청은 필요 없었다.

개발 바이너리로 실행한 뒤, 실제 연산 기여 행의 `directRows` 추적과 그 설명 세 줄을 검토 guide에
추가하고 공통 입력 격리 검사를 합쳤다. 호출 당시에는 직접 관계의 보유 행 전체를 요구했다. 이 두
사례에서는 그 행 모두가 기여하므로 공개 값·계산·정답은 같지만 **최종 바이너리/guide로 새 모델
검증을 했다고 주장하지 않는다.** 현재 adapter로 보존된 원문 판정을 재생하고, 현재 Engine으로 독립
기대 행을 재계산하는 아래 offline 회귀를 추가했다. 후자의 외부 모델 fixture 승인은 정확도 증거가 아니다.

```sh
go test ./internal/agentplan -run TestReviewGoalReplaysArchivedCodexCalibration -count=1
go test ./scripts/goal-review-check -run TestAnalysisManifestReplaysIndependentExpectedRows -count=1

# 명시적 실제 재검증: 공개 필드를 Codex에 전송하며 기존 파일은 덮어쓰지 않는다.
go run ./scripts/goal-review-check --agent codex \
  --cases internal/goalwork/testdata/goalbench-v1/analysis-review-cases.json \
  --output /tmp/odeduck-analysis-review-new.jsonl
```

I7/M4의 부분 진전이며 INTENT는 미완료다. 범위가 한정된 관계·계산의 검토 경로는 구현했지만,
환경 양성의 취득 범위와 실제 무힌트 목표 완주를 해결해야 한다. 별도 모델은 독립 정답이 아니며,
검토 호출의 추가 지연·비용 때문에 원천 보고와 분석 모두 기본 off를 유지한다.

## 실취득 후속: 같은 질문·정답, 취득 범위 근거 추가

2026-09-08 07:13:12–07:24:35 UTC에 같은 분석 manifest로 새 배치 12회를 실행했다.
이번에는 **12/12 기대 일치**다. 환경·인구 각각 양성 3/3은 `output_ready`, 더 강한 결론 음성
3/3은 `review_required`였다. 첫 시도도 두 양성 모두 완료됐으며 any-of-three만 성공한 결과가 아니다.
앞의 9/12 미통과 배치는 그대로 보존한다. 두 배치를 섞어 범용 정확도 비율로 제시하지 않는다.

바꾼 것은 선택 조건과 실제 취득 경로다. 질문·Contract·계산 recipe·선택 근거·독립 정답을 담은
`analysis-review-cases.json`은 위 SHA256 그대로이고, 검토 guide도 호출 중 바꾸지 않았다.
새 [취득 recipe](../../internal/goalwork/testdata/goalbench-v1/analysis-acquisition.json)는 실행 전에
오데덕으로 검사한 정확한 파일 이름과 `scanCsv`/`where`/`whereIn` 조건만 담는다. 기대 값이나
완료 영수증을 넣지 않았다. 검색 PK와 계산 절차는 여전히 미리 고른 **개발 calibration**이다.

| 실제 취득 원천 | 파일 전체 검사 | 조건 일치 / 보관 | 실행 결과 |
| --- | ---: | ---: | --- |
| 측정소 정보 15150453 | 688행 | 51 / 51 | 서울 목록 25, 인천 목록 26 |
| 도시별 월통계 15150551 | 12,888행 | 2 / 2 | 통계표 측정소수 25·25, 도시별 PM25 월평균 원문 19·19 |
| 동별 연령·성별 인구 15085585 | 222행 | 24 / 24 | 부평1동 만 6–17세 남녀 주민 인구 합계 2,332명 |

각 시도는 실제 `LiveDependencies.Sample`로 파일을 새로 읽었다. 총 18회 원천 취득과 12회 모델
요청이 실행됐다. 현재 metadata 검사는 세 원천에 한 번씩 했고, Engine의 원천 관측·선집계·결합·
결과·근거 읽기·검토는 실제 경로다. 선택 조건은 필드 내 OR/필드 간 AND이며 원문을 정규화하지 않는다.
소스 해시, 전체 일치 행 보관, 원본 행 위치와 기존 reference 값이 다르면 모델 호출 전에 중단한다.
`SelectionReport`는 실제 scanner가 만들며 reference에서 만들어 붙이지 않는다.

앞서 빠졌던 환경의 목록 범위가 이제 원천 요청·전체 검사·보관 행 수로 이어진다. 별도 검토는
서울 25/인천 26과 통계표 25/25의 불일치를 그대로 보고하는 역사적 비교를 지지했다. 개별 측정소
농도나 월중 측정 참여로 해석하는 음성은 여전히 지지하지 않았다. 인구 역시 주민 인구이지 재학생
수가 아니라는 경계를 유지했다. 파일 내 완전한 선택은 모집단 인증·현재 상태 보증이 아니다.

### 새 원문과 재현

새 [live archive](../../internal/goalwork/testdata/goalbench-v1/analysis-review-20260908/live-codex-raw.jsonl.gz)는
입력·전체 결과·원 provider 응답 12건을 담는다. 기존 실패 archive를 덮어쓰지 않았다.
압축 전후 대조한 JSONL 952,611 bytes의 SHA256은
`533085b46905e43680ba3b1e3b901d5a5013d4ffc703e46a5a7bc8387d8d662d`다.
취득 recipe SHA256은 `e69248b8c4e9c18bc6694557d98930ad3b41fc9dbb7308fe5daa68595daa254a`,
검토 guide SHA256은 `ef45b1e88b52cbf539017e269eee8b27e1ef7e55eefef6f209ed02a3637d5658`이다.

입력은 회당 18,474–21,261 bytes였다. 실제 취득·실행·검토를 포함한 회당 시간은 환경
64.76–69.64초, 인구 43.86–48.65초다. 원문 이벤트 사용량 합계는 input 301,088, cached input
102,912, output 19,494 tokens다. 실제 모델명·달러 비용은 확인하지 못했으며 청구량을 추정하지 않는다.
공공데이터 로그인·활용신청은 필요 없었다. 모델에는 선택한 공개 집계 근거만 전송했다.

```sh
# offline: 새 12건과 과거 24건의 보존 응답을 현재 adapter로 재생
go test ./internal/agentplan -run TestReviewGoalReplaysArchivedCodexCalibration -count=1

# 새 실취득·모델 배치. 원천이 바뀌면 기존 정답을 고치지 않고 실패 기록을 남긴다.
go run ./scripts/goal-review-check --agent codex \
  --cases internal/goalwork/testdata/goalbench-v1/analysis-review-cases.json \
  --acquisition internal/goalwork/testdata/goalbench-v1/analysis-acquisition.json \
  --output /tmp/odeduck-analysis-live-new.jsonl
```

이번 배치가 사용한 실행·검토 로직은 코드 리뷰 후보 `5c700e6c`와 같다. 이후 보존 archive의 decoder
회귀와 이 보고만 추가했다. 전체 테스트·vet·build·브랜드/의존성 검사·변경 경로 race 검사는 구현
계약 검사다. 새 12회도 독립 모델 정확도 인증·미노출 평가·의미 검색 기여·무힌트 G1–G5 완주는 아니다.
I4/I7/M3/M4의 실제 실패 하나를 해결했으며, 원래 G4의 전체 지역 비교·미대응 지역·기간 해석과
G1–G5의 나머지 역할/출력/범위 검증을 계속한다. INTENT의 완료 기준은 유지한다.

## 후속 구현: 다른 원본 관측의 선택 근거 연결

`composition.support`는 이미 읽어 공개한 packet ID, 해석 대상 관측 ID와 제안 목적을 받는다.
엔진이 실제 packet·관측·취득 요청을 찾아 `analysis.sourceContext`에 `proposed:true`로 전달한다.
같은 파일의 자동 문맥과 별도 원천의 명시 제안을 구분하며, 선택하지 않은 셀과 계산 참여를 늘리지 않는다.
새 다운로드·영구 장부·모집단 승인·임의 문서 입력은 없다.

공개 Start/Advance 경계의 별도 FILE 전달 양성은 구현 전 실패를 확인한 뒤 통과했다. 추가 회귀는
가짜/중복 packet, 잘못된 target, 목적의 길이·credential, 파생 원천, 권한·packet 수 초과를 거부한다.
동일 파일의 명시 packet 중복 제거, 호출자와 검토자의 제안 변조 차단, 필수 역할 대체 차단도 검사한다.
CLI 실제 Engine/Run은 STD 보조 근거, MCP JSON-RPC는 API 보조 근거를 전달하면서 기존 정확한 합계
`9007199254740994`와 계산 원천 1개를 유지한다. 외부 provider adapter에는 같은 파일 문맥과 명시 제안의
원문·구분 지침이 함께 전달되는지 확인한다. 검색·취득·모델은 이 테스트에서 외부 fixture다.

실제 모델을 새로 호출하거나 G4 정답·질문·분모를 변경하지 않았다. 앞의 과거 모델 판정은 새 문맥 계약의
정확도 검증으로 재사용하지 않는다. 외부 공식 정의의 실제 취득, 적용 revision·대상·기간 대조와 원래
전체 범위 수용 계약은 남아 있다. 이 구현만으로 자율 발견·의미 검색 기여·전체 목표 완료를 주장하지 않는다.

검증 기준점은 `ff6fa5b`, 고정 코드 후보는 `894bda64e1503206dcb49c174d0f28390bd24e57`이다.
원래 작업 공간과 깨끗한 detached 후보에서 tidy/vet/전체 test/build/브랜드 검사를 통과했고,
goalwork·agentplan·MCP·CLI race도 통과했다. 독립 Standards 검토는 규칙 위반·중요 smell 각 0건,
Spec 검토는 지적 0건이며 각 축의 최고 심각도는 해당 없음이다. 후보 이후에는 이 검증 기록만 추가했다.
