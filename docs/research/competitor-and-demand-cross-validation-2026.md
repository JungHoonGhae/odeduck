# 한국 공공데이터 AI 도구 경쟁·수요 교차 검증

조사일: 2026-09-01

## 결론

oddsock은 무경쟁 제품이 아니다. 한국의 전체 공공데이터 카탈로그를 자연어로 찾고 판단하는
경쟁 구현은 이미 여러 개 존재한다. 다만 공개 소스에서 확인한 범위에서는
`전체 카탈로그 발견 → API·FILE·LINK 실제 계약 검사 → 활용신청 → 승인 API 호출`을 한 제품에서
완결하는 구현은 oddsock이 유일하다.

이것은 현재의 제품 차별점이지 아직 방어 가능한 해자는 아니다. 카탈로그와 검색은 복제 가능하고,
활용신청 자동화는 포털 변경에 민감하다. 지속적인 우위는 provider adapter, 실제 호출 검증 결과,
사람이 검수한 검색 평가셋, 재현 가능한 사업 사례에서 만들어야 한다.

## 한국의 직접 경쟁군

| 프로젝트 | 2026-09-01 GitHub 신호 | 실제 범위 | oddsock과의 관계 |
| --- | ---: | --- | --- |
| [Public Data Lens](https://github.com/hike-lab/public-data-lens) | 82 stars, 19 forks | 약 9.6만 건, BM25, 비교·변경 추적·컬럼 검색·실파일 구조, 원격 MCP·웹 | 발견·판정 계층의 가장 강한 직접 경쟁자. 신청·범용 호출은 하지 않음 |
| [Public Data API Finder](https://github.com/boam79/public-data-api-finder) | 40 stars, 7 forks | 아이디어→키워드 확장·점수화, 포털 검색 API, Swagger 상세 | 자연어 추천 경쟁자. 검색 API와 개별 활용신청 키를 사용자가 준비해야 함 |
| [Korea Public Data Catalog MCP](https://github.com/obundh/korea-public-data-catalog-mcp) | 15 stars, 3 forks | 96,056건 정적 카탈로그, 원격 MCP·MCPB | 설치 없는 카탈로그 탐색 경쟁자. 실제 값·API 호출은 하지 않음 |
| [FieldCure PublicData MCP](https://github.com/fieldcure/fieldcure-mcp-publicdata) | 0 stars | discover→describe→call, 키 elicitation, 응답 정규화 | API 실행 계층 경쟁자. FILE과 자동 신청은 없고 개별 신청은 수동 |
| [Public Data Opportunity MCP](https://github.com/capitalparser/public-data-opportunity-mcp) | 2 stars | 카탈로그 검색, 맥락 매칭, 자동화 적합도 | 사업 맥락 발견 경쟁자. 실데이터 검증·신청·호출은 하지 않음 |

[data-go-mcp-servers](https://github.com/Koomook/data-go-mcp-servers)(288 stars)와
[opendata-kr](https://github.com/opendata-kr)는 선별 API를 수작업 adapter로 제공하는 인접 경쟁군이다.
전국 카탈로그 발견 제품은 아니지만, 결과가 명확한 vertical adapter에 대한 수요와 기여 모델을 보여준다.

## 수요 판단

직접 제품의 stars만 보면 수요가 작아 보이지만, 대부분 2026년에 생긴 1인 초기 프로젝트다.
그중 Public Data Lens는 공개 후 약 3주 만에 82 stars와 19 forks를 얻었다. 더 구체적인 결과를 제공하는
[real-estate-mcp](https://github.com/tae0y/real-estate-mcp)는 373 stars·61 forks,
[archhub-mcp](https://github.com/chrisryugj/archhub-mcp)는 64 stars·10 forks다. 이는 사용자가
“공공데이터 인프라”보다 “아파트 가격·건축물 분석”처럼 즉시 이해되는 결과에 더 빨리 반응한다는 신호다.

해외에서는 [mcp-brasil](https://github.com/Mcp-Brasil/mcp-brasil)이 70개 브라질 공공데이터 소스,
533개 도구, BM25 도구 탐색과 교차 조회를 제공하며 1,742 stars·257 forks를 기록했다.
[CKAN MCP Server](https://github.com/ondata/ckan-mcp-server)는 로컬·원격 설치를 모두 제공하고,
이탈리아 디지털 정부기관 AgID가 재사용했다. 저장소에 공개된 호스팅 로그 기반 평가 자료는 1,824건의
실제 도구 호출을 출발점으로 삼는다. 국가 공공데이터를 AI가 다루게 하는 범주 자체에는 수요가 있다는
교차 증거다.

다만 stars는 설치·반복사용·지불의사가 아니다. 현재 증거로 말할 수 있는 것은 “문제와 개발자 관심은
검증됨”까지이며, oddsock의 자동 신청이나 교차 도메인 사업 발견에 대한 반복 사용 수요는 아직
검증되지 않았다.

## 가져올 설계

1. **공개 읽기 계층 + 로컬 권한 계층 분리**: CKAN MCP와 Public Data Lens처럼 검색·비교는 원격에서
   무설치로 제공하고, 로그인·쿠키·키·활용신청·호출만 로컬 oddsock이 담당한다.
2. **검색 품질 release gate**: Public Data Lens의 golden query와 nDCG/Recall 접근을 도입하되,
   자동 생성 label이 아니라 사람이 검수한 한국어 목적 질의·hard negative·no-connection 표본을 쓴다.
3. **증거 수준과 변경 추적**: catalog metadata, 실제 파일 관측, 실제 호출을 명시적으로 구분하고,
   월별 추가·삭제·변경과 저장된 질의 알림을 제공한다.
4. **Adapter lifecycle**: mcp-brasil의 package-by-feature와 auto-registry를 참고해 각 adapter에
   owner, contract version, last verified, fixture, health 상태를 둔다. 당장은 monorepo 단일 제품 버전을
   유지하고 adapter별 독립 배포는 생태계가 생긴 뒤 도입한다.
5. **계획·배치 실행**: mcp-brasil의 planner/batch 아이디어는 가져오되, 500개 도구를 노출하지 않는다.
   oddsock의 작은 `catalog_search → inspect_dataset → call_api` 표면은 유지한다.
6. **대용량 로컬 저장 형식 개선**: 100MB JSON 전량 로드와 flat float32 벡터 대신 indexed metadata,
   packed/quantized vector, mmap을 검토한다. 별도 vector DB는 평가로 필요성이 증명되기 전까지 보류한다.
7. **Vertical proof packs**: 상권 조기경보, 경매 실사, 조달 기회처럼 세 개 정도의 재현 가능한
   end-to-end 사례를 제공해 범용 인프라의 가치를 결과로 보여준다.

## 출시 전 교차 검토에서 확인한 설계 위험

- 기본 `catalog sync`가 풍부한 composite snapshot을 CSV-only snapshot으로 낮출 수 있다. 갱신은
  coverage가 단조 증가하거나 최소한 기존 품질을 보존해야 한다.
- 릴리즈 gate가 유형별 건수만 확인해 대표 자연어 질의의 top-k 회귀를 잡지 못한다.
- 일부 CSV 기반 계약의 provenance가 제한된 목록 API에서 온 것처럼 표시된다.
- 상세 계약에서 필수 여부를 확인하지 못해도 호출이 진행될 수 있다. credential 호출은 fail-closed여야 한다.
- 대용량 streaming download가 공통 60초 timeout에 묶여 느린 네트워크에서 중단될 수 있다.
- 새 경로 감사 기준 30개 중 18개가 자동 테스트됐고, auto fallback·릴리즈 설치·실제 신청/호출 등
  12개 경로가 추가 검증 대상이다. 이는 전체 line coverage가 아니라 위험 기반 경로 감사 결과다.

v0.14.0 구현은 위 위험 중 snapshot downgrade, lexical release regression, provenance, credential
fail-closed, streaming timeout을 각각 단조 coverage guard, 7개 reviewed golden query, evidence source,
선택 operation 계약 gate, 별도 10분 stream timeout으로 닫았다. 자동 fallback·릴리즈 설치·실제 신청과
호출의 운영 검증은 unit test와 별개로 계속 유지해야 한다. 이후 핵심 성공지표는 stars가 아니라 설치 후
첫 유효 데이터 발견 시간, inspect 성공률, 신청 완료율, 실제 호출률, 저장 질의 재사용률, 검증된 데이터
조합 수다.
