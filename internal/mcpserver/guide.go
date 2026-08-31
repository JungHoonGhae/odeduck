package mcpserver

// GuideDoc is the gongctl://guide resource. It keeps the normal path deliberately
// small: thousands of portal endpoints stay behind three generic MCP tools, and
// only one compact candidate list and one selected specification enter context.
const GuideDoc = `# gongctl — data.go.kr 사용 가이드

## 기본 경로: 검색 → 상세 → 필요시 AI 활용신청 → 호출

### 1. catalog_search(query, concepts?)
모든 탐색은 여기서 시작한다. 구체적인 데이터명·현상을 찾는 요청이면 query만 사용한다. 사용자가
"돈 될 만한 것", "새 서비스를 만들 기회", "대한민국에서 지금 달라지는 것"처럼 목표만 말했으면
그 말을 키워드로 잘라 넣지 않는다. 대화 맥락을 이해하는 네가 먼저 2~8개의 구체적인 데이터 축을
추론해 concepts에 함께 전달한다.

- concepts는 동의어 목록이 아니다. 직접 대상, 인접 시장, 선행지표, 제약·위험, 다른 기관 관점처럼
  서로 다른 가설이어야 한다. 예를 들어 수익 기회라면 공매 저감률, 조달 입찰, 도매 경매가격,
  상권 매출, 지원사업, 공급망 변화를 별도 축으로 볼 수 있다. 사용자가 이 단어를 먼저 말할 필요는 없다.
- planned/hybrid 결과는 각 hit의 matchedQuery로 어떤 축에서 발견됐는지 밝힌다. 축별 결과를
  round-robin으로 섞으므로 한 기관의 비슷한 API가 첫 화면을 독점하지 않는다.
- ranking=balanced(기본)는 활용 수요가 검증된 결과와 최근 수정된 저활용 결과를 함께 본다.
  이미 알려진 API를 찾으면 demand, 신설·변화를 우선 탐색하면 recent를 쓸 수 있다.
- concepts가 있는 탐색은 공식 설명의 짧은 preview를 기본 반환한다. 제목만으로 용도를 단정하지 말고
  preview로 후보를 이해한 뒤 pk 하나를 상세 조회한다.
- semantic.status=used면 선택 설치된 Ollama 벡터 인덱스까지 결합한 것이다. not-indexed/unavailable이면
  의미 분해 concepts + 결정적 로컬 검색으로 폴백한 것이며 검색 자체는 계속 유효하다.

- restOnly는 생략하면 true다. 따라서 기본 결과는 describe_api와 call_api로 이어갈 수 있는
  REST 데이터셋뿐이다.
- 호출이 목적이 아닌 전체 현황 조사에만 restOnly=false를 사용한다. 이때 나오는 LINK 데이터셋은
  포털에 명세가 없고 제공기관의 별도 사이트·계정·인증키가 필요할 수 있다.
- lexical 결과의 relaxed=true면 모든 검색어를 만족하는 결과가 없어 일부 단어만 맞는 후보까지 확장한 것이다.
  terms와 각 hit의 matched를 보고 관련성을 다시 판단한다.
- stale=true면 최근 신설 API가 빠졌을 수 있다. ` + "`gongctl catalog sync`" + `로 갱신한다.

후보를 하나 고른 뒤 그 hit의 pk를 describe_api에 넘긴다. 검색 결과만 보고 엔드포인트나
파라미터를 추측하지 않는다.

### 2. describe_api(pk)
선택한 데이터셋 하나의 상세기능, 실제 엔드포인트, 요청변수, 심의유형을 반환한다.

- operations에서 호출할 상세기능을 고른다. op 값은 해당 endpoint의 마지막 경로 조각이다.
- params의 Required가 필수인 요청변수를 구성한다. sample은 예시일 뿐 실제 요청 의도에 맞게 바꾼다.
- params가 비고 rawHtml만 있으면 rawHtml에서 표를 확인한다.
- operations가 비고 note가 있으면 guideDocUrl 또는 linkUrl의 문서를 먼저 읽는다. 문서가 없으면
  그 후보는 호출하지 말고 다른 API를 찾는다. 파라미터를 추측하지 않는다.
- 미승인 API라면 approval.dev를 먼저 확인한 뒤 아래 2.5단계로 이어간다. 심의승인은 제공기관의
  사람 승인이 필요하고, 자동승인은 신청 직후 승인 상태가 된다.

### 2.5. apply(pk, purpose, category) — 미승인일 때만
AI가 선택한 OpenAPI의 활용신청을 실제 제출한다. purpose에는 사용자의 목표와 활용 방식을 구체적으로
요약하고 category는 실제 용도에 맞춰 web, app, research, ref, etc 중 하나로 분류한다. 계정에 신청
기록을 남기는 외부 변경이므로 MCP 클라이언트의 도구 승인 정책을 따른다.

- 이미 신청한 API는 다시 신청하지 말고 list_applications로 승인 상태를 확인한다.
- 개발단계 자동승인이면 신청 결과를 확인한 뒤 3단계 call_api로 바로 이어간다.
- 로그인 세션이 없으면 사람에게 ` + "`gongctl login`" + `을 안내한다. 로그인 이후에는 브라우저 조작,
  인증키 복사, 신청 폼 입력을 AI가 대신한다.

### 3. call_api(pk, op, params)
describe_api에서 확인한 pk·op·params로 승인된 API를 호출한다. MCP 입력에는 raw endpoint와
serviceKey가 없다. gongctl이 pk로 명세를 다시 읽고 엔드포인트를 결정하며 필수 파라미터 누락을
검사한 뒤 로그인 세션의 인증키를 주입한다. XML 응답은 JSON으로 변환한다.

- 상세기능이 하나면 op를 생략할 수 있다. 여러 개면 describe_api에서 확인한 op를 지정한다.
- body의 resultCode 또는 제공기관별 성공 코드를 확인한다. HTTP 200만으로 성공을 판단하지 않는다.
- 미승인 오류면 아래의 계정 보조 도구를 사용한 뒤 같은 pk로 다시 호출한다.
- 방금 승인된 API라면 waitSeconds=300을 줄 수 있다. 403은 승인 실패나 키 오류가 아니라
  게이트웨이 전파 대기일 수 있으므로 다시 신청하거나 키를 바꾸지 않는다.

## 보조 도구

- search_datasets(keyword): 카탈로그가 stale이거나 최신 신설 항목을 라이브로 재확인할 때만 쓴다.
  포털의 현재 결과 페이지만 보므로 catalog_search를 대신하지 못한다.
- list_applications(): 이미 활용신청한 API와 상태를 확인한다.

## 인증키와 승인 전파

data.go.kr은 같은 serviceKey를 Encoding/Decoding 두 형태로 표시한다. 인증키는 MCP 도구나 모델
컨텍스트에 노출되지 않으며, call_api가 세션에서 내부적으로 읽고 전송에 맞게 처리한다. 포털의 활용신청 상태가
승인이어도 게이트웨이 반영에는 보통 수 분, 안내상 최대 1시간이 걸릴 수 있다.

## 오류를 숨기지 않는다

data.go.kr은 HTTP 200 본문에 오류코드를 담기도 한다. call_api가 반환한 status와 body를 함께 읽고,
필수 파라미터 누락·키 거부·403 전파 대기를 서로 다른 문제로 다룬다.
`
