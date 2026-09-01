# LINK 상위 provider 참조 어댑터 조사

조사일: 2026-09-01
범위: VWorld, 식품안전나라, 서울 열린데이터광장, 제품안전정보센터(SafetyKorea)
근거: 각 provider와 data.go.kr의 공식 페이지·공식 API 응답만 사용했다.

> **후속 구현 결정 (2026-09-01):** 이 문서의 “호출 보류”는 matcher 조사 당시 판단이다.
> provider-scoped credential과 typed caller 안전장치를 구현한 뒤 SafetyKorea, FoodSafetyKorea,
> VWorld의 검증된 family는 adapter revision 2에서 `implemented`로 승격했고, revision 3에서
> 2D service-to-data binding과 operation 가용성을 fail-closed로 고정했다. 서울은 공식 HTTPS가
> 없어 `blocked_insecure_transport`다. 현재 구조와 기여 절차는
> [provider adapter guide](../provider-adapters.md)를 본다.

## 결론

네 provider는 모두 `data.go.kr`의 LINK 한 종류로 보이지만 계약은 서로 다르다.

| Provider | 확인된 LINK 문서 형태 | 외부 인증 | 호출 구현 판단 |
|---|---|---|---|
| VWorld | 여러 버전별 API guide와 국가중점데이터 검색 결과 | query의 `key`; 일부 호출은 발급 시 등록한 `domain`도 query로 전달 | provider 계약 어댑터는 가능. API family별 endpoint·필수 인자가 달라 범용 호출은 보류 |
| 식품안전나라 | `svc_no`가 들어간 서비스별 상세 | URL path의 `keyId`; 식품안전나라 전용 키 | HTTPS 호출 형식은 규칙적이지만 서비스별 필터·응답 계약 확인이 필요하므로 호출은 보류 |
| 서울 열린데이터광장 | `OA-...` 데이터셋 상세의 구형 query형·신형 path형 | URL path의 `KEY`; 일반 API 키와 실시간 지하철 키가 분리 | 문서화된 호출 endpoint가 현재 HTTP이므로 자격증명 호출은 보류 |
| SafetyKorea | 하나의 Open API hub | header의 `AuthKey`; 신청서 심의 후 별도 발급 | v2.0의 endpoint가 유한해 구현 가능. 별도 키 저장·host/path 제한 전까지 호출은 보류 |

즉, 조사 단계의 `contract_known`은 “어떤 provider의 어떤 공식 신청·인증 계약인지 안다”는 뜻이며
그 자체가 `callable`을 뜻하지 않았다. 후속 구현에서는 별도의 typed invocation 검증을 통과한
family만 `implemented`로 승격했다.

## 조사 방법과 실제 LINK 표본

2026-08-31에 동기화한 공식 data.go.kr 카탈로그(11,902개)에서 LINK를 활용신청 수로 내림차순 정렬하고 상위 50개의 공식 `selectApiLinkUrl` 응답을 2026-09-01에 다시 해석했다. VWorld 16개, 식품안전나라 6개, 서울 열린데이터광장 6개로 세 provider가 28/50(56%)이었다. 이 집계는 전체 LINK의 시장점유율이 아니라 **상위 수요 표본의 구현 우선순위 근거**다.

대표 공식 resolution 응답:

- VWorld 연속지적도: [data.go.kr LINK resolution](https://www.data.go.kr/tcs/dss/selectApiLinkUrl.do?publicDataPk=15056910), [공식 상세](https://www.data.go.kr/data/15056910/openapi.do)
- 식품안전나라 레시피 DB: [data.go.kr LINK resolution](https://www.data.go.kr/tcs/dss/selectApiLinkUrl.do?publicDataPk=15060073), [공식 상세](https://www.data.go.kr/data/15060073/openapi.do)
- 서울 지하철 실시간 도착정보: [data.go.kr LINK resolution](https://www.data.go.kr/tcs/dss/selectApiLinkUrl.do?publicDataPk=15058052), [공식 상세](https://www.data.go.kr/data/15058052/openapi.do)
- SafetyKorea Open API: [data.go.kr LINK resolution](https://www.data.go.kr/tcs/dss/selectApiLinkUrl.do?publicDataPk=15116894), [공식 상세](https://www.data.go.kr/data/15116894/openapi.do)

일부 data.go.kr LINK는 아직 `http`다. VWorld·식품안전나라·서울 공식 문서 host는 확인한 표본에서 동일 path/query의 HTTPS 주소로 301/302 전환했다. 어댑터가 이 레거시 링크를 provider 식별에 사용할 수는 있지만, 원문 `handoff.url`을 숨기거나 임의의 외부 URL을 따라가면 안 된다. contract에 넣는 문서·신청 주소만 고정 HTTPS 상수로 반환하고, 실제 fetch는 별도의 safe fetcher가 담당해야 한다.

## 공통 matcher 및 credential 경계

모든 matcher에 다음 제약을 공통 적용해야 한다.

1. URL을 표준 parser로 한 번만 해석하고 IDNA·host 대소문자·말단 점을 정규화한다.
2. userinfo, fragment, 비기본 port, encoded slash/backslash, dot segment, 중복 query value를 거부한다.
3. `*.example.go.kr` 같은 suffix match 대신 아래에 열거한 정확한 공식 origin만 허용한다. 같은 기관의 apex host가 존재해도 공식 path를 보존하지 않거나 404를 반환하면 허용하지 않는다.
4. path는 문자열 prefix가 아니라 정확한 path 또는 고정 regex로 맞춘다. query는 allowlist에 있는 key가 각각 정확히 한 번, 필요한 값은 비어 있지 않을 때만 받는다.
5. data.go.kr 키를 외부 host에 보내지 않는다. 외부 키는 provider별 namespace로 저장하고, credential scope는 실제 API origin과 path prefix까지 제한한다.
6. redirect에 credential을 자동 승계하지 않는다. DNS 해석 결과와 매 redirect hop을 다시 검사하는 safe fetcher 밖에서는 provider URL을 fetch하지 않는다.
7. 한 matcher가 성공하면 반환된 `adapterId`, `adapterRevision`, 신청 URL, auth 위치는 코드 상수 또는 검증된 service ID로만 구성한다. publisher query 값을 임의 URL에 연결하지 않는다.

## VWorld

### 확인된 공식 범위

상위 표본에서 확인한 canonical documentation origin은 정확히 `https://www.vworld.kr`다. apex `https://vworld.kr`는 같은 guide path를 유지하지 않고 메인으로 이동하므로 matcher origin으로 사용하지 않는 편이 안전하다.

확인된 path shape:

- `/dev/v4dv_2ddataguide2_s002.do?svcIde={service-id}` — [2D 데이터 API 2.0 시군구 예시](https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=adsigg)
- `/dev/v4dv_wmsguide2_s001.do` — [WMS/WFS API 2.0](https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do)
- `/dev/v4dv_geocoderguide2_s001.do` — [Geocoder API 2.0](https://www.vworld.kr/dev/v4dv_geocoderguide2_s001.do)
- `/dev/v4dv_search2_s001.do` — [검색 API 2.0](https://www.vworld.kr/dev/v4dv_search2_s001.do)
- `/dtna/dtna_apiSvcList_s001.do?searchKeyword={keyword}` — [국가중점데이터 API 검색 예시](https://www.vworld.kr/dtna/dtna_apiSvcList_s001.do?searchKeyword=%EA%B0%9C%EB%B3%84%EA%B3%B5%EC%8B%9C%EC%A7%80%EA%B0%80)

공식 동적 메뉴가 추가로 열거하는 버전별 guide는 [API 레퍼런스 허브](https://www.vworld.kr/dev/v4apiRefer.do)에서 확인할 수 있다. 다만 “VWorld의 `/dev/` 전체”를 match하면 안 된다. 새 path는 공식 레퍼런스에 존재하고 계약 fixture가 추가될 때 하나씩 allowlist에 넣어야 한다.

### 신청·인증·호출

- 공식 레퍼런스의 “인증키 발급하기” 동작은 로그인 전용 [인증키 발급 화면](https://www.vworld.kr/mypo/mypo_apiKey_i001.do)으로 연결된다.
- 2D 데이터 API 2.0은 `https://api.vworld.kr/req/data?...&key={인증키}`를 문서화하며, 발급할 때 등록한 URL을 `domain` query로 요구하는 사용 형태도 설명한다. WMS/WFS 2.0도 `https://api.vworld.kr/req/wms`와 `/req/wfs`, query의 `key`·`domain`을 문서화한다. [2D 데이터 예시](https://www.vworld.kr/dev/v4dv_2ddataguide2_s002.do?svcIde=adsigg), [WMS/WFS 예시](https://www.vworld.kr/dev/v4dv_wmsguide2_s001.do)
- 따라서 credential scope는 단순 `vworld.kr` 전체가 아니라 `https://api.vworld.kr/req/`로 제한해야 한다. `key`와 `domain`은 로그·오류·cache key에서 가려야 한다.
- 계정·발급 상태가 필요한 provider 전용 키이므로 `provider_account_required`가 맞다. 공개 페이지에서 자동승인 여부를 확정할 근거는 찾지 못했다.

### matcher 제약과 구현 판단

- host는 `www.vworld.kr`만, port는 HTTPS 443 또는 관측된 레거시 HTTP 80만 허용한다.
- 2D 상세는 정확한 path와 단일 `svcIde`만 허용하고 service ID는 비어 있지 않은 제한된 ASCII identifier로 검증한다.
- WMS·Geocoder·검색 등 static guide는 query가 없어야 한다.
- 국가중점데이터 검색은 정확한 path와 단일 `searchKeyword`만 허용한다. 이는 service-specific spec이 아니라 공식 검색 결과임을 유지해야 한다.
- API family마다 request·response가 달라 provider 공통 invoker를 지금 추가하지 않는다. 먼저 contract adapter와 family별 fixture를 제공한다.

2D data family는 `svcIde` 문자열만 맞는다고 호출 가능하다고 보지 않는다. 상위 수요 표본의
공식 guide에서 다음 고정 `data` 값을 다시 확인해 registry에 묶었다. 호출자는 이 값을 선택하거나
바꿀 수 없고, registry 밖의 `svcIde`는 `not_implemented`다.

| `svcIde` | 자동 주입 `data` |
|---|---|
| `cadastral` | `LP_PA_CBND_BUBUN` |
| `adsigg` | `LT_C_ADSIGG_INFO` |
| `utiscctv` | `LT_P_UTISCCTV` |
| `lhblpn` | `LT_C_LHBLPN` |
| `ademd` | `LT_C_ADEMD_INFO` |
| `adsido` | `LT_C_ADSIDO_INFO` |
| `upisuq151` | `LT_C_UPISUQ151` |
| `sgisgolf` | `LT_P_SGISGOLF` |
| `spbd` | `LT_C_SPBD` |

현재 guide에서 `GetFeatureType`은 서비스 예정으로 표시되므로 typed operation에는 `GetFeature`만
노출한다.

문서 버전은 path와 화면에 표시된 `2.0`을 사용한다. 예시 2D 데이터 “시군구” 페이지는 갱신일 `2026-08-18`을 표시했고, 계약은 2026-09-01에 다시 확인했다.

## 식품안전나라 (FoodSafetyKorea)

### 확인된 공식 범위

canonical documentation origin은 `https://www.foodsafetykorea.go.kr`다. apex는 동일 path의 `www` origin으로 301 전환하지만 실제 data.go.kr 표본은 `www`였으므로 matcher는 `www`만 허용하는 편이 명확하다.

상위 표본은 모두 다음 shape였다.

```text
/api/openApiInfo.do?...&svc_no={service-id}[&svc_type_cd=...]
```

[바코드연계제품정보 `C005`](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C005), [건강기능식품 원료인정 `I-0040`](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?menu_grp=MENU_GRP31&menu_no=656&show_cnt=10&start_idx=1&svc_no=I-0040&svc_type_cd=API_TYPE06) 같은 서비스 상세가 같은 호출 골격을 게시한다.

### 신청·인증·호출

- [공식 데이터활용서비스 메인](https://www.foodsafetykorea.go.kr/api/main.do)은 인증키가 하나만 발급되고 타인에게 양도할 수 없다고 안내한다. “인증키 신청” 동작은 로그인 전용 [신청 화면](https://www.foodsafetykorea.go.kr/api/newUserApiKey.do?menu_grp=MENU_GRP32&menu_no=691)으로 연결된다.
- 호출 shape는 `https://openapi.foodsafetykorea.go.kr/api/{keyId}/{serviceId}/{dataType}/{startIdx}/{endIdx}[/{filters}]`다. 공식 상세는 `keyId`, `serviceId`, `dataType`, `startIdx`, `endIdx`를 필수 path 변수로 설명한다. 현재 페이지 예시는 여전히 `http`로 인쇄되지만 동일 공식 sample path의 HTTPS 응답도 2026-09-01에 확인했다. [C005 공식 명세와 sample](https://www.foodsafetykorea.go.kr/api/openApiInfo.do?svc_no=C005)
- credential scope는 `https://openapi.foodsafetykorea.go.kr/api/`이고 위치·이름은 path의 `keyId`다. path에 들어간 키가 access log와 오류에 남지 않도록 완전 redaction해야 한다.
- provider 계정에서 별도 키를 발급받으므로 `provider_account_required`가 맞다.

### matcher 제약과 구현 판단

- 정확한 host `www.foodsafetykorea.go.kr`, path `/api/openApiInfo.do`만 허용한다.
- `svc_no`는 정확히 하나 필요하고 `[A-Za-z0-9-]+` 형태만 허용한다.
- 관측한 navigation query인 `menu_grp`, `menu_no`, `show_cnt`, `start_idx`, `svc_type_cd`만 추가 허용한다. 알 수 없는 query·중복 값·fragment는 거부한다.
- contract의 `providerServiceId`는 검증한 `svc_no`를 반환한다. documentation URL은 가능하면 해당 target의 canonical HTTPS URL을 유지하고, application URL은 별도 신청 화면으로 둔다.
- 공통 path 골격은 규칙적이지만 filter와 schema는 서비스별이다. 별도 FoodSafetyKorea 키 저장, HTTPS 고정, 서비스 상세 parser와 fixture를 추가하기 전에는 호출하지 않는다.

서비스마다 “최종수정일”이 다르다. 예를 들어 `C005`는 `2020-08-13`을 표시한다. provider-wide 문서 버전은 없으므로 contract에는 버전을 추측해 넣지 말고 `verifiedAt=2026-09-01`과 service ID를 기록한다.

## 서울 열린데이터광장

### 확인된 공식 범위

canonical documentation origin은 정확히 `https://data.seoul.go.kr`다. 상위 표본에는 두 URL 세대가 함께 있었다.

```text
/dataList/datasetView.do?infId=OA-{digits}&srvType={A|S}&serviceKind=1[&currentPageNo={n}]
/dataList/OA-{digits}/{A|S}/1/datasetView.do
```

대표 문서: [실시간 지하철 도착 OA-12764](https://data.seoul.go.kr/dataList/OA-12764/A/1/datasetView.do), [노선별 지하철역 OA-15442](https://data.seoul.go.kr/dataList/OA-15442/A/1/datasetView.do), [역별 요일별 환승인원 OA-12033](https://data.seoul.go.kr/dataList/OA-12033/A/1/datasetView.do).

### 신청·인증·호출

- [Open API 공식 이용 가이드](https://data.seoul.go.kr/together/guide/useGuide.do)는 키 발급 후 API 검색·활용·애플리케이션 등록 순서를 설명한다. 신청 landing은 [인증키 신청](https://data.seoul.go.kr/together/mypage/actkeyMain.do)이다.
- 일반 데이터는 로그인 전용 [일반 인증키 신청](https://data.seoul.go.kr/together/mypage/actkeyReq_ss.do), 실시간 지하철은 별도 [지하철 인증키 신청](https://data.seoul.go.kr/together/mypage/actkeyMetroReq_ss.do)을 사용한다.
- 일반 API의 공식 shape는 `http://openapi.seoul.go.kr:8088/{KEY}/{TYPE}/{SERVICE}/{START_INDEX}/{END_INDEX}/...`다. [OA-15442](https://data.seoul.go.kr/dataList/OA-15442/A/1/datasetView.do)는 `SearchSTNBySubwayLineInfo`를 이 origin으로 안내한다.
- 실시간 지하철의 공식 shape는 `http://swopenapi.seoul.go.kr/api/subway/{KEY}/{TYPE}/{SERVICE}/...`다. [OA-12764](https://data.seoul.go.kr/dataList/OA-12764/A/1/datasetView.do)는 `realtimeStationArrival`, [OA-12601](https://data.seoul.go.kr/dataList/OA-12601/A/1/datasetView.do)은 `realtimePosition`을 이 origin으로 안내한다.
- 두 종류 모두 키 이름은 `KEY`, 위치는 URL path다. 하지만 credential scope와 신청 키가 다르므로 dataset detail URL shape만 보고 하나의 `openapi.seoul.go.kr` scope를 반환하면 실시간 지하철에서 틀린다.
- 2026-09-01 확인 결과 공식 호출 명세는 HTTP를 사용했고, `openapi.seoul.go.kr`의 HTTPS 443은 응답하지 않았으며 8088 TLS도 성립하지 않았다. 자격증명을 평문 path로 보내는 invoker는 기본 구현에 넣지 않는다.

### matcher 제약과 구현 판단

- exact host `data.seoul.go.kr`, 위 두 path shape만 허용한다. 신형 path의 service kind는 관측·문서화된 `/1/`로 고정한다.
- 구형 query는 `infId`, `srvType`, `serviceKind`, `currentPageNo`만 허용한다. `infId`는 `^OA-[0-9]+$`, `srvType`은 `A|S`, 존재하는 `serviceKind`는 `1`, `currentPageNo`는 양의 정수여야 한다.
- provider 식별 결과는 `providerServiceId=OA-...`, 일반 [이용 가이드](https://data.seoul.go.kr/together/guide/useGuide.do), 신청 landing까지만 확정한다.
- OA ID만으로 일반·지하철 credential scope를 추측하지 않는다. 호출 단계는 공식 dataset API fragment를 safe fetcher로 읽어 endpoint origin과 신청 종류를 분류하거나, 검증된 OA fixture registry를 별도로 가져야 한다.
- transport가 HTTPS로 공식 지원되기 전에는 두 subtype 모두 `invocationState=not_implemented`로 둔다.

서울 페이지에는 provider-wide 명세 버전이 없다. 예시 metadata 수정일은 OA-12764 `2025-09-15`, OA-15442 `2025-04-01`, OA-12033 `2026-07-24`로 서로 다르므로 service ID와 `verifiedAt=2026-09-01`을 사용하고 버전을 추측하지 않는다.

## 제품안전정보센터 (SafetyKorea)

### 확인된 공식 범위

data.go.kr의 공식 target과 실제 hub는 정확히 다음 하나다.

```text
https://www.safetykorea.kr/release/openapi
```

[공식 hub](https://www.safetykorea.kr/release/openapi) 외의 path, query, fragment는 match하지 않는다. `https://safetykorea.kr/release/openapi`는 2026-09-01에 404였으므로 apex host도 허용하지 않는 것이 정확하다.

### 신청·인증·호출

- [Open API 사용방법·신청](https://www.safetykorea.kr/release/openapi2)은 신청서를 작성해 제품안전정보센터에 보내고, 승인 후 인증키를 이메일로 발급받는 절차를 게시한다. 따라서 `manual_approval`이다.
- 공식 [Open API 인터페이스 설계서 v2.0](https://www.safetykorea.kr/resources/fileData/openapi/Open_API_%EC%82%AC%EC%9A%A9%EC%84%A4%EB%AA%85%EC%84%9C_v2.0.pdf)은 2025-06-30 버전이며 KC 인증, 국내 리콜, 국외 리콜을 제공한다.
- 문서는 발급받은 서비스 ID를 HTTP header `AuthKey`에 넣고 대소문자를 구분하라고 명시한다. endpoint는 `/openapi/api/cert/`, `/openapi/api/recall/` 아래의 GET JSON/XML API다.
- 문서의 URL 예시는 HTTP지만 같은 공식 endpoint의 HTTPS 응답을 2026-09-01에 확인했다. invoker를 추가할 때 credential scope는 `https://www.safetykorea.kr/openapi/api/`로 고정하고 redirect에 `AuthKey`를 넘기지 않아야 한다.

### matcher 제약과 구현 판단

- `https`, exact `www.safetykorea.kr`, default 443, exact `/release/openapi`, query·fragment 없음만 허용한다.
- documentation version은 `2.0 (2025-06-30)`, application URL은 `/release/openapi2`, auth는 `header/AuthKey`로 반환한다.
- 호출 계약 자체는 네 provider 중 가장 좁고 명확하다. 다만 수동 발급한 별도 key의 안전한 설정 namespace와 endpoint별 parameter validation이 없으므로 현재는 호출하지 않는다.

## 권장 참조 어댑터 구조

외부 기여자가 참고할 seam은 작게 유지한다.

```text
portal LINK URL
  -> exact provider matcher
  -> evidence-backed contract
  -> optional provider-specific inspector
  -> optional scoped invoker
```

- `matcher`: URL만 보고 provider와 service ID를 판정한다. 네트워크를 사용하지 않는다.
- `contract`: 고정 HTTPS 문서·신청 URL, auth 위치·이름·scope, 문서 버전 또는 확인일을 반환한다.
- `inspector`: 필요한 provider만 safe fetcher를 통해 dataset-specific endpoint/schema를 해석한다. 서울처럼 subtype을 URL alone으로 판정할 수 없을 때 사용한다.
- `invoker`: 별도 interface로 두고 구현되지 않은 provider가 억지 no-op method를 갖지 않게 한다. scoped credential과 exact endpoint validator가 모두 있을 때만 등록한다.
- `canary`: 각 adapter마다 대표 `publicDataPk`, 예상 LINK URL, adapter ID를 고정하고 portal drift와 provider drift를 분리해서 진단한다.

각 신규 matcher fixture는 최소한 정상 URL, same-host wrong path, subdomain spoof, apex mismatch, HTTP/HTTPS 정책, non-default port, userinfo, fragment, encoded slash, unknown/repeated/empty query를 포함해야 한다. contract fixture는 문서·신청 URL, service ID, auth 위치·이름·scope, `invocationState`, `verifiedAt`까지 비교해야 한다.

## 한계와 재검증 조건

- 상위 50개 집계는 수요가 큰 LINK 우선순위를 보여주는 표본일 뿐 전체 4,766개 LINK 분포를 대표하지 않는다.
- portal LINK와 provider UI는 예고 없이 바뀔 수 있다. `verifiedAt` 이후 180일 또는 canary drift 발생 시 공식 문서와 신청 흐름을 다시 확인한다.
- 로그인 뒤에만 보이는 발급 조건·quota·승인 정책은 공개 화면에서 확정하지 않았다. 확인하지 못한 자동승인·호출량을 추측하지 않는다.
- sample endpoint의 200 응답은 transport·shape 확인에만 사용했다. 실제 credential을 사용하지 않았고 운영 quota·전체 schema의 E2E 성공을 주장하지 않는다.
