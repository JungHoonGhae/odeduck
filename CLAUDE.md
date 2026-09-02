# CLAUDE.md — oddsock

data.go.kr(공공데이터포털)의 OpenAPI **활용신청·인증키 발급·호출을 AI 에이전트가 대신**하게 하는
Go CLI + MCP. 사람은 정부 SSO 로그인 한 번만 하고 이후 포털 작업을 에이전트가 잇는 것이 핵심.

## 현재 상태 (2026-09-02)

- v0.16은 README의 공개 정체성을 문틈에서 조용히 얼굴을 내민 캐릭터와 한 줄 질문 중심으로 바꿨다.
  이름·문구·로고 경로의 기준은 `docs/brand/brand.json`이며, `go run ./scripts/sync-brand.go`가 README
  상단 블록을 갱신하고 CI의 `--check`가 불일치를 막는다.
- v0.14는 공식 월간 목록 CSV와 포털 제공형을 합친 약 9.6만 건의 prebuilt composite 카탈로그를
  릴리즈에 포함한다. API+FILE 복수 제공형을 보존하고, REST·LINK·FILE을 공통 `inspect_dataset`으로
  검사한 뒤 호출 가능한 계약만 신청·호출 흐름으로 넘긴다. 신규 통합 API+FILE 페이지, 공식 ODCloud
  Swagger, `multiCloudApiRequestForm` 신청 폼과 `api.odcloud.kr` HTTPS 호출을 지원한다.
- v0.12의 SafetyKorea, VWorld, FoodSafetyKorea, 서울 열린데이터광장 adapter와 11개 live canary를
  유지한다. SafetyKorea·FoodSafetyKorea·typed VWorld family는 provider-scoped key로 자동 호출하고,
  서울은 HTTPS 부재로 차단한다. v0.10의 connection discovery와 응답 필드 프로파일링, v0.9에서 도입한
  `oddsock` 이름을 저장소·Go module·기본 CLI·MCP에 사용한다. 이전 `opendatactl`과 `gongctl`
  바이너리·설정 경로·환경변수·MCP 가이드 URI는 전환 호환으로 유지한다.
- 목표 기반 카탈로그 검색, 선택형 Ollama 의미 검색, 검색→검사→활용신청→호출 MCP 흐름을 제공한다.
  discovery는 Generate→Search→Expand→Search→Compose→Search의 세 단계 검색으로 동작하며,
  Codex·Claude·Gemini는 전체 계획을, Cursor는 안전한 초기 검색 계획을 지원한다.
- data.go.kr KRDS 개편 파서와 세션 쿠키 회전 갱신을 적용했다.
- 온비드·나라장터·도매시장·중소기업 지원사업 API를 실계정으로 신청·승인·호출했다.
- 현재 설계 근거는 `docs/adr/`, discovery 계약은 `docs/specs/cross-domain-connection-discovery-v1.md`, 실제 검색 평가는
  `docs/research/connection-discovery-evaluation.md`와 `docs/research/semantic-search-evaluation.md`, 경쟁 조사는
  `docs/research/competitive-workflow-audit.md`와
  `docs/research/competitor-and-demand-cross-validation-2026.md`, 포털 경계는
  `docs/reverse-engineering/portal-catalog.md`가 단일 소스다. `docs/superpowers/specs/`와
  `docs/superpowers/plans/`는 최초 구현의 역사적 기록이고, 홍보 영상 제작 기록은
  `docs/promo/oddsock-agent-explainer.md`에 있다.

## 확정된 핵심 결정 (스펙 요약)

- **범위**: data.go.kr 전용, 깊게. 멀티포털(KOSIS·나라장터) 안 함.
- **호출 설계**: data.go.kr REST는 surface-only + 에이전트 주도, LINK는 검증된 provider adapter만
  typed 호출. 포털 명세를 임의로 추론하지 않으며, 외부 provider는 공식 계약·exact credential scope·
  typed operation registry가 있을 때만 호출한다. (kvote 국정수행 PDF 교훈.)
- **인터페이스**: CLI(사람) + MCP(에이전트), 같은 백엔드. kvote 패턴.
- **MCP tools**: catalog_search → inspect_dataset → apply(필요시) → call_api. `describe_api`는 API-only
  호환 surface이고 search_datasets와 list_applications는 보조 도구다. 인증키는 call_api 내부에서만
  사용하며 모델 컨텍스트로 반환하지 않는다.
  `call_api`는 key 생략 시 세션에서 자동 조회 → **검색→신청→승인확인→키→호출이 사람 개입 0**
  (로그인 1회 제외). 인증키는 `/iim/api/selectApiKeyList.do`의 `#pblisrCrtfcKeyPlain`에서 파싱.
- **인증키**: data.go.kr은 **계정당 일반 인증키 하나**(첫 신청 시 발급). 엔드포인트별 매칭 불필요.
  Encoding/Decoding 키 함정 있음 — 잘못 쓰면 조용히 실패, 에러 힌트로 surface.
- **로그인**: 정부 SSO는 자동화 안 함. 사람이 브라우저 1회(`oddsock login`) → **쿠키 추출 후 브라우저 종료**.
  읽기는 순수 HTTP(`internal/portal/session.go`), `apply`만 headless Chrome에 쿠키 주입해 폼 구동.
  tossinvest-cli의 storage-state 패턴을 이식(단, Python helper 없이 chromedp in-process).

## kvote에서 이식한 기반 (검증된 코드)

`~/workspace/projects/oss-k-vote-cli` 의 다음을 복사 이식(공유 라이브러리 추출 안 함 — §5):
- `internal/datagokr/*` (browser·apply·accounts·daemon·config) — 활용신청 CDP-attach 자동화의
  원형. 손 검증된 로직(SSO 트램펄린·JS 다이얼로그·
  `currentMyMenuId` 쿠키 전제) 그대로 가져올 것.
- `internal/nec` 의 datasets·openportal 검색 부분 → `internal/portal/search.go`.
- `internal/output`, `internal/version`, CLI/MCP 패턴, goreleaser·install.sh/ps1 파이프라인.
- NEC 전용 하드코딩 API(turnout/winners/elections)는 **가져오지 않음** — 범용 call_api로 대체.

## 신규로 구현한 것 (스펙 §4)

- `internal/apicall/describe.go` — OpenAPI 상세페이지 → REST 명세 또는 LINK 외부 제공기관 handoff surface.
- `internal/apicall/external_adapter*.go` — exact URL matcher, revision, canary와 공식 문서로 검증한 LINK 신청·인증 계약 registry.
- `internal/apicall/dataset_caller.go` — CLI/MCP 공통 REST/LINK dispatch와 typed operation gate.
- `internal/apicall/external_call.go`, `internal/providerauth/` — provider별 exact HTTPS caller와 scope 제한 key lifecycle.
- `internal/apicall/call.go` — data.go.kr 계정 인증키 주입 + HTTP GET + XML→JSON + 에러코드 surface.
- `internal/agentplan/` — provider별 계획 생성과 검색 결과 기반 확장·조합, 안전한 abstention.
- `internal/catalog/` — 공식 API·월간 CSV·웹 composite 수집, prebuilt snapshot, 키워드·의미 검색,
  release golden query gate, connection discovery의 제한·중복 제거·증거 경계.
- `internal/dataset/` — REST/LINK/FILE 공통 검사와 실제 FILE 자산·bounded CSV/DBF schema 관찰.
- `internal/apicall/profile.go` — 호출 응답의 선택 필드에 대한 경로·고유값 프로파일링.

## 경쟁 지형

검색→상세→호출 MCP와 9.6만 건 한국 카탈로그 검색기는 이미 존재한다. oddsock의 검증된 차이는
목표 기반 전체 카탈로그 탐색, API/FILE/LINK 실제 계약 검사, data.go.kr 활용신청·승인·키 재사용·
실호출을 하나로 연결하는 조합이다. 경쟁·수요 주장은
`docs/research/competitive-workflow-audit.md`와
`docs/research/competitor-and-demand-cross-validation-2026.md`의 고정 근거로만 갱신한다.

## 주의

- `.github/workflows/` 커밋은 git 토큰 **workflow 스코프** 필요(kvote에서 겪음, 해결됨).
- 이건 fragile scraping — data.go.kr HTML 바뀌면 파서가 조용히 빈 결과. `oddsock doctor`가
  각 seam(search·REST describe·LINK handoff·applications)을 라이브 호출해 drift를 시끄럽게 감지(CI용 exit 1).
  `doctor --adapters-only`는 4개 provider의 11개 canary와 180일 freshness만 점검하며 주간 workflow가
  실패 시 canonical GitHub issue를 생성·갱신한다. LINK matcher, invocation, provider key 또는 revision을
  바꿀 때는 `docs/provider-adapters.md`의 contract-test 규약을 먼저 읽는다.
- **보안(HIGH, 해결됨)**: `daemon.go`에서 `--remote-allow-origins=*` 제거(스파이크
  `proto/cdp-origin`로 검증 — chromedp는 flag 없이 재부착, 외부 Origin은 Chrome이 403 거부).
  이전 kvote verbatim 이식이 세션탈취 표면을 열어뒀던 것을 닫음.
- 배포: private GitHub Release + goreleaser + 인증된 `gh` 기반 install.sh/ps1. 공개 Homebrew 배포는 중단.

## Testing

- 기본 검증: `go test ./...`
- 릴리즈 전: `go mod tidy -diff && go vet ./... && go test ./... && go build ./...`
- 기능·버그 수정은 같은 package의 `*_test.go`에서 public seam을 먼저 실패시키고 구현한다.
- 외부 provider 성공 응답은 fixture transport로, 계약 drift는 `oddsock doctor --adapters-only`로 나눠 검증한다.
- 인증·로그아웃 테스트는 실제 명령을 수동 실행하지 않는다. macOS의 `os.UserConfigDir`는 `XDG_CONFIG_HOME`만으로
  격리되지 않으므로 테스트에서는 `HOME`, `XDG_CONFIG_HOME`, `APPDATA`를 모두 임시 경로로 지정한다.

## Agent skills

### Issue tracker

canonical tracker는 `JungHoonGhae/oddsock`의 GitHub Issues다. 구현 spec과 장기 문서는 저장소에
versioned Markdown으로 두고, 추적 이슈에서 링크한다. See `docs/agents/issue-tracker.md`.

### Triage labels

기본 5개 canonical 라벨(needs-triage/needs-info/ready-for-agent/ready-for-human/wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

single-context — 루트 `CONTEXT.md` + `docs/adr/`. See `docs/agents/domain.md`.
