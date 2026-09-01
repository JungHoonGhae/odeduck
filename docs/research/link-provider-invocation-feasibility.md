# LINK provider 자동 호출 타당성 검토

검토일: 2026-09-01
범위: SafetyKorea, VWorld, 식품안전나라, 서울 열린데이터광장 일반 API·실시간 지하철 API
제약: 공식 1차 자료와 공개 `sample`/무자격 요청만 사용했다. 개인 credential, 로그인, 활용신청, 외부 상태 변경은 수행하지 않았다.

> **후속 구현 결정 (2026-09-01):** 아래 결론은 안전 기반을 먼저 분리 릴리즈하자는 조사 시점의
> 권고다. 이후 사용자가 일괄 릴리즈를 선택했고, 이 문서가 선행조건으로 적은 provider별 credential
> store, exact scope, escaped-secret redaction, HTTPS/redirect/size 제한, typed parameter validator와
> provider 오류 parser를 모두 같은 변경에서 구현했다. 현재 SafetyKorea, FoodSafetyKorea와 VWorld
> typed family는 `implemented`, 서울은 `blocked_insecure_transport`다. 실제 전후 효과와 남은 범위는
> [LINK 품질 평가](link-search-quality-evaluation.md)를 본다.

## 결론

**조사 시점에는 v0.12에 자동 호출을 넣지 않고 contract adapter와 drift 감시를 먼저 릴리즈하는 것이 맞다고 판단했다.**
자동 호출은 기술적으로 가능하지만 지금 추가하면 provider credential 저장, path/query secret 완전
redaction, exact endpoint validator, service별 parameter schema라는 새로운 보안 경계를 함께 출시해야 한다.
현재 `call_api`는 `apis.data.go.kr`와 그 `serviceKey`만 신뢰하도록 의도적으로 제한되어 있어 외부
provider를 우회적으로 통과시키면 안 된다.

| Provider | HTTPS 실제 확인 | 계약 규칙성 | 자동 호출 판단 | 권장 시점 |
|---|---|---|---|---|
| SafetyKorea | 지원 | 높음: GET 5종, header `AuthKey` | **구현 가능, 첫 대상** | 별도 v0.13 |
| 식품안전나라 | 지원(공식 표기는 아직 HTTP) | 중간: 공통 path + 서비스별 filter/schema | 구현 가능하나 inspector 선행 | v0.13 이후 |
| VWorld | 공식 HTTPS | 낮음: data/WMS/WFS/geocoder/search별 계약 상이 | family별 typed invoker만 가능 | 후속 릴리즈 |
| 서울 일반 | 미지원으로 관측 | 중간: 서비스별 path 인자 | **credential 호출 금지** | 공식 HTTPS 이후 |
| 서울 실시간 지하철 | 미지원으로 관측 | 중간: 별도 key·endpoint | **credential 호출 금지** | 공식 HTTPS 이후 |

공개 sample 호출만 별도 기능으로 서둘러 넣는 것도 권하지 않는다. sample은 rate/time 제한이 있고
실사용 credential 경로를 검증하지 못하므로 제품 기능보다 doctor의 선택적 canary에 가깝다.

## 공통 구현 선행조건

외부 invoker는 기존 LINK matcher와 분리된 작은 registry로 두고, raw URL이 아니라
`adapterID + providerServiceID + operation + validated params`만 입력받아야 한다.

1. **Credential 저장**: provider·scope별 namespace를 만들고 stdin 또는 OS secret store로만 입력한다.
   MCP argument, command line, catalog, APISpec에는 credential 값을 넣지 않는다. 파일 fallback을
   제공한다면 config directory 아래 `0600` 원자적 파일이어야 하며 logout/credential-delete 경로가 필요하다.
2. **Credential scope**: exact HTTPS origin과 검증된 path prefix에만 주입한다. redirect에는 header,
   query, path credential을 절대 승계하지 않는다.
3. **Redaction**: raw 값뿐 아니라 query-escaped, path-escaped, percent-encoded 형태도 URL, transport error,
   HTTP trace, cache key, profile, MCP response에서 가린다. 현재 data.go.kr query-key 전용 redactor를
   일반화해야 한다.
4. **Validation**: provider가 문서화한 operation·parameter name·enum·page 범위를 allowlist한다.
   임의 endpoint, 임의 path segment, 임의 query name을 받지 않는다.
5. **Transport**: credential 호출은 HTTPS만 허용하고 response size·timeout을 제한한다. DNS와 각
   redirect hop을 검사하는 safe fetcher를 사용하거나 redirect를 완전히 금지한다.
6. **오류 판정**: HTTP 200 안의 provider별 오류 코드도 실패로 해석한다. credential 자체가 URL path에
   들어가는 provider는 오류 메시지의 request URL까지 redaction test를 갖춰야 한다.

## SafetyKorea

### 공식 계약과 HTTPS

[Open API 사용방법](https://www.safetykorea.kr/release/openapi2)은 신청서를 이메일로 보내고 승인 후
인증키를 이메일로 받는 절차, 키 양도 금지, 인증키 필수를 명시한다. 따라서 자동 발급이 아니라
`manual_approval`이다. 공개 페이지와 설계서에서는 domain/IP 제한을 찾지 못했으므로 **없다고
단정하지 않고 미확인으로 둔다**.

[공식 인터페이스 설계서 v2.0](https://www.safetykorea.kr/resources/fileData/openapi/Open_API_%EC%82%AC%EC%9A%A9%EC%84%A4%EB%AA%85%EC%84%9C_v2.0.pdf)은
서비스 ID를 HTTP header `AuthKey`에 넣으며 대소문자를 구분한다고 설명한다. 문서 URL은 HTTP지만
다음 HTTPS endpoint가 실제 인증 경계까지 동작했다.

```text
GET https://www.safetykorea.kr/openapi/api/cert/certificationList.json
GET https://www.safetykorea.kr/openapi/api/cert/certificationDetail.json
GET https://www.safetykorea.kr/openapi/api/recall/recallList.json
GET https://www.safetykorea.kr/openapi/api/recall/recallDetail.json
GET https://www.safetykorea.kr/openapi/api/recall/fRecallList.json
```

목록 operation은 `conditionKey`·`conditionValue`, 상세는 `certNum` 또는 `recallUid`를 받는다.
인증·국내리콜·국외리콜의 허용 `conditionKey` enum과 응답 schema가 다르지만 endpoint 수가 유한하다.
설계서는 목록을 최대 1,000줄로 제한한다.

### 공개 probe

2026-09-01에 `AuthKey` 없이 HTTPS 인증목록을 한 번 요청했다. 서버는 HTTPS
`/openapi/api/error/accessDeniedByKey.json`으로 302한 뒤 HTTP 200과
`resultCode=4000` 인증키 오류를 반환했다. 이는 데이터 성공이 아니라 **HTTPS transport와 header
auth 경계가 실제 동작한다는 확인**이다.

### 최소 구현과 판단

`safetykorea` 전용 credential, 위 5개 exact path, operation별 enum/필수값, `AuthKey` header 무승계,
provider 오류 코드 parser가 최소 범위다. 네 provider 중 가장 작고 검증 가능하므로 첫 invoker로
적합하지만 credential subsystem 자체가 새 기능이다. **v0.12에는 넣지 않고 v0.13의 단독 목표로
분리한다.**

## 식품안전나라 (FoodSafetyKorea)

### 공식 계약과 HTTPS

[공식 데이터활용서비스 메인](https://www.foodsafetykorea.go.kr/api/main.do)은 로그인 계정에서 인증키를
발급받아야 하고 키는 한 개만 발급되며 양도할 수 없다고 안내한다. 서비스 상세의 오류 계약은 유효한
키여도 해당 서비스 활용신청이 없으면 `INFO-110`을 반환한다고 명시하므로 provider key와 서비스별
활용 권한을 둘 다 고려해야 한다. 공개 페이지에는 domain/IP 제한이 확인되지 않았다.

[C005 공식 상세](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C005)의 호출 골격은 다음과
같다.

```text
GET https://openapi.foodsafetykorea.go.kr/api/{keyId}/{serviceId}/{dataType}/{startIdx}/{endIdx}[/{filters}]
```

`dataType`은 XML/JSON이고 한 번에 최대 1,000건이다. filter는 `C005`의 `CHNG_DT`,
`PRDLST_REPORT_NO`, `BAR_CD`처럼 서비스마다 다르고, 출력 field도 서비스마다 다르다. 추가 filter는
URL의 한 path segment 안에 `name=value&name=value` 형태로 들어가므로 일반 query builder를 재사용할
수 없다.

### 공개 probe

공식 문서는 `http` sample을 게시하지만 2026-09-01에 공개 sample key로
`https://openapi.foodsafetykorea.go.kr/api/sample/C005/json/1/1`을 요청하자 HTTPS 200과 JSON을
반환했다. 당시 응답은 운영시간 제한을 나타내는 `ERROR-503`이어서 데이터 성공으로 보지는 않지만
HTTPS와 공식 path shape는 확인됐다. 같은 날 메인은 “Open API 제한적 운영” 공지를 노출하고 있었다.

### 최소 구현과 판단

path credential redaction, `serviceId` 고정, `json|xml`, `1 <= start <= end` 및 1,000건 제한 외에
각 서비스 상세의 filter/schema를 가져와 검증하는 provider inspector와 fixture cache가 필요하다.
임의 filter passthrough는 injection과 잘못된 호출을 만들 수 있다. **SafetyKorea와 credential 기반을
먼저 만든 뒤 inspector를 포함한 후속 릴리즈에서 구현한다.**

## VWorld

### 공식 계약과 HTTPS

[2D 데이터 API 2.0 공식 guide](https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=adsigg)는
`https://api.vworld.kr/req/data`, query `key`, 선택 `domain`을 문서화한다. `domain`은 브라우저 등에서
사용할 때 발급 시 입력한 URL을 보내도록 설명한다. [WMS/WFS 공식 guide](https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do)도
HTTPS `/req/wms`, `/req/wfs`, `key`, 등록 domain을 설명하며 key/domain 불일치 오류를 별도로 정의한다.
[인증키 발급 화면](https://www.vworld.kr/mypo/mypo_apiKey_i001.do)은 비로그인 상태에서 로그인 UI를
보이므로 계정이 필요하다. 공개 자료에서 IP 제한이나 승인 소요를 확인하지 못했다.

호출 family는 공통 schema가 아니다.

- data: `request`, `data`, `geomFilter`/`attrFilter`, paging, geometry/CRS
- WMS: OGC operation, layers/styles, bbox, width/height, CRS, image response
- WFS: OGC feature operation과 type/filter
- geocoder/search/static/WMTS: 서로 다른 path, 필수값, 결과 형태

### 공개 probe

2026-09-01에 key 없이 HTTPS `/req/data`의 문서화된 최소 shape를 요청하자 HTTP 200 JSON과
`PARAM_REQUIRED`(필수 `key` 누락)를 반환했다. 이는 공식 HTTPS endpoint와 query auth 처리가
동작한다는 확인이며 데이터 성공은 아니다.

### 최소 구현과 판단

`key`와 선택 `domain`의 query redaction, 발급 등록 domain 검증, exact `/req/{family}` scope,
family별 typed operation/parameter/response parser가 필요하다. 하나의 “VWorld raw params” invoker는
안전하지 않다. **2D data 또는 geocoder 한 family를 골라 별도 릴리즈에서 시작한다.**

## 서울 열린데이터광장

### 키와 두 credential scope

[공식 Open API 이용 가이드](https://data.seoul.go.kr/together/guide/useGuide.do)는 키 발급 후 API를
검색·활용하고 애플리케이션을 등록하는 순서를 안내한다. 일반 키와 실시간 지하철 키 신청을 구분하고,
실시간 지하철은 하루 최대 1,000건, 한 요청은 최대 1,000건이라고 명시한다. 비로그인 상태의
[일반 키 신청](https://data.seoul.go.kr/together/mypage/actkeyReq_ss.do)과
[지하철 키 신청](https://data.seoul.go.kr/together/mypage/actkeyMetroReq_ss.do)은 각각 로그인으로
연결된다. 공개 자료에서는 domain/IP 제한을 확인하지 못했다.

일반 API와 지하철 API는 키도 endpoint도 다르다.

```text
GET http://openapi.seoul.go.kr:8088/{KEY}/{TYPE}/{SERVICE}/{START_INDEX}/{END_INDEX}/...
GET http://swopenapi.seoul.go.kr/api/subway/{KEY}/{TYPE}/{SERVICE}/...
```

[일반 API 예시 OA-15442](https://data.seoul.go.kr/dataList/OA-15442/A/1/datasetView.do)는
`SearchSTNBySubwayLineInfo`, [실시간 예시 OA-12764](https://data.seoul.go.kr/dataList/OA-12764/A/1/datasetView.do)는
`realtimeStationArrival`을 게시한다. 공통 paging 뒤의 추가 path parameter와 response field는
서비스별이다.

### 공개 probe와 HTTPS 판단

2026-09-01 관측은 다음과 같다.

- 일반 공개 sample의 HTTP 요청은 200 JSON(`INFO-200`, 데이터 없음)을 반환했다.
- `https://openapi.seoul.go.kr/...` 443은 15초 내 연결되지 않았다.
- `https://openapi.seoul.go.kr:8088/...`는 TLS protocol alert로 실패했다.
- 실시간 지하철 공개 sample HTTP 요청은 200 JSON 안의 provider 오류 `ERROR-336`을 반환했다.
- `https://swopenapi.seoul.go.kr/...`는 15초 내 연결되지 않았다.

공식 사이트가 HTTPS 페이지에서 server-side sample proxy를 제공하거나 CSP
`upgrade-insecure-requests`를 사용해도, CLI가 실제 credential을 평문 HTTP path로 보내도 된다는
근거가 되지 않는다.

### 최소 구현과 판단

두 key를 별도 namespace/scope로 저장하고 OA dataset을 service name·추가 path schema에 연결하는
inspector가 필요하다. 그러나 그보다 앞선 차단 조건은 transport다. **OpenDataCTL 기본 invoker는
HTTP credential 전송을 허용하면 안 되므로 현재 일반·지하철 모두 no-go다.** 사용자가 명시적으로
허용하는 insecure mode도 agent가 비밀을 평문 전송하게 만들어 기본 제품의 신뢰 경계를 약화시키므로
권장하지 않는다. 공식 HTTPS가 확인된 뒤 다시 검토한다.

## 권장 릴리즈 순서

1. **v0.12**: 현재의 4개 contract adapter, adapter revision, live drift canary만 릴리즈한다.
2. **v0.13**: provider credential lifecycle과 SafetyKorea typed invoker 하나를 함께 출시한다.
3. **후속**: 식품안전나라 inspector + service schema fixture, 그 다음 VWorld family별 invoker를 추가한다.
4. **서울**: official invocation origin에서 HTTPS가 canary로 확인될 때까지 contract/신청 안내만 유지한다.

이 순서라면 v0.12가 LINK를 “호출 가능”으로 과장하지 않으면서도 검색→상세→정확한 신청 경로라는
현재 가치를 즉시 제공한다. 후속 invoker는 동일 adapter ID에 선택적으로 붙일 수 있으므로 지금
릴리즈한다고 구조를 되돌릴 필요도 없다.

## 확인하지 못한 사항

- 로그인 뒤에만 보이는 provider별 quota, 승인 기간, 등록 domain/IP 정책은 추측하지 않았다.
- 공개 sample의 200은 운영 credential로 E2E 성공했다는 뜻이 아니다.
- 2026-09-01의 서울 TLS 실패는 시점 관측이다. drift canary가 성공으로 바뀌면 공식 문서와 함께
  다시 검증해야 한다.
