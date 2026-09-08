# 공식 보조 문서의 선택 관측 v1

상태: 아래의 제한된 읽기·전달 계약 구현 및 회귀·실취득 검증 완료. 실제 정의 적용과 제품 목표 완주는
미완료다. [INTENT.md](../../INTENT.md) 및
[실행·검증 계획](goal-driven-completion-plan.md)의 I4/I5/I7/I9와
[선택 근거 연결](goal-source-report-review-v1.md#다른-관측의-선택-근거-연결--2026-09-08-후속-계약)의 취득 공백을 잇는다.
사용자의 설계·구현 위임에 따른 선택이며 G1–G5의 질문·oracle·완료 조건은 유지한다.

## 결정

기존 `inspect → sample → read_evidence → support → review_result`를 확장한다. 새 취득 작업
체계와 자동 문서 수집은 도입하지 않는다. `dataset` module이 참조 인식·공개 HTTP·HTML 추출·
원본 위치를 소유하고 CLI/MCP는 기존 공통 Engine을 사용한다. 문서를 SourceDeclaration에
자동 삽입하면 선택 공개를 우회하므로, 별도 불변 원본 관측으로 보관한다.

## 읽기 계약

- FILE inspection의 `documents`는 읽을 수 있는 등록 참조의 ID·URL·제목·adapter revision·
  검증일·발견 출처를 제공한다. 본문은 제공하지 않는다. `portal_link`는 실제 포털이 광고한 링크,
  `adapter_reference`는 공식 문서에 근거해 등록한 보조 참고자료다. 후자를 포털 링크로 표시하지 않는다.
- `sample`은 `delivery:"document"`, `document:{referenceId,contains?}`를 받는다. ID는 같은
  목표에서 검사한 계약의 참조여야 한다. contains는 로컬에서 적용하는 최대 128 UTF-8 byte의
  선택적 리터럴 부분문자열이며 URL·CSS·원문·hash·HTTP params는 입력받지 않는다.
  기존 file/API/STD 선택자·파생 연산과 혼합하지 않는다.
- 정확히 등록한 HTTPS host/path/query에 credential 없는 GET만 수행한다. cookie jar·authorization·
  browser·폼 실행·링크 재귀 탐색과 redirect는 사용하지 않는다. 문서당 15초·1 MiB를 제한하고
  200 UTF-8 HTML만 읽는다. 원문 hash와 등록된 추출 범위·revision을 보존한다.
- adapter가 정의한 DOM section을 원문 순서로 읽는다. HTML entity를 해독하고 block 구분과
  whitespace를 정규화한 `text`를 보관한다. 이는 요약/번역/의미 매핑이 아니다. DOM의 원본 경로와
  전체 정규화 section의 text hash가 인용 위치다. 선택 전 section 수·일치 수·보관 수를 남긴다.
  section 누락/개수 drift, 비어 있는 선택, 20개 초과 일치, 선택한 셀의 2048 JSON bytes 초과는
  잘라 성공시키지 않고 오류로 남긴다. HTML 전체나 논리적 모집단을 읽었다고 주장하지 않는다.
- 반환은 `DOCUMENT` 관측이며 원문은 기존 메모리·취득 시도·재시도·만료·8회 sample/8 MiB 예산에
  포함한다. 원본 URL과 발견 근거는 관측 안에 보존하고 data.go.kr PK의 데이터 본문으로 오인하지 않는다.
- `read_evidence`는 같은 기본 비공개·고정 수신자·8 packet/64 KiB 제한을 적용한다. 위치 종류는
  `document_block`이며 원문 URL/해시·추출 revision·DOM 경로를 추적한다. 별도 문맥 연결은 기존
  명시적 `support`만 사용한다. 문서 관측은 계산·필수 데이터 역할·population 자격을 채우지 않는다.
  취득 근거, 제안된 적용, 실제 의미 판정은 별개다.

## 최초 읽기 계약과 근거

등록은 목표 ID나 특정 데이터 PK가 아니라 실제 외부 제공 URL family로 선택한다.

- `https://jumin.mois.go.kr/ageStatMonth.do`: 포털에서 광고된 경우에만 월간 통계 설명을 읽는다.
  `form[name=search] .popoverBox .pContent`의 세 PC 도움말 section이 읽기 범위다.
- 같은 통계 family의 보조 참고문서:
  `https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124`. `adapter_reference`로 표시하고,
  실제 게시물 ID와 `.answers > .tbx`, `.answers > .an_txt`가 각각 정확히 하나인지 확인하고,
  합집합도 두 section인지 검사한다. 한 구역의 누락을 다른 구역의 중복으로 상쇄하지 않는다.
  질문자 본문이나 화면 탐색 영역을 수집하지 않는다. 만 나이 정의를 반환값에 만들어 넣지 않는다.

관측 근거·원문 위치·적용 한계는
[인구 원천 조사](../research/population-source-applicability-2026-09-08.md)에 있다. 특정 July 파일에
방법론이 적용된다는 판단과 공식 export의 코드·수치 대조는 이 읽기 계약이 자동 승인하지 않는다.
임의 KOSIS 검색이나 별도 데이터 카탈로그를 여는 변경이 아니다.

## 검증

위임에 따라 기존 Inspector/LiveDependencies·Start/Advance·CLI command·MCP JSON-RPC를
검증 seam으로 사용한다. HTTP만 fixture adapter로 교체하고 실제 HTML 파싱·근거 생성·공개·검토
전달을 검증한다. URL 변조·redirect·cookie·비정상 HTML·초과 크기·drift·미공개 값·계산 전용성·
revision 변조를 음성으로 고정한다. 실제 두 문서 재취득은 별도 opt-in live contract 검사다.
테스트 통과·원문 전달만으로 모델 적용 정확도나 G4 완주를 주장하지 않는다.

이후 필요한 다른 원천의 읽기 계약은 같은 원천 조사·정확한 요청·fixture·live 확인을 거쳐 추가한다.
PDF/OCR, 임의 URL, 월간 통계 export 실행 및 실제 목표의 의미 검토는 별도 남은 작업이다.

### 2026-09-08 실취득

`ODEDUCK_DOCUMENT_LIVE=1 go test ./internal/dataset -run '^TestSupportingDocumentLiveContracts$' -count=1 -v`
로 PK 3033304의 실제 inspection에서 두 참조를 얻어 재취득했다. 로그인·쿠키·모델 호출 없이
행안부 3 section과 KOSIS 2 section을 보관했고 각 셀은 2048 JSON bytes 이하였다.

| 원천 | HTML bytes | HTML SHA-256 | section별 JSON bytes |
| --- | ---: | --- | --- |
| 행안부 월간 도움말 | 141860 | `971e26e6e7da62716a41f058ccd2215d8b90b02e8e86f4b86244965e443df28e` | 606 / 489 / 331 |
| KOSIS 답변 22124 | 83632 | `84e9e75d7f80d662dc9895c5f3ac34a8cb8e59585db43d61e8cef4d506e9b6ca` | 39 / 849 |

추출 revision은 `html-block-space-v1`이다. source HTML은 변할 수 있어 live 검사에서 이 해시를
정답으로 고정하지 않는다. 원문 전달·적용 해석·목표 완료는 별개이며, G4의 기존 7시도/5모델 호출/
목표 완료 0회는 이 transport 검사로 바뀌지 않는다.

### 구현 검토와 회귀 확인

구현 `6881c5f`와 후속 수정 `e051689`를 `4346156` 기준으로 독립적인 두 축에서 검토했다.

- Standards: 강제 위반 0건. 추출기와 Engine에 중복된 revision·용량 계약이라는 설계 의견 1건은
  `dataset` 소유 상수로 모아 해결했다. Engine의 독립적인 출처·원문 검증은 유지한다. 재검토 잔여 0건.
- Spec: KOSIS의 필수 구역 누락을 다른 구역의 중복이 가릴 수 있는 P2 1건을 발견했다. 공개
  Inspector 테스트에서 두 방향의 오통과를 먼저 재현한 뒤 selector별 개수 검사를 추가했다.
  부분 관측 반환도 거부하며 재검토 잔여 0건이다.

수정 후 `go mod tidy -diff`, `go vet ./...`, `go test ./...`, `go build ./...`, brand 동기화
`--check`, `git diff --check`를 통과했다. `dataset`·`goalwork`·`fetch`·`mcpserver`·CLI package의
race 검사도 통과했다. 일반 회귀는 외부 HTTP/모델 경계 fixture이며 실제 모델 정확도 측정이 아니다.

같은 opt-in live 검사를 수정 후 다시 통과했다. HTML bytes·각 section의 원본 위치·text SHA-256·
JSON bytes는 첫 취득과 같았다. 전체 HTML SHA-256은 행안부
`8e245c331da918657d4a80fc34e02fdcd32ea515a1bbc6541c6ef6706e1fa783`, KOSIS
`8ee24c4d3ad5797cf55f107dc9a90590247071531ad63f46750bedb8541a588c`로 바뀌었다.
전체 HTML의 동일성과 선택 구역의 동일성은 구분하며, live 원문 전체를 이번 문서에 보관한 것은 아니다.

다음 실제 G4 진단은 기존 `with-table-context`가 이미 8개 근거 packet을 사용하는 점을 먼저
해결해야 한다. 중복 공개를 줄이거나 근거 선택을 재구성하되 필요한 원천 정의·계산 입력·전체 범위를
버리거나 예산을 넓혀 통과시키지 않는다. 이 준비를 실제 모델 호출이나 새 G4 시도로 집계하지 않는다.
