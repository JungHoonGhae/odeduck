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
  CSV data record 12460·12463, 원 관측 시각은 `2026-09-07T04:06:30.759848+00:00`이다.
  선택한 8개 필드에는 도시별 구분, 측정소수 25·25와 `1시간 월평균` 19·19가 포함된다.
- 인구: [공식 동별 연령·성별 자료](https://www.data.go.kr/data/15085585/fileData.do)의 기존 독립
  [교육·인구 원천 기록](../../internal/goalwork/testdata/goalbench-v1/education-reference.json)을 재생했다.
  CSV data record 14·15, 원 관측 시각은 `2026-09-07T04:04:54.160580+00:00`이다.
  선택한 3개 필드는 `연령(세)` 6, `성별` 남·여, `부평1동인구수` 64·62다. 합계나 재학 여부를 만들지 않았다.
- 당시 파일의 SHA256과 record 위치는 기존 reference 및 새
  [사례 manifest](../../internal/goalwork/testdata/goalbench-v1/source-report-cases.json)에 고정했다.
  이번 실행에서 파일 전체를 다시 내려받았다고 주장하지 않는다.
- 현재 메타데이터는 오데덕 `LiveDependencies.Inspect`로 각 공식 페이지에서 읽었다.
  검색과 sample 취득은 위 기록을 재생했다. 계획·PK·선택 필드를 미리 정했으므로 키워드 없는
  자율 발견 실험이 아니다. 상태 전이와 실행·근거·검토 결합은 실제 Engine을 사용했다.
- 공개 집계 기록만 명시한 provider로 전송했다. 공공데이터 로그인이나 활용신청은 필요 없었다.
  모델에는 기대 판정, 음성 사례 설명, 이전 검토 답, 전체 reference 파일을 보내지 않았다.

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
