# Architecture

odeduck은 대한민국 공공데이터를 **찾고 → 실제 계약을 확인하고 → 필요한 권한을 신청하고 → 첫 호출까지
검증하는** 로컬 컨트롤 플레인이다. CLI와 MCP가 같은 도메인 모듈을 사용하며, 모델은 검색 계획을
도울 수 있지만 카탈로그 순위·자격증명·API 호출의 최종 경계는 결정론적인 Go 코드가 맡는다.

## System at a glance

```text
사람 ───────────────→ Cobra CLI ───────────────┐
                         │ catalog discover    │
                         ▼                     │
                  installed agent CLI          │
                         │ concepts·axes only  │
                         └─────────────────────┤
                                               ▼
AI host ── stdio MCP → MCP server ───────→ local catalog
                                               │
                                               ▼
                                   unified dataset inspector
                                               │
                             ┌─────────────────┴─────────────────┐
                             ▼                                   ▼
                   API contract & caller              bounded FILE observer
                     │               │                          │
                     ▼               ▼                          ▼
           data.go.kr portal     reviewed LINK              data.go.kr
             & session            adapters
                     │               │
                     ▼               ▼
               data.go.kr       external provider APIs

local state: catalog + optional semantic index | portal session + key | provider-scoped keys
```

MCP의 주 경로는 의도적으로 작다.

```text
catalog_search → inspect_dataset → (미승인 REST만 apply) → call_api
                                                               ↓
                                             record_connection_assessment
```

`call_api`가 선택 필드를 profile하면 MCP 세션 메모리의 bounded receipt가 operation, delivery, request
hash와 집계를 묶는다. `record_connection_assessment`는 이 receipt와 두 dataset의 공식 출처·field
evidence를 다시 대조한 뒤에만 sample 판정을 저장한다. 저장 장부는 raw 응답 값과 credential 없이
append-only JSONL로 유지된다.

`search_datasets`와 `list_applications`는 각각 최신 포털 재확인과 계정 상태 확인을 위한 보조 도구다.
`describe_api`는 기존 클라이언트를 위한 `inspect_dataset`의 API 전용 호환 도구다.

## Request lifecycle

### 1. Discover

`catalog_search`와 `odeduck catalog search`는 릴리스에 포함되거나 `catalog sync`로 갱신된 로컬
`catalog.json`을 검색한다. 엄격한 어휘 검색부터 시작하고, 선택적 Ollama 인덱스가 있으면 로컬 의미 검색
결과를 합친다. 일반 검색은 인덱스가 없거나 오래됐거나 Ollama가 실패하면 어휘 검색으로 돌아가고
저하 상태를 `semantic.status`와 `warnings`에 남긴다. 고신뢰 조사에서 `--require-semantic` 또는 MCP
`requireSemantic=true`를 사용하면 의미 검색이 실제로 쓰이지 않은 경우 어휘 결과를 반환하지 않고 실패한다.

`catalog.Searcher`가 CLI·MCP 공통으로 인덱스 로드, 상태별 폴백, 인덱스 모델에 맞는 embedder 선택과
필수 semantic 판정을 소유한다. 실패 시 내부 진단 결과는 유지하지만, 두 adapter는 오류를 먼저 처리해
후보가 출력되지 않도록 한다. 일반 CLI 검색에서도 저하 경고를 stderr에 표시한다.

독립 CLI의 `catalog discover`는 설치되어 이미 로그인된 Codex·Claude·Gemini·Cursor 중 하나를 별도
프로세스로 실행한다. [`internal/agentplan`](internal/agentplan)은 자연어 목표를 작은 `QueryPlan`으로 바꾸고,
실제 카탈로그 순위와 네트워크 호출은 [`internal/catalog`](internal/catalog)이 소유한다. MCP에서는 이미
호스트 AI가 검색축을 만들 수 있으므로 중첩 모델을 실행하지 않는다.

[`internal/discovery`](internal/discovery)의 `Runner`가 독립 CLI의
Generate → Search → Expand → Search → Compose → Search 순서를 실행한다. 원래 Anchor 축,
provider, 검색 정책을 유지하며 Bridge 선택지와 단계별 폴백·Abstention을 관리한다. planner와 검색은
주입 가능한 seam이며, 진행 순서 테스트는 Cobra나 package 전역 함수 교체 없이 이 interface를 사용한다.

교차 도메인 발견도 검색 결과를 곧바로 관계라고 부르지 않는다. Anchor와 Bridge 역할별 후보를 제한된
수로 꺼낸 뒤, 실제 PK가 명시적으로 선택되고 역할·연결 계약을 통과한 조합만 `candidate` 카드가 된다.
필드, grain, 값 교집합을 검사하기 전에는 join이나 사업성을 검증했다고 표시하지 않는다.

### 1.5. Goal-driven composition (experimental)

`odeduck solve "목표"`와 MCP `advance_goal`은 [`internal/goalwork`](internal/goalwork)의 동일한
Start/Advance interface를 사용한다. `solve`만 tool-free Codex·Claude·Gemini를 호출하고 MCP에서는
host가 다음 행동을 제안한다. 고정 Anchor 없이 역할별 후보와 여러 Composition을 보존한다.
상세 행동 안내는 Goal Engine의 [정적 계획 계약](internal/goalwork/planning-guide.md) 하나를
CLI prompt와 MCP resource가 공유한다. 각 adapter는 전달·출력·신뢰된 시작 설정만 덧붙인다.

```text
goal → define(필수 역할·범위·출력) → search → inspect → sample → compose → execute
                                      ↑                                  │
                                      └── gap / 부분 산출물 / 대안 탐색 ───┤
                                                                         └→ evaluate → review_required
```

module이 revision·중복 억제·예산·실제 원천 획득·실행 결과를 소유한다. 모델 JSON은 행이나 성공
근거를 제출할 수 없다. API는 기존 DatasetCaller를, 직접 CSV는 검사된 Asset의 bounded reader를
재사용한다. STD는 검증된 first-party JSON 계약의 private handle로 첫 페이지를 읽는다. 인증키 입력과
자동 신청은 없다. XLSX/DBF/ZIP은 스키마 검사와 결합 실행을 구분한다.

실행은 복합키 exact/trim inner join, projection, groupBy+count/sum이며 tuple을 보존한다. null은
결합되지 않고 중복 expansion과 빈 전체 경로는 실패한다. 원문 행은 세션 메모리에만 두며 외부 CLI
planner에는 기본적으로 컬럼·타입·해시·오류만 보낸다. 명시한 단일 agent에 --share-evidence를 켜면
공통 read_evidence가 선택한 행·필드만 원본 주소와 함께 전달한다. MCP는 --share-goal-evidence로
서버 시작 시 상한을 정하고 모델 입력으로 바꾸지 못한다. 선택 근거는 셀·packet·세션 누적 예산과
만료를 적용하며 전체 Artifact는 외부 CLI 계획 입력에서 계속 제외한다. 기존 MCP Artifact/call_api
원문 반환은 별개다. [공개·재사용 신뢰 경계](docs/adr/0007-selected-evidence-and-reuse.md)를 따른다.
최종 artifact에는 제한된 행·recipe·요청·해시·관측시각·
join metrics가 들어간다. 표본 재현에 필요한 요청은 남지만 원천의 과거 bytes는 보관하지 않는다.

문자열 측정값은 원본과 별도의 Measure에 형식·단위를 선언해 변환한다. 결합 키는 바꾸지 않는다.
수치 합은 십진 정밀도를 보존하며, 같은 원천 행이 결합 과정에서 반복 기여한 합계는 거부한다.
MCP 출력도 숫자를 float64로 재해석하지 않고 직렬화한다. 클라이언트의 정확한 수치 복원 조건은 spec에 있다.

시간 비교는 모든 참여 원천의 실제 기간 필드를 바인딩하고 전체 경로의 공통 달력 기간을 계산한다.
목표에서 고정한 날짜 범위는 조합에서 제거할 수 없다. 날짜 누락·불일치를 통과시키지 않으며,
조회 시각과 원천의 유효 기간은 구분한다. 계산된 시간 검사와 미검증 필드 의미를 함께 반환한다.
한계 설명은 가상의 데이터 컬럼이 아니라 원천·실행 근거에서 만든 Evidence Explanation으로 제공한다.

Row-bound Scope Check는 후보 행의 실제 범위 필드들을 토큰 단위로 대조한다. 키가 같아도 설정한
범위 조건이 불일치하거나 값이 없으면 시간 검사·행 확장·집계 전에 제외하며, 집계된 검사 결과를
Discovery Gap과 함께 재탐색에 돌려준다. 문자열 규칙의 일치는 지리 해석·식별 관계의 승인이 아니다.
행별 상위 범위가 없으면 명시적인 Cited Scope Claim으로 관측 당시 원천 설명을 인용할 수 있다.
문구 위치·출처·관측/선언 해시를 보존하되 새 행 컬럼이나 식별자로 만들지 않는다. 인용은 가설이며,
그 문구가 긍정적·보편적·현재 유효한 범위를 뜻하는지까지 검증한 것은 아니다.

한 원천의 조회·집계와 여러 원천의 결합은 같은 실행 경로를 사용한다. `sample_executed`는 표본 실행 결과이지
namespace 동일성·인과·사용자 목표 효과의 검증이 아니다.
필수 역할·출력 타입·출처가 빠진 artifact는 partial로 남기고 탐색을 계속한다. 구조적 조건을 통과해도
`needsSemanticReview=true`이면 `review_required`로 멈추고 후보를 보존하며 CLI는 실패 코드를 반환한다.
현재 의미 검증은 미완성이므로 `output_ready`를 자동 발급하지 않는다. 모델의 가정 문구는 승인이 아니다.
기존 connection ledger의 `sample_verified` gate와 분리한다. bounded join에는 별도 graph DB가
필요하지 않다. 계약과 현재 한계는 [spec](docs/specs/goal-driven-composition-v1.md), 결정은
[ADR-0006](docs/adr/0006-goal-driven-composition.md)에 있다.

### 2. Inspect

[`internal/dataset`](internal/dataset)은 PK 하나를 기준으로 제공 형태를 나눈다.

| 제공 형태 | 검사 결과 | 다음 단계 |
| --- | --- | --- |
| `REST` | 포털이 게시한 operation, endpoint, 필수 파라미터, 심의 유형 | 필요하면 `apply`, 이후 `call_api` |
| `LINK` | 공식 시작점과 검토된 provider adapter 계약 | 구현된 typed adapter만 `call_api` |
| `FILE` | 실제 다운로드 자산, 기간, 수정일 | bounded 표본에서 CSV/DBF/XLSX worksheet 컬럼과 SHA-256 관찰 |
| `STD` | 공식 포털의 PK·표·컬럼 계약, 표시명, 선언 건수 | private handle로 bounded 첫 JSON 페이지 관찰·조합; 레코드 날짜와 모집단 범위는 별도 검증 |

API와 FILE을 함께 제공하는 항목은 두 계약을 모두 보존한다. 알 수 없는 LINK를 임의의 API endpoint로
해석하거나, FILE을 API처럼 호출하지 않는다.

### 2.5. Apply

[`internal/portal`](internal/portal)은 data.go.kr 로그인 세션을 재사용해 실제 활용신청 폼을 제출하고 신청
상태와 인증키를 읽는다. `apply`는 data.go.kr의 `REST`에만 허용된다. CLI는 제출 전 `y/N`을 묻고,
MCP 도구는 외부 상태를 바꾸는 destructive action으로 표시해 호스트의 승인 정책을 따른다.

### 3. Call and verify

[`internal/apicall`](internal/apicall)은 입력받은 PK로 공식 계약을 다시 확인하고 필수 파라미터를 검증한 뒤
자격증명을 주입한다. data.go.kr REST와 구현된 LINK adapter가 같은 `call_api` 표면을 쓰지만, provider별
호스트·path·credential scope는 분리된다. 방금 자동 승인된 API는 gateway 반영을 기다리며 제한된 시간
동안 재시도할 수 있다. XML은 JSON으로 정규화하고, 선택한 필드의 count·distinct·null·중복·값 표본을
계산해 실제 연결 가능성을 확인한다.

## Module boundaries

| 경로 | 책임 | 맡지 않는 것 |
| --- | --- | --- |
| [`cmd/odeduck`](cmd/odeduck) | Cobra 명령, 플래그, 출력 연결, dependency composition | 검색·신청·호출 규칙 |
| [`internal/mcpserver`](internal/mcpserver) | stdio MCP 도구·리소스, 입력 제한, tool annotation | 별도 비즈니스 로직 |
| [`internal/catalog`](internal/catalog) | snapshot 동기화, 어휘·hybrid 검색, bounded connection 후보 | 모델 실행, 자격증명, API 호출 |
| [`internal/discovery`](internal/discovery) | 독립 CLI의 계획·검색·선택 순서, 폴백·Abstention | ranking, 연결 검증, MCP host 계획 대체 |
| [`internal/goalwork`](internal/goalwork) | 불변 목표·관측·예산·실행·평가·선택 근거와 공통 행동 안내 | 자동 의미 승인, 자격증명 소유, 전역 entity merge |
| [`internal/agentplan`](internal/agentplan) | 자연어 목표를 검색축으로 변환하고 실제 후보 중 Bridge PK 선택 | 카탈로그 ranking, 신청, 호출 |
| [`internal/dataset`](internal/dataset) | REST·LINK·FILE 통합 검사, bounded FILE schema 관찰 | FILE을 호출 가능한 API로 추측 |
| [`internal/apicall`](internal/apicall) | 공식 API 계약 해석, REST/LINK dispatch, 검증·호출·profiling | 브라우저 로그인 UI |
| [`internal/portal`](internal/portal) | data.go.kr 공개 페이지, 로그인 세션, 신청·계정·키 흐름 | 외부 provider credential 재사용 |
| [`internal/providerauth`](internal/providerauth) | 고정 provider와 HTTPS scope별 자격증명 저장 | 키 목록이나 값을 MCP에 노출 |
| [`internal/connectionledger`](internal/connectionledger) | 연결 assessment의 provenance·집계·시간을 검증하고 append-only JSONL에 저장 | raw 응답 값, credential, 실세계 entity 병합 |
| [`internal/fetch`](internal/fetch) | 공통 throttle, timeout, response bound, data.go.kr TLS 정책 | 임의 LINK를 안전하다고 판정 |
| [`internal/doctor`](internal/doctor) | 포털 markup과 provider adapter drift 점검 | 자동 계약 수정 |
| [`internal/output`](internal/output) | JSON·JSONL·table 렌더링 | 도메인 결과 생성 |

의존 방향은 transport에서 도메인 모듈로 흐른다. `catalog`은 저장·검색을, `portal`은 data.go.kr 연동을
소유하고 `dataset`과 `apicall`이 사용 사례 단위로 조합한다. CLI와 MCP에 같은 규칙을 두 번 구현하지
않는다.

## State and trust boundaries

기본 로컬 상태는 운영체제의 사용자 설정 디렉터리 아래 `odeduck`에 저장된다.

| 상태 | 저장 위치 | 성격 |
| --- | --- | --- |
| `catalog.json` | config root | 공개 카탈로그 snapshot, atomic replace |
| `catalog-semantic.gob` | config root | 선택적 로컬 embedding index, snapshot digest로 호환성 확인 |
| `datagokr-session.json` | config root | 로그인 cookie; 프로세스 간 lock으로 회전 갱신 직렬화 |
| `datagokr-apikey` | config root | data.go.kr API key cache |
| `config.json` | config root | 자동신청 같은 사용자 설정 |
| `provider-credentials/*.json` | config root | adapter ID와 exact scope별 외부 provider key, atomic write |
| `connection-evidence.jsonl` | config root | 검증·차단·기각된 연결 assessment와 해시·집계, append-only |

Unix에서는 민감 파일을 사용자 전용 권한으로 저장한다. 외부 provider credential은 Windows에서도 현재
사용자와 SYSTEM만 허용하는 보호된 DACL을 적용한다. 파일은 암호화되지 않으므로 공용 머신은 신뢰 경계
밖이다. `odeduck logout`은 현재 설정 루트에 남은 세션, 키, 브라우저 profile까지 정리한다.

중요한 경계는 다음과 같다.

- MCP의 `call_api`는 raw endpoint나 key를 입력받지 않는다. PK와 typed parameter만 받는다.
- 일반 LINK는 publisher가 준 untrusted handoff다. 검토된 adapter가 없으면 호출하지 않는다.
- credential을 쓰는 외부 호출은 고정된 HTTPS host·path·scope에만 보내고 redirect를 따르지 않는다.
- data.go.kr 공개 HTTP는 공통 throttle과 timeout을 거치며, 일반 응답은 크기가 제한된다.
- `catalog discover`는 목표를 선택된 모델 provider에 보내지만 odeduck이 provider 로그인 토큰을 읽지는
  않는다. 후속 metadata 선택은 tool-free 격리를 지원하는 provider에서만 수행한다.
- 원격 상태를 바꾸는 핵심 동작은 활용신청이다. 검색·검사·호출은 읽기 경계에 머문다.

## Failure and drift behavior

odeduck은 지원 범위를 넓히는 것보다 실패를 명시하는 쪽을 택한다.

- 로컬 snapshot이 오래되면 `stale`을 반환하고, 최신 항목은 보조 live search로 재확인한다.
- 의미 인덱스가 없거나 snapshot과 맞지 않으면 어휘 검색은 계속 동작한다.
- 포털 HTML이나 provider 문서가 바뀌면 parser가 조용히 추측하지 않고 오류 또는 미지원 상태를 낸다.
- `doctor`와 주간 provider canary가 포털·adapter drift를 검사한다.
- 카탈로그와 의미 인덱스는 검증 후 교체하며, provider credential은 임시 파일에서 atomic replace한다.
- data.go.kr 세션 회전은 프로세스 내부 slot과 OS별 file lock으로 직렬화한다.
- 네트워크 응답과 FILE 관찰은 크기·형식·archive shape를 제한한다.

## Distribution

[`.goreleaser.yaml`](.goreleaser.yaml)은 `odeduck`을 macOS·Linux·Windows의 amd64/arm64로 빌드한다.

릴리스 checksum에는 각 archive뿐 아니라 `install.sh`, `install.ps1`, 검증된 catalog snapshot도 포함된다.
설치기는 같은 버전의 release asset과 checksum을 사용한다. PR CI는 모듈 상태, 브랜드 동기화, 설치기,
정적 분석, 취약점 검사, 전체 테스트와 build를 검증하고 Windows job이 PowerShell 설치 경로를 별도로
확인한다.

## Design decisions

- [ADR 0001: API-first discovery와 결정론적 HTML fallback](docs/adr/0001-html-scraping-over-api-discovery.md)
- [ADR 0002: 모델 계획 + 선택적 로컬 hybrid retrieval](docs/adr/0002-model-planned-hybrid-catalog-search.md)
- [ADR 0003: LINK를 외부 provider handoff로 취급](docs/adr/0003-link-as-external-provider-handoff.md)
- [ADR 0004: API + FILE 탐색과 bounded composition](docs/adr/0004-broad-catalog-bounded-composition.md)
- [ADR 0005: 증분 semantic index와 immutable snapshot](docs/adr/0005-incremental-semantic-index-distribution.md)

새 provider adapter를 추가할 때는 [provider adapter guide](docs/provider-adapters.md)를 따른다. 사용자에게
보이는 검색·연결 계약은 [cross-domain connection discovery spec](docs/specs/cross-domain-connection-discovery-v1.md)이
기준이다.

## Verify a change

```sh
go mod tidy -diff
go run ./scripts/sync-brand.go --check
go vet ./...
go test ./...
go build ./...
./scripts/test-install.sh
```

Windows installer 변경은 PowerShell에서 `./scripts/test-install.ps1`도 실행한다. 실계정 포털과 외부
provider 점검은 일반 PR 테스트와 분리된 `odeduck doctor` 및 scheduled canary가 맡는다.
