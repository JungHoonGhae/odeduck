# LINK provider adapter guide

`data.go.kr`의 LINK 데이터셋은 포털 바깥의 서로 다른 API 계약으로 연결된다.
odeduck은 provider마다 MCP tool을 추가하지 않고
`catalog_search → inspect_dataset → call_api` 세 진입점 안에서 provider별 차이를 격리한다.
`describe_api`는 기존 API-only 클라이언트를 위한 호환 도구다.

## 어디까지 템플릿화되는가

**provider 단위 adapter는 충분히 정형화할 수 있지만, 모든 사이트를 한 raw caller로 만들 수는 없다.**

공통 프레임워크가 책임지는 것은 다음과 같다.

```text
portal LINK URL
  → exact provider matcher
  → evidence-backed contract
  → optional service-schema inspector
  → provider-scoped credential
  → typed operation/parameter validator
  → exact HTTPS request + redirect rejection
  → bounded response + provider error parser + secret redaction
```

provider adapter가 채우는 부분은 exact host/path/query, API family, operation, 필수값·enum,
인증 위치와 scope, provider 성공/오류 코드다. FoodSafetyKorea처럼 서비스마다 filter가 달라지는
경우에만 공식 상세 페이지를 읽는 inspector를 추가한다. 이 분리는 새 사이트의 차이를 억지로
하나의 규칙에 맞추지 않으면서도 보안·테스트·MCP 표면을 재사용하게 한다.

복사해서 시작할 파일:

- [matcher/contract template](templates/external_adapter.go.tmpl)
- [contract/fail-closed test template](templates/external_adapter_test.go.tmpl)
- [typed operation registry template](templates/external_invoker.go.tmpl)

## 현재 참조 구현

| Adapter | Revision | 호출 범위 | 인증 | Live canary |
|---|---:|---|---|---:|
| `safetykorea` | 2 | 인증·국내리콜·국외리콜 GET 5종 | `AuthKey` header | 1 |
| `vworld` | 3 | 고정 data-code 2D, address, search, WMS/WFS typed family | `key`와 선택 `domain` query | 5 |
| `foodsafetykorea` | 2 | `svc_no`별 list + 공식 요청인자 표의 동적 filter | `keyId` path | 1 |
| `seoul-open-data` | 2 | 일반·실시간 지하철 계약 분류; 호출은 HTTPS 부재로 차단 | 서로 다른 `KEY` path scope | 4 |

SafetyKorea, FoodSafetyKorea와 VWorld의 위 family는
`invocationState=implemented`다. 서울은 `blocked_insecure_transport`이고, VWorld의 일반 catalog
page와 등록하지 않은 provider는 `not_implemented` 또는 `inspection_required`로 멈춘다.

구현 근거는 [provider 조사](research/link-provider-adapters.md), 호출 안전성 판단은
[자동 호출 타당성 조사](research/link-provider-invocation-feasibility.md), 실제 전후 효과는
[LINK 품질 평가](research/link-search-quality-evaluation.md)에 있다.

## 두 개의 깊은 경계

### 1. Recognition adapter

[`externalProviderAdapter`](../internal/apicall/external_adapter.go)는 메서드가 하나다.

```go
type externalProviderAdapter interface {
    ContractFor(target *url.URL) (*ExternalContract, bool)
}
```

이 adapter는 네트워크를 사용하지 않는다. 포털이 반환한 URL을 exact origin, path, query
allowlist로 판정하고 공식 근거가 있을 때만 계약을 반환한다. registry가 `adapterId`와
`adapterRevision`을 붙인다.

### 2. Typed invocation

[`ExternalCaller`](../internal/apicall/external_call.go)는 raw URL이나 raw auth parameter를 받지 않는다.
`ExternalContract + operation + ordinary params + scoped credential`만 받아 provider별 exact HTTPS
request를 만든다. [`DatasetCaller`](../internal/apicall/dataset_caller.go)는 REST와 LINK를 한 번만
`describe`한 뒤 이 호출기로 dispatch하므로 CLI와 MCP가 같은 경계를 사용한다.

provider key는 별도 namespace에 저장한다.

```bash
odeduck provider-key set safetykorea
odeduck provider-key set foodsafetykorea
odeduck provider-key set vworld --domain https://example.com/map
odeduck provider-key status
odeduck provider-key delete vworld
```

비밀값은 command argument나 MCP payload로 받지 않는다. config directory의 제한된 하위에
provider별 원자적 파일로 저장하고(Unix `0600`; Windows 현재 사용자+SYSTEM 전용 protected
DACL) exact credential scope에서만 읽는다. `logout`은 현재·호환 config root의 이 파일과
중단된 원자적 쓰기 조각까지 지운다.

## adapter 추가 절차

1. provider의 공식 문서만 사용해 application URL, credential 위치·이름·scope, 문서 버전,
   실제 HTTPS transport와 provider 오류 코드를 확인한다.
2. template을 복사해 `ContractFor`를 구현한다. host suffix나 넓은 path prefix가 아니라 확인한
   exact shape만 허용한다. 호출기가 아직 없으면 `InvocationNotImplemented`로 먼저 등록한다.
3. [`externalProviderAdapters`](../internal/apicall/external_adapter.go)에 고유 ID, revision, 대표
   `publicDataPk`와 URL variant canary를 등록한다.
4. 정상 URL과 same-host wrong path, apex/subdomain spoof, userinfo, fragment, non-default port,
   encoded path, unknown·empty·repeated query 거부 fixture를 추가한다.
5. 자동 호출을 추가할 때는 raw endpoint passthrough가 아니라 typed family를
   [`external_call.go`](../internal/apicall/external_call.go)에 등록한다. 공개 `contract.operations`와
   request builder가 각자 다른 표를 갖지 않도록 provider 파일의 하나의 typed operation
   registry에서 parameter·required·enum·endpoint를 함께 파생한다. SafetyKorea와 VWorld가
   이 패턴의 참조 구현이다.
6. `InvocationImplemented`로 올릴 때는 [`providerauth`](../internal/providerauth/store.go)의
   provider ID·exact credential scope도 함께 등록한다. doctor가 adapter contract와 credential
   store의 scope 조합이 일치하지 않으면 drift로 거부한다. CLI help의 provider 목록은
   이 store inventory에서 자동 생성된다.
7. header/query/path credential 각각에 대해 redirect rejection과 raw/query/path-escaped redaction을
   public seam에서 테스트한다. response size와 timeout도 제한한다.
8. MCP JSON round-trip, `go test ./...`, `odeduck doctor --adapters-only`와 고정 자연어 검색
   시나리오를 통과시킨다.

## 언제 inspector가 필요한가

다음 조건이면 matcher에 endpoint 추측을 넣지 말고 inspector를 별도 구현한다.

- 같은 provider에서도 service ID마다 filter나 추가 path parameter가 다르다.
- 문서 화면이 실제 요청변수 표의 유일한 공식 source다.
- URL alone으로 credential subtype이나 API family를 판정할 수 없다.

inspector는 exact HTTPS documentation origin, service ID 일치, response size, redirect 정책을 모두
검사해야 한다. 검사에 성공한 schema는 adapter revision·service ID·문서 URL별로 짧게 캐시하되,
오류는 캐시하지 않고 개수와 TTL을 제한한다. 공식 schema를 읽지 못하면 호출하지 않는다.
FoodSafetyKorea가 이 형태의 참조 구현이다. JavaScript 실행이나 로그인된 browser가 필요한
provider는 일반 parser template의 범위를 벗어나므로 별도 ADR과 위협 모델이 필요하다.

## 버전과 유지보수

세 숫자는 의미가 다르다.

- odeduck SemVer: 공개 CLI/MCP 호환성과 릴리즈 버전
- `adapterRevision`: matcher, family, auth scope, operation 계약이 바뀔 때 올리는 정수
- `documentationVersion`: provider가 게시한 문서 버전(없으면 비워 둠)

`verifiedAt`은 공식 근거를 마지막으로 재확인한 날짜다. 180일이 지나거나 live canary의 portal
URL·adapter 해석이 달라지면 doctor가 exit 1을 반환한다.

```bash
odeduck doctor --adapters-only -f table
```

매주 [LINK adapter drift workflow](../.github/workflows/adapter-drift.yml)가 같은 검사를 실행한다.
실패하면 canonical GitHub issue를 열거나 기존 이슈에 결과를 추가한다. URL fixture만 바뀌고 계약은
동일하면 revision을 유지할 수 있지만 matcher·auth·scope·family·operation이 바뀌면 revision을
올리고 공식 근거와 fixture를 함께 갱신한다.

## 저장소 경계

현재 adapter들은 하나의 Go 바이너리, 계약 타입, credential subsystem, 테스트와 릴리즈 주기를
공유하므로 단일 module이 더 단순하다. 독립 배포, 다른 runtime, 외부에서 import하는 안정 SDK가
필요해질 때 `packages/adapter-sdk`와 `adapters/*`로 분리하는 것이 monorepo 전환 시점이다. provider별
revision은 이미 독립적으로 추적하되 바이너리 SemVer는 하나로 유지한다.
