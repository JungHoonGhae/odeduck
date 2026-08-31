# OpenDataCTL

> The AI control plane for Korean open data.

**공공데이터를 찾고, 신청하고, 호출하는 AI 컨트롤 플레인.**
정확한 검색어를 몰라도 AI가 필요한 공공데이터를 찾아 신청하고 실제 응답까지 가져옵니다.
data.go.kr에 한 번 로그인하면 Codex, Claude, Gemini, Cursor가 자연어 목표를 검색축으로 바꾸고,
11,900여 개 API의 검색·명세 확인·활용신청·승인 확인·호출을 CLI/MCP 한 경로로 끝냅니다.

이름의 `Open Data`는 누구나 재사용할 수 있도록 공개된 공공데이터를, `CTL`은 검색부터 실제
호출까지 하나의 명령 표면으로 연결하는 control plane을 뜻합니다. 데이터를 통제한다는 의미가
아니라, 흩어진 이용 절차를 AI가 실행할 수 있는 한 경로로 묶는다는 의미입니다.

실제 계정에서 온비드 공매, 나라장터 입찰, 공영도매시장 경매, 중소기업 지원사업 API를
신청하고 승인된 데이터까지 호출해 검증했습니다.

## 포털에서 끊기던 네 번을 한 번에

| 기존 흐름의 병목 | OpenDataCTL 원스톱 흐름 |
| --- | --- |
| 포털이 알아듣는 정확한 검색어를 사람이 추측 | 자연어 목표를 여러 기회축으로 나눠 전체 카탈로그 검색 |
| 결과가 호출 가능한지, 어떤 값이 필수인지 상세페이지를 돌며 판별 | `describe_api`가 상세기능·엔드포인트·필수 요청변수·심의유형 확인 |
| 활용신청 폼을 열어 목적을 쓰고 기능을 선택한 뒤 승인 상태를 다시 확인 | `apply`가 실제 포털 폼을 제출하고 자동승인 결과 확인 |
| 인증키를 복사하고 엔드포인트별 호출 코드를 별도 구현 | `call_api`가 `pk`로 명세를 검증하고 키를 주입해 XML도 JSON으로 반환 |

공개된 대체 CLI/MCP와의 기능별 비교 근거는
[경쟁 워크플로 조사](docs/research/competitive-workflow-audit.md)에 기록했습니다. 검색→상세→호출
형태의 MCP 자체는 이미 있습니다. 2026-08-31에 확인한 공개 구현과의 차이는 그 앞뒤입니다.
OpenDataCTL은 keyword gateway를 목표 기반 discovery로 확장하고, 기존 구현에서 빠져 있던
**활용신청·승인 확인·키 재사용**을 첫 실호출까지 연결합니다.

> [!WARNING]
> 활용신청 자동화는 data.go.kr의 HTML 구조에 의존하는 fragile scraping입니다. 포털 개편 시
> 깨질 수 있습니다. 정부 SSO 로그인 자체는 자동화하지 않습니다 — 브라우저로 사람이 1회 로그인하면
> 이후 세션을 재사용합니다.

## 설치

```sh
curl -fsSL https://raw.githubusercontent.com/JungHoonGhae/opendatactl/main/install.sh | sh
```

Windows:

```powershell
irm https://raw.githubusercontent.com/JungHoonGhae/opendatactl/main/install.ps1 | iex
```

Homebrew:

```sh
brew install --cask JungHoonGhae/opendatactl/opendatactl
```

또는 Go가 있다면:

```sh
go install github.com/JungHoonGhae/opendatactl/cmd/opendatactl@latest
```

### `gongctl`에서 이전

v0.9에는 새 `opendatactl`과 기존 스크립트·자동화를 위한 `gongctl` 호환 바이너리가 함께
들어 있습니다. 기존 사용자의 설정 디렉터리 로그인 세션·인증키·카탈로그도 복사 없이 그대로
사용합니다. 새 설치만 운영체제의 표준 설정 위치 아래 `opendatactl` 디렉터리에 상태를 저장합니다.
기존 Homebrew cask는 tap의 rename 매핑을 따라 `brew update && brew upgrade --cask gongctl`로
새 cask로 전환됩니다.

## 사용법

```sh
# 1. 브라우저가 한 번 열립니다 — data.go.kr에 로그인하세요 (SSO는 자동화하지 않음)
opendatactl login

# 2. 자연어 목표로 호출 가능한 후보 탐색
opendatactl catalog discover "우리 동네 대기질 서비스에 쓸 데이터" --rest-only

# 3. 명세·필수 요청변수·개발단계 승인유형 확인
opendatactl describe <PK>

# 4. 활용신청 (AI/MCP에서는 확인 없이 자동 제출, 첫 신청 때 인증키 자동 발급)
opendatactl apply <PK> --purpose "대기질 분석 프로젝트" --category research

# 5. 승인 확인
opendatactl applications -f table

# 6. 실제 호출 (XML 응답도 JSON으로 변환해 돌려줍니다)
opendatactl call --pk <PK> --param numOfRows=5   # 엔드포인트·인증키 자동
```

```bash
# 무엇이 존재하는지 먼저 훑기 — 로컬 카탈로그(한 번 sync 후 즉시 검색)
opendatactl catalog sync              # 전체 오픈API 목록 수집 (약 2~3분)
opendatactl catalog search 폭염 온열   # 활용신청 많은 순, 설명문 없이 간결하게
opendatactl catalog discover "내가 몰랐던 돈 될 만한 공공데이터"  # 로그인된 AI CLI로 검색축 생성
opendatactl catalog discover "지역 소멸로 생길 사업 기회" --agent gemini
opendatactl catalog search 폭염               # 기본: 실제로 호출 가능한 REST만
opendatactl catalog search 폭염 --rest-only=false  # LINK까지 포함한 전체 탐색
opendatactl catalog info               # 수집 시각 + 유형 분포
opendatactl catalog orgs 폭염          # 그 주제를 개방한 기관 순위
```

### 자연어 기회 탐색: 기존 AI 구독 우선, Ollama는 선택

`catalog discover`는 설치되어 있고 로그인된 **Codex, Claude Code, Gemini CLI, Cursor Agent** 중 하나를
검색 계획기로 사용합니다. 사용자가 정부 데이터의 정확한 명칭을 몰라도 목표를 거래·가격, 선행지표,
제약·위험, 지원·인프라 같은 3~8개의 서로 다른 검색축으로 바꾼 뒤 전체 카탈로그를 탐색합니다.
`--agent auto`가 기본이며 `codex | claude | gemini | cursor`로 고정할 수 있습니다. OpenDataCTL은 로그인
토큰을 읽거나 저장하지 않고, 각 CLI가 평소 사용하는 인증·요금제를 그대로 사용합니다. 검색 목표는
CLI의 stdin으로 전달해 로컬 프로세스 목록에 남기지 않으며, 읽기 전용/질문 모드로 실행합니다.
`auto`는 한 CLI가 실패하면 설치된 다음 CLI에도 같은 목표를 전달하므로 민감한 내용은 넣지 마세요.

MCP에서 쓸 때는 상위 Codex·Claude·Gemini·Cursor가 이미 검색 계획기입니다. 에이전트가
`catalog_search`의 `concepts`를 직접 채우므로 하위 CLI를 한 번 더 실행하지 않습니다. 즉 **MCP가
권장 기본 경로이고 Ollama 설치는 필수가 아닙니다.** 독립 CLI에서는 아래 비대화식 인터페이스를
사용합니다: [Codex](https://developers.openai.com/codex/noninteractive/),
[Claude Code](https://code.claude.com/docs/en/agent-sdk),
[Gemini CLI](https://geminicli.com/docs/cli/headless/),
[Cursor Agent](https://cursor.com/docs/cli/headless).

Ollama 의미 벡터는 에이전트가 만든 검색축 밖의 표현까지 추가로 회수하거나, 네트워크 없이 반복
검색할 때 쓰는 선택 기능입니다. [Ollama](https://docs.ollama.com/)만 설치한 뒤 아래 명령 한 번이면
추천 다국어 모델 다운로드, 11,902개 카탈로그 임베딩, 로컬 인덱스 저장까지 처리합니다. 별도 벡터 DB와
API 키는 필요하지 않습니다.

```bash
opendatactl catalog semantic-build
opendatactl catalog discover "시세보다 싸게 살 수 있는 물건" --rest-only
```

기본 모델은 한국어를 포함한 100개 이상 언어를 지원하는 약 238MB의
`embeddinggemma:300m-qat-q4_0`입니다. 인덱스가 준비되면 `catalog search`와 MCP `catalog_search`가
키워드 결과와 벡터 유사도를 자동 결합하고, Ollama나 인덱스가 없으면 기존 검색으로 폴백합니다.
현재 규모에서는 768차원 벡터 전체가 약 51MB라 메모리 평면 검색으로 충분합니다. 벡터 저장소는
별도 모듈 경계로 격리해, 향후 카탈로그가 수십만 건 이상이 될 때 벡터 DB로 교체할 수 있습니다.

> 포털은 키워드 검색만 제공합니다. 카탈로그가 없으면 "이런 데이터가 있나?"를 확인하려면
> 검색어를 하나씩 추측해볼 수밖에 없고, 못 찾았을 때 *없는 것*인지 *단어가 틀린 것*인지
> 알 수 없습니다. `catalog`는 전체를 로컬에 두고 한 번에 훑습니다.
> `doctor`가 카탈로그가 오래됐는지도 함께 점검합니다.
>
> **오픈API의 약 40%(11,932개 중 4,770개)는 `LINK` 유형**으로, 포털에 명세가 없고 제공기관
> 사이트로만 연결됩니다. 즉 `describe`가 엔드포인트를 줄 수 없어 신청해도 호출할 수 없습니다.
> LINK의 경우 `describe`가 제공기관 페이지 주소(`linkUrl`)를 넘겨주므로 명세를 읽는 것까지는
> 가능합니다(표본 70건 전부 주소가 있었습니다). 다만 제공기관은 롱테일이고(표본 70건에 호스트
> 39개, 최다 13%) 대부분 별도 회원가입·별도 인증키를 요구하므로, **"로그인 한 번"으로 호출까지
> 가는 경로는 LINK에 적용되지 않습니다.**
> 카탈로그는 각 항목의 유형을 표시하고, `--rest-only`로 걸러낼 수 있습니다 — 그냥 인기순으로
> 고르면 데드엔드에 활용신청을 쓰게 됩니다(`폭염` 검색 2위가 LINK입니다).
>
> 구체적인 검색어는 `catalog search`에 문장으로 써도 됩니다 — 조사(`~에서`, `~으로`)와 군더더기(`데이터`, `알려줘`)는
> 걸러집니다. 모든 단어를 포함하는 결과가 없으면 조용히 0건을 주는 대신 일부만 일치하는
> 것까지 보여주고, 그렇게 넓혔다는 사실을 `relaxed`로 알려줍니다. 모호한 목표는 MCP 호스트 모델이
> 여러 검색축으로 의미 분해합니다. 독립 터미널에서는 `catalog discover`가 같은 일을 로그인된 AI
> CLI로 수행합니다. OpenDataCTL은 축별 검색·선택적 벡터 검색을 합쳐 다양하게 반환합니다.

```bash
# 호출 — 엔드포인트 URL 을 타이핑하지 않습니다
opendatactl call --pk 15077974 --param pageNo=1 --param numOfRows=3 --param type=xml
```

> **승인 조건은 `describe`의 `approval`에 나옵니다.** 포털은 개발단계와 운영단계를 따로
> 심의하는데, OpenDataCTL이 쓰는 개발계정 경로는 조사한 데이터셋에서 사실상 모두 자동승인이었습니다
> (무작위 90건 표본에 개발단계 심의는 0건). 운영단계는 약 1/3이 심의승인이므로, 나중에 상용으로
> 옮길 계획이면 미리 확인할 값입니다. 그 행이 없는 데이터셋(대개 LINK)은 자동승인으로
> 가정하지 않고 `approval` 없음으로 보고합니다.
>
> 엔드포인트 경로는 추측할 수 없는 형태(`HeatWaveCasualtiesRegion/getHeatWaveCasualtiesRegionList`)이고,
> 틀리면 "없다"가 아니라 404·500이 옵니다. `--pk` 를 주면 포털에서 조회하고, **명세의 필수
> 요청변수가 빠졌는지 호출 전에 확인**합니다 — 빠진 채로 부르면 data.go.kr은 에러가 아니라
> 빈 결과를 주기 때문에 데이터가 없는 것과 구분할 수 없습니다.

```bash
# 계정 인증키 조회 (call 은 생략 시 자동으로 이 키를 씁니다)
opendatactl key

# 스크래핑이 아직 살아있는지 점검 (data.go.kr HTML 변경 감지, CI용 exit 1)
opendatactl doctor -f table
```

> **사람의 개입은 `opendatactl login` 한 번뿐입니다.** 검색 → 활용신청 → 승인 확인 → 인증키 획득 →
> 호출까지 에이전트가 스스로 끝냅니다. 인증키를 사람이 복사해 붙여넣을 필요가 없습니다.
>
> **신청 직후 403은 정상입니다.** (`call --wait 10m` 을 주면 opendatactl이 1분 간격으로 재시도하며
> 기다립니다.) 승인은 즉시 끝나지만 게이트웨이 반영에 시간이 걸립니다 —
> 실측 **7~10분**, 포털 안내상 최대 1시간. `list_applications`에 '승인'으로 보여도 아직
> 호출이 안 될 수 있습니다. 1~2분 간격으로 재시도하면 되고, 키를 바꾸거나 다시 신청할 필요는
> 없습니다. 여러 개를 쓸 계획이면 **먼저 다 신청해두고 함께 기다리는 편이 빠릅니다.**

`opendatactl status` / `opendatactl logout` / `opendatactl version`도 있습니다.

## MCP 서버로 쓰기

`opendatactl mcp`는 stdio MCP 서버로 동작합니다. 수천 개의 개별 API를 MCP 도구로 한꺼번에 노출하지
않고, 아래의 작은 흐름이 필요한 정보와 권한만 단계적으로 가져옵니다.

1. `catalog_search` — 자연어 목표를 모델이 여러 검색축으로 의미 분해하고, 로컬 키워드·선택적 Ollama
   벡터 검색을 결합해 작은 후보 목록을 반환
2. `describe_api` — 선택한 `pk` 하나의 상세기능·엔드포인트·요청변수를 반환
   - `apply`(2.5단계) — 미승인 API라면 AI가 활용목적을 작성해 신청하고 자동승인 결과를 확인
3. `call_api` — 같은 `pk`와 확인한 파라미터로 명세 검증 후 실제 호출

MCP의 `call_api`는 raw endpoint와 인증키를 입력받지 않습니다. 항상 `pk`로 명세를 다시 확인하고
로그인 세션의 키를 주입하므로 상세 단계를 우회할 수 없습니다. 최신 데이터 재확인용
`search_datasets`와 계정 확인용 `list_applications`는 보조 도구로 분리되어 있습니다. 인증키는 MCP 도구로 노출하지 않고 `call_api` 내부에서만 주입합니다.
`apply`는 보조 기능이 아니라 **발견한 데이터를 실제로 쓸 수 있게 만드는 핵심 연결 단계**입니다.
로그인 한 번 뒤에는 에이전트가 명세 확인, 신청 폼 제출, 승인 상태 확인, 인증키 주입과 호출까지
스스로 이어갑니다.

Codex·Claude Code·Gemini CLI에서는 각자 한 줄로 등록할 수 있습니다. 이 경로에서는 해당 호스트
모델이 자연어 목표를 검색축으로 만들므로 `catalog discover`가 하위 에이전트를 다시 실행하지 않습니다.

```bash
codex mcp add opendatactl -- opendatactl mcp
claude mcp add opendatactl -- opendatactl mcp
gemini mcp add --scope user opendatactl opendatactl mcp
```

Cursor와 Claude Desktop처럼 JSON 설정을 쓰는 클라이언트의 예시는 같습니다:

```json
{
  "mcpServers": {
    "opendatactl": {
      "command": "opendatactl",
      "args": ["mcp"]
    }
  }
}
```

Cursor Agent는 `cursor-agent mcp list-tools opendatactl`로 연결과 도구 목록을 확인할 수 있습니다.
ACP는 편집기와 에이전트 사이의 세션 프로토콜이고, opendatactl 같은 도구 서버를 연결하는 경계는 MCP입니다.
따라서 Gemini/Cursor의 ACP 실행 모드를 별도 추론 API처럼 중첩하지 않고, 호스트가 MCP
`catalog_search → describe_api → (미승인 시 apply) → call_api`를 호출하게 합니다.

## 보안 주의

`opendatactl login`은 브라우저 창을 한 번 띄웁니다(정부 SSO는 자동화하지 않습니다). 로그인이 확인되면
OpenDataCTL이 세션 쿠키를 복사해 저장하고 **그 브라우저를 종료합니다** — 이후 명령은 창 없이 동작합니다.
세션이 유효한 동안에는 `applications`·`key` 같은 인증 요청에서 포털이 회전시킨 쿠키를 자동 병합해
`datagokr-session.json`을 갱신하므로, 계속 사용하는 세션은 가능한 범위에서 연장됩니다. 정부 SSO의
절대 만료나 재인증 요구가 오면 이를 우회하지 않고 다시 `opendatactl login`을 안내합니다.

**동작 방식**

- **읽기**(`applications`·`key`·`search`·`describe`)는 브라우저 없이 순수 HTTP로 처리됩니다. `doctor`는 기본적으로 신청 폼까지 headless Chrome으로 점검하며, `--skip-apply`를 주면 HTTP 점검만 수행합니다.
- **활용신청 제출**(`apply`)만 브라우저가 필요합니다(포털 폼의 검증 로직을 그대로 구동 — ADR 0001).
  이때 **headless** Chrome을 잠깐 띄워 저장된 세션을 주입하고, 끝나면 정리합니다.
- Chrome은 `--remote-allow-origins`를 **설정하지 않고** 실행됩니다. 악성 웹페이지가 보내는
  Origin 헤더 붙은 WebSocket 연결은 Chrome이 403으로 거부합니다(스파이크로 검증). chromedp는
  Origin 없이 붙으므로 재부착에는 영향이 없습니다.
- 인증키는 로그·명령 출력에 남기지 않고 전송 실패 메시지에서도 마스킹합니다. 호출 지속성을 위한
  로컬 캐시 한 곳에만 아래와 같이 `0600`으로 저장합니다.

**디스크에 저장되는 것** (디렉터리 `0700`)

- macOS: `~/Library/Application Support/opendatactl`
- Linux: `~/.config/opendatactl`
- Windows: `%AppData%\opendatactl`
- 기존 사용자는 같은 위치의 `gongctl` 디렉터리를 자동으로 계속 사용

| 파일 | 내용 | 권한 |
| --- | --- | --- |
| `datagokr-session.json` | data.go.kr 세션 쿠키(인증 요청 중 자동 갱신) | `0600` |
| `datagokr-apikey` | 계정 인증키(serviceKey) | `0600` |
| `chrome-profile/` | 로그인용 Chrome 프로파일 | `0700` |
| `chrome-headless-*` | 신청 제출 중에만 존재하는 격리된 임시 headless 프로파일 | `0700` |
| `catalog.json` | 공개 OpenAPI 카탈로그 스냅샷 | `0644` |
| `catalog-semantic.gob` | 선택 기능인 공개 카탈로그 의미 벡터 | `0600` |

**`opendatactl logout`은 세션·키·두 Chrome 프로파일을 삭제하고 공개 카탈로그 파일은 유지합니다.** 로그인 프로파일에는 사람이 로그인에 사용한
SSO 제공자(네이버 등)의 쿠키도 함께 쌓이기 때문에, 쿠키 파일만 지우는 것으로는 충분하지 않습니다.
작업이 끝나면 `logout`을 실행하세요.

**남는 위험 — 알고 쓰세요**

- **평문 저장**: 위 파일들은 `0600`이지만 암호화되지 않습니다. 같은 사용자 권한으로 실행되는
  악성 프로그램이나 백업 사본은 읽을 수 있습니다.
- **로컬 CDP 포트**: `login`과 `apply` 동안 Chrome이 `127.0.0.1`의 디버깅 포트를 엽니다.
  Chrome의 Origin 검사는 *브라우저에서 오는* 연결만 막으므로, **같은 머신의 다른 프로세스**는
  그 창이 살아 있는 동안 붙을 수 있습니다. 노출 구간은 짧지만 0은 아닙니다 —
  공용/다중 사용자 머신에서는 사용을 피하세요.
- **MCP `apply`는 사람 확인 없이 실제 신청을 생성합니다**(에이전트 주도가 목적). 에이전트는
  포털에서 읽은 텍스트를 근거로 행동하므로, 프롬프트 인젝션으로 원치 않는 신청이 만들어질
  수 있습니다. 신청은 포털에서 취소할 수 있고 파괴적이지 않지만, **계정에 실제 변경을 남깁니다.**
  민감한 계정에서는 MCP 대신 CLI(`apply`는 기본적으로 y/N 확인)를 쓰세요.
- **인증키는 계정 단위**입니다. 유출되면 승인된 모든 API가 함께 노출됩니다.

## 라이선스

[MIT](LICENSE)
