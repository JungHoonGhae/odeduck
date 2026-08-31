# 공공데이터 MCP·CLI 경쟁 워크플로 감사

조사일: 2026-08-31
대상: 공개 GitHub 저장소와 공공데이터포털의 공식 페이지
판정 기준: README의 표현보다 해당 커밋의 MCP tool 등록, 검색 구현, 인증키 주입, HTTP 호출 코드를 우선했다.

## 결론

사용자 가설은 절반만 맞다. **범용 `검색 → 상세 → 호출` MCP는 이미 있다.** FieldCure는 이 세 도구를 명시적으로 제공한다. 그러나 검색은 의미 검색이 아니라 입력 문자열을 공공 카탈로그의 제목 `LIKE` 조건으로 그대로 보내며, 목록조회서비스와 호출 대상 API의 활용신청은 사용자가 포털에서 먼저 해야 한다.

이번에 조사한 공개 구현에서는 다음 전체 경로를 한 제품 안에 묶은 대안을 찾지 못했다.

```text
일상적인 목표 표현
  → 전역 카탈로그의 model-planned/semantic 탐색
  → 실제 오퍼레이션·필수 파라미터 확인
  → data.go.kr 활용신청 제출
  → 승인 상태·인증키 확인/재사용
  → 인증된 실데이터 호출
```

따라서 gongctl의 방어 가능한 차별점은 “최초의 검색·호출 MCP”가 아니라 **키워드 게이트웨이를 목표 기반 discovery로 넓히고, 기존 도구가 사람에게 돌려보내던 활용신청·승인 구간까지 원스톱으로 연결한다**는 것이다.

## 비교표

표기: ✅ 지원, △ 일부 지원·범위 제한, ❌ 미지원. “인증 호출”은 경쟁 서비스를 실제 호출했다는 뜻이 아니라, 조사한 소스에 인증키를 주입하는 호출 경로가 구현돼 있다는 뜻이다.

| 구현 | 자연어·semantic 전역 발견 | 점진적 catalog 흐름 | 활용신청·승인 처리 | 인증 실호출 | 소스 판정 |
|---|---|---|---|---|---|
| data.go.kr 웹 포털 | △ 웹 키워드 검색 | △ 검색→상세→Swagger 실행 | △ 사람이 로그인·신청·승인 확인 | ✅ 발급 키로 실행 | 공식 상세 페이지는 활용신청 버튼, 승인 유형, 요청변수와 서비스 URL을 함께 보여주며, 공식 Swagger 가이드는 활용신청으로 발급된 키를 사용한다고 설명한다. [상세 페이지](https://www.data.go.kr/data/15088949/openapi.do), [Swagger 가이드](https://www.data.go.kr/images/biz/swagger-guide/gw/gateway_swagger_guide.pdf) |
| `fieldcure/fieldcure-mcp-publicdata` | △ 광범위하지만 제목 `LIKE` 키워드 | ✅ `discover_api`→`describe_api`→`call_api` | ❌ 목록조회서비스와 개별 API를 먼저 수동 신청 | ✅ 임의 허용 호스트에 키 자동 주입 | 도구 설명이 세 단계 사용을 강제한다. [discover](https://github.com/fieldcure/fieldcure-mcp-publicdata/blob/33b5448a590d910a7fab908dec26912ef3a654d3/src/FieldCure.Mcp.PublicData.Kr/Tools/DiscoverApiTool.cs#L19-L50), [describe](https://github.com/fieldcure/fieldcure-mcp-publicdata/blob/33b5448a590d910a7fab908dec26912ef3a654d3/src/FieldCure.Mcp.PublicData.Kr/Tools/DescribeApiTool.cs#L19-L48), [call](https://github.com/fieldcure/fieldcure-mcp-publicdata/blob/33b5448a590d910a7fab908dec26912ef3a654d3/src/FieldCure.Mcp.PublicData.Kr/Tools/CallApiTool.cs#L18-L77). 실제 검색 조건은 `cond[list_title::LIKE]` 하나다. [HTTP 구현](https://github.com/fieldcure/fieldcure-mcp-publicdata/blob/33b5448a590d910a7fab908dec26912ef3a654d3/src/FieldCure.Mcp.PublicData.Kr/Services/PublicDataHttpClient.cs#L47-L84). 사전 수동 신청은 README가 명시한다. [prerequisites](https://github.com/fieldcure/fieldcure-mcp-publicdata/blob/33b5448a590d910a7fab908dec26912ef3a654d3/README.md#L38-L46) |
| `obundh/korea-public-data-catalog-mcp` 기본 `catalog` 모드 | △ 목표·data needs 입력, 검색은 SQLite FTS5/BM25 | △ 검색→메타 상세, 호출 없음 | ❌ 읽기 전용 | ❌ | 공개 도구는 검색·상세·조합 추천을 제공하지만 `call_api`가 없다고 명시한다. [도구 목록](https://github.com/obundh/korea-public-data-catalog-mcp/blob/10f82a28e06276360b5c35007d48c77c816e379f/README.md#L207-L225), [tool 등록](https://github.com/obundh/korea-public-data-catalog-mcp/blob/10f82a28e06276360b5c35007d48c77c816e379f/src/public-data-tools.ts#L265-L367). 검색은 FTS `MATCH`와 `bm25`다. [검색 구현](https://github.com/obundh/korea-public-data-catalog-mcp/blob/10f82a28e06276360b5c35007d48c77c816e379f/src/public-data-inventory.ts#L2434-L2568) |
| `capitalparser/public-data-opportunity-mcp` | △ 자연어 입력을 받지만 token substring 검색 | △ 검색→메타 상세 | ❌ README가 명시 | ❌ README가 명시 | `find_access_route`와 context matching이 같은 lexical catalog search를 호출한다. [도구](https://github.com/capitalparser/public-data-opportunity-mcp/blob/1a6f18a87a69655f59204684d4f2364bf2ce39e4/src/tools/register.ts#L33-L168), [검색 점수](https://github.com/capitalparser/public-data-opportunity-mcp/blob/1a6f18a87a69655f59204684d4f2364bf2ce39e4/src/catalog/search.ts#L16-L95). 호출·신청 제외 범위도 명시돼 있다. [현재 한계](https://github.com/capitalparser/public-data-opportunity-mcp/blob/1a6f18a87a69655f59204684d4f2364bf2ce39e4/README.md#L50-L58) |
| `hjsh200219/korea-public-data-mcp` | △ host LLM이 고정 action으로 라우팅 | △ 일부 도메인만 검색→상세 | ❌ 승인 오류를 안내할 뿐 제출하지 않음 | ✅ 고정 data.go.kr API | `public_data`는 약국·병원·온비드 등 열거된 12개 action 디스패처다. [action 목록과 등록](https://github.com/hjsh200219/korea-public-data-mcp/blob/b4a6818d68bfc30b8a22d68e20697863287ede1c/src/tools/skills/public-data.ts#L543-L617). 키가 있어야 도구가 등록된다. [config gate](https://github.com/hjsh200219/korea-public-data-mcp/blob/b4a6818d68bfc30b8a22d68e20697863287ede1c/src/tools/skills/index.ts#L44-L58) |
| `jinny-han/water-api-mcp` | △ 물 분야 인덱스의 token AND 검색 | △ 분야별 검색→호출, 독립 상세 단계 없음 | ❌ `apply_url`에서 사용자가 클릭 | ✅ 등록된 물 API | 검색 구현은 공백 token을 모두 포함하는지 검사한다. [검색 코드](https://github.com/jinny-han/water-api-mcp/blob/5e0e2bd1e86dca43821c8d3c7d5af573aee30584/src/water_api_mcp/clients/_dataset_based.py#L114-L165). 401/403이면 신청 링크에서 클릭하라고 반환한다. [호출·신청 안내](https://github.com/jinny-han/water-api-mcp/blob/5e0e2bd1e86dca43821c8d3c7d5af573aee30584/src/water_api_mcp/clients/_dataset_based.py#L167-L212) |
| `opendata-kr/narajangteo-bid-mcp` | △ 나라장터 한 도메인에서 자연어 tool routing | △ 공고 검색→단건·부가 상세 호출 | ❌ 그림 가이드로 수동 신청·승인 확인 | ✅ 나라장터 API | MCP가 검색과 단건 조회를 분리한다. [tool 등록](https://github.com/opendata-kr/narajangteo-bid-mcp/blob/1196d5e84a57dc23747127a26bdda7bdc917796a/src/server.ts#L30-L52). 별도 가이드는 “서비스 찾기→활용신청→승인 확인→키 복사”를 사람이 수행하게 한다. [신청 가이드](https://github.com/opendata-kr/narajangteo-bid-mcp/blob/1196d5e84a57dc23747127a26bdda7bdc917796a/docs/service-key-guide.md#L3-L71) |
| `Koomook/data-go-mcp-servers` | △ host LLM이 6개 고정 서버의 도구로 라우팅 | ❌ 전역 catalog 없음 | ❌ 미리 발급한 `API_KEY` 요구 | ✅ 고정 API | 저장소가 제공하는 서버 6종이 열거돼 있다. [목록·설정](https://github.com/Koomook/data-go-mcp-servers/blob/dd27f99490400b31fa14f96045a138fa217580a4/README.md#L45-L75). 나라장터 client는 고정 base URL과 환경변수 키로 호출한다. [client](https://github.com/Koomook/data-go-mcp-servers/blob/dd27f99490400b31fa14f96045a138fa217580a4/src/pps-narajangteo/data_go_mcp/pps_narajangteo/api_client.py#L12-L29) |
| `DatagokrGit/data-go-kr-mcp` | △ host LLM이 고정 도구로 라우팅 | ❌ 전역 catalog 없음 | ❌ 사전 수동 신청 | ✅ 대기질·날씨 고정 API | README는 현재 2개 서버와 사전 활용신청을 명시한다. [범위·준비](https://github.com/DatagokrGit/data-go-kr-mcp/blob/6e57780ade808cf6c7807716d9855cb3866966f2/README.md#L17-L35). 대기질 도구와 endpoint도 코드에 고정돼 있다. [tools](https://github.com/DatagokrGit/data-go-kr-mcp/blob/6e57780ade808cf6c7807716d9855cb3866966f2/typescript/packages/servers/air-quality/src/tools.ts#L6-L81), [client](https://github.com/DatagokrGit/data-go-kr-mcp/blob/6e57780ade808cf6c7807716d9855cb3866966f2/typescript/packages/servers/air-quality/src/client.ts#L4-L24) |
| `open-play-ground/datago-mcp` | ❌ MCP 검색 도구 없음 | △ 상세→샘플 호출 | ❌ 403이면 신청 필요 안내 | ✅ URL에 키 주입 | MCP tool은 상세, 샘플 호출, 필터 목록뿐이며 사용법도 포털에서 ID를 먼저 찾으라고 한다. [server](https://github.com/open-play-ground/datago-mcp/blob/4cb9fd658d515579e8f3d9cdcae4dfc76d3fd3af/src/datago_mcp/server.py#L35-L118). 호출 코드가 키를 주입한다. [client](https://github.com/open-play-ground/datago-mcp/blob/4cb9fd658d515579e8f3d9cdcae4dfc76d3fd3af/src/datago_mcp/client.py#L146-L175) |
| `JeHwanYoo/data-go-kr` CLI | ❌ | ❌ 사용자가 endpoint·operation·params를 구성 | ❌ | ✅ | 필수 config에 `serviceKey`, `authType`, `endpoint`, `serviceName`을 넣고 GET한다. [README](https://github.com/JeHwanYoo/data-go-kr/blob/43ca466e409ae1dc148ef7e9dd276553cbbdd2d3/README.md), [호출 구현](https://github.com/JeHwanYoo/data-go-kr/blob/43ca466e409ae1dc148ef7e9dd276553cbbdd2d3/app.js#L39-L82) |
| `kyusik-yang/open-assembly-mcp` | △ 국회 endpoint registry의 다중 필드 keyword 검색 | △ `discover_apis`→`query_assembly`, 별도 detail 없음 | 해당 없음: data.go.kr가 아닌 열린국회 키 | ✅ 열린국회 API | 발견 결과가 code·key params·notes를 주고 범용 호출로 이어진다. [discover/call](https://github.com/kyusik-yang/open-assembly-mcp/blob/b68133e4c351314f39b7022a00c808733f98f927/data_go_mcp/open_assembly/server.py#L1485-L1589). registry 검색이며 전역 공공데이터 검색은 아니다. [registry](https://github.com/kyusik-yang/open-assembly-mcp/blob/b68133e4c351314f39b7022a00c808733f98f927/data_go_mcp/open_assembly/registry.py#L1-L34) |

## 검증된 병목과 포지셔닝

기존 사용자 여정의 병목은 각각 다른 도구에 흩어져 있다.

1. 포털 또는 keyword MCP에서 **공식 용어를 알아야 검색**된다.
2. 후보가 실제 원하는 행과 필드를 주는지 모른 채 상세 명세를 따로 읽는다.
3. 로그인한 포털에서 API마다 활용목적·약관을 입력하고 신청한다.
4. 승인 상태와 Encoding/Decoding 키를 찾아 복사한다.
5. endpoint, 필수 파라미터, XML/JSON 오류 봉투를 직접 처리한다.

FieldCure는 1·2·5를 좋은 3단계 인터페이스로 묶었고, 정적 catalog MCP들은 1과 서비스 기획을 강화했다. 분야별 MCP들은 이미 승인된 좁은 API의 5를 편하게 만든다. **공통으로 남는 단절은 3·4이며**, 검색 단계도 대부분 `LIKE`, substring, FTS/BM25라서 사용자의 목표를 여러 기회 축으로 확장하는 semantic retrieval과는 다르다.

gongctl은 다음 메시지로 설명하는 편이 정확하다.

> 검색→상세→호출 MCP는 이미 있습니다. gongctl은 검색어를 알아야 하는 keyword gateway를 목표 기반 discovery로 확장하고, 에이전트가 찾은 API의 활용신청·승인·키 재사용·실호출까지 한 번에 끝냅니다.

제품의 핵심 흐름도 3단계보다 아래 5단계로 드러내는 편이 낫다.

```text
Discover → Inspect → Apply → Verify → Call
```

- **Discover:** Codex·Claude·Gemini·Cursor의 목표 분해와 선택형 Ollama vector 검색으로 묻힌 후보를 넓힌다.
- **Inspect:** 신청 전에 오퍼레이션, 필수 파라미터, 샘플과 승인 유형을 확인한다.
- **Apply:** 에이전트가 활용신청을 실제 제출한다. CLI는 확인 기본값, MCP는 자동화라는 실행 의미를 명확히 표시한다.
- **Verify:** 승인 상태와 계정 키를 재사용하고, 아직 전파 중인 승인과 잘못된 키를 구분한다.
- **Call:** dataset ID 기반으로 명세를 다시 확인한 뒤 인증키를 주입해 실데이터를 반환한다.

피해야 할 문구:

- “기존 MCP에는 검색→상세→호출이 없다.” — FieldCure가 반례다.
- “기존 MCP는 자연어 검색을 지원하지 않는다.” — 여러 서버가 자연어 tool routing이나 goal 입력을 지원한다. 정확한 차이는 retrieval이 lexical이라는 점이다.
- “세계 유일.” — 비공개·신규 구현까지 증명할 수 없다.

권장 문구:

- “조사한 공개 구현 중, 활용신청 제출과 승인·키 확인까지 포함한 원스톱 data.go.kr agent workflow.”
- “검색 결과 링크에서 다시 사람이 신청하던 마지막 병목을 없앤다.”
- “목표 기반 discovery에서 실제 승인 API 호출까지 이어지는 closed loop.”

## 방법과 한계

- 저장소 내 기존 비교 대상에서 시작해 GitHub/web 검색으로 광범위 catalog, 기회 탐색, 분야별 MCP와 CLI를 추가했다. 위 표는 2026-08-31에 확인한 각 고정 commit을 기준으로 한다.
- README만으로 기능을 확정하지 않았다. 가능한 경우 tool 등록, 검색 함수, HTTP client, 키 주입과 오류 처리 코드를 확인했다. 다만 “미지원”은 공개 tree의 tool 표면과 관련 문자열을 조사한 결과이며 숨겨진 외부 서비스까지 부정하지 않는다.
- 경쟁 도구에 개인 인증키를 넣거나 실 API를 호출하지 않았다. 따라서 인증 호출 ✅는 구현 존재 판정이지 운영 가용성·응답 정확성의 E2E 인증이 아니다.
- 공개 GitHub 검색은 전체 인터넷이나 비공개 제품의 완전한 목록이 아니다. 새 프로젝트가 생기면 “조사한 공개 구현 중”이라는 범위를 다시 갱신해야 한다.
- semantic 판정은 입력창이 자연어를 받는지가 아니라 retrieval 구현을 기준으로 했다. host LLM이 좋은 keyword를 만들어 줄 수 있어도, 서버 자체가 `LIKE`·substring·FTS만 수행하면 △로 표시했다.
