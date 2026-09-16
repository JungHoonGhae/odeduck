> 이 README는 오데덕이 직접 안내하는 설정이라, 일부러 반말로 썼습니다. 편하게 읽어주세요.

<!-- brand:start -->
<p align="center">
  <img src="docs/assets/brand-symbol.svg" width="140" alt="흩어진 공공데이터의 연결을 찾는 오데덕 캐릭터">
</p>

<h1 align="center">오데덕 · odeduck</h1>

<p align="center"><strong>The open-source control plane for Korean public data.</strong><br>
  Give AI agents a local runtime for public-data goals: discovery, API access, queries, analysis, and source evidence.
</p>

<p align="center">
  공공데이터는 열려 있습니다.<br>
  찾고, 신청하고, 가져오는 건<br>
  이 덕후가 합니다.
</p>

<p align="center"><strong>공공데이터 덕후, 오.데.덕.</strong></p>
<!-- brand:end -->

<p align="center">
  <img src="https://img.shields.io/badge/works%20with-Codex%20%C2%B7%20Claude%20%C2%B7%20Gemini%20%C2%B7%20Cursor-111111?style=flat-square" alt="Works with Codex, Claude, Gemini, and Cursor">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-111111?style=flat-square" alt="MIT license"></a>
</p>

**AI 에이전트가 한국 공공데이터로 목표를 실행하도록 연결해.** 필요한 자료를 찾고, 접근권한을
신청하고, 실제 값을 가져와 계산·비교와 근거를 만드는 로컬 실행 기반이야.

- **목표에서 시작:** `solve` · MCP `advance_goal`로 필요한 자료와 결과를 찾아가.
- **실제 접근과 실행:** 계약 검사 → 활용신청 → 승인 확인 → 인증키 재사용 → API 호출.
- **96,000+개 자료 탐색:** API·파일·외부 제공기관 자료를 함께 검색해. 첫 검색은 로그인·API 키 없이 가능해.

[설치하고 연결하기](#빠른-시작) · [목표 실행](#목표에서-산출물까지-solve) · [Agent Skill](#agent-skills) · [전체 구조](#전체-아키텍처) · [검증 근거](docs/validation-summary.md)

사람은 정부 SSO에 직접 로그인해. 기관 심의가 필요한 API는 승인을 기다려야 해.
카탈로그의 모든 자료가 자동 신청·호출 대상인 것은 아니며, [제공형별 지원 범위](#자료별-지원-범위)를 따라.

<!-- Keep the introduction animation visible. Core capabilities and architecture must also remain visible, not collapsed. -->
<p align="center">
  <img src="docs/assets/odeduck-hero.gif" width="800" alt="거대한 캐비닛 사이를 뛰어다니는 오데덕. 좌측 하단에 투명한 빼꼼 로고, 궁서체 오.데.덕., GitHub 주소가 세로로 배치되어 있다.">
</p>

---

<a id="찾은-자료로-비교표-만들기--실험-기능"></a>

## 목표에서 산출물까지: solve

**오데덕의 핵심 사용 방식은 “이 자료 찾아줘”에서 시작해, “이 목표에 필요한 결과를 만들어줘”까지 가는 거야.**
CLI의 `solve`와 MCP의 `advance_goal`이 같은 목표 실행기를 사용해.

> 이 공매 아파트, 비슷한 거래보다 싼지 비교해줘. 비교 조건과 출처, 자료가 부족한 물건도 표시해줘.

목표를 필요한 역할·지역·기간·출력으로 나누고, 자료를 찾아 검사한 뒤 실제 값을 취득해 계산해.
비교 조건이 안 맞거나 근거가 부족하면, 같은 목표 안에서 다른 자료나 중간 연결 자료를 다시 찾아.

```text
목표 → 필요한 역할·출력 정의 → 탐색 → 검사 → 취득 → 계산·연결 → 결과 검토
                                ↑                           │
                                └──── 빠진 근거·대안 재탐색 ──┘
```

| 결과에 담는 것 | 사용자가 확인할 것 |
| --- | --- |
| 보고·비교·계산한 값 | 요청한 항목과 범위가 채워졌는지 |
| 원천과 계산 근거 | 어떤 자료·기간·조건을 사용했는지 |
| 미대응 기록과 남은 공백 | 무엇이 부족하고 어떤 판단이 아직 불가능한지 |

![설명용 가상 예시: 공매 물건의 최소입찰가와 비교할 실거래가를 나란히 놓고, 비교 자료가 부족한 물건과 출처·비교 조건을 함께 표시.](docs/assets/odeduck-goal-flow.png)

그림의 물건과 금액은 **설명용 가상 예시**야. 공매 분석을 끝까지 수행한 실제 결과는 아니야.

### 현재 실행할 수 있는 범위

API·CSV·ZIP 내부 CSV·XLSX·표준데이터의 제한된 관측으로 조회·연결·집계를 실행하고,
출처와 계산 근거를 남겨. 선택한 원천을 바탕으로 보고·관계·계산을 별도 모델이 검토하는 경로도 있어.

**목표 실행은 핵심 개발 경로이며, 현재 릴리스의 안정성 표시는 experimental이야.**
키워드나 데이터 ID 없이 다양한 목표를 자율적으로 완주하는 능력은 아직 검증 중이야.
표본 계산 성공과 목표 완료는 구분하고, 필요한 결과 검토가 끝나지 않으면 미완료로 반환해.

시작하려면 [목표 실행 설정과 명령](docs/advanced-usage.md#목표에서-결과까지-실행하기--experimental)을 따라.
의미 검색·계획 모델·원천 공유·결과 검토의 설정과 비용을 설명해뒀어.
현재 `solve` 안에서 활용신청을 자동 제출하지는 않아. 필요한 API 접근은
[일반 신청·조회 경로](#활용신청과-데이터-조회)로 마련해.
[확인한 결과와 남은 검증](docs/validation-summary.md) · [제품이 도달하려는 결과](INTENT.md)

## 빠른 시작

**먼저 터미널에서 설치 명령을 실행해.** 이미 설치했으면 [2번](#2-첫-검색-해보기)부터 시작해.
아래 네 단계를 마치면 AI에 목표 실행이나 자료 찾기를 맡길 수 있어.

### 1. 설치하기

macOS·Linux라면 이 명령을 실행해.

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.sh | sh
```

<details>
<summary>Windows 설치 명령</summary>

```powershell
irm https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.ps1 | iex
```

</details>

검색에 쓸 데이터 목록도 같이 설치돼.

### 2. 첫 검색 해보기

로그인·API 키·AI 연결 없이 먼저 검색해봐.

```sh
odeduck catalog search "건축물" --limit 5 --semantic=false -f table
```

**데이터 목록이 나오면 성공.** 다음은 AI에 연결할 차례야.

### 3. 사용하는 AI에 연결하기

쓰는 AI에 오데덕을 연결해. Codex라면 아래 명령이야.

```sh
codex mcp add odeduck -- odeduck mcp
```

<details>
<summary>Claude·Gemini 연결 명령</summary>

사용하는 AI의 명령 하나만 실행해.

```sh
# Claude
claude mcp add odeduck -- odeduck mcp

# Gemini
gemini mcp add --scope user odeduck odeduck mcp
```

</details>

<details>
<summary>Cursor·Claude Desktop 연결 설정</summary>

앱의 MCP 설정에 아래 내용을 추가해.

```json
{
  "mcpServers": {
    "odeduck": {
      "command": "odeduck",
      "args": ["mcp"]
    }
  }
}
```

</details>

**도구 목록에 `odeduck`이 보이면 연결 완료.** Codex에서는 `codex mcp list`로 확인해.

### 4. AI에 목표 맡기기

AI 앱에서 새 대화를 열고 아래 문장을 붙여 넣어. 연결이 안 보이면 앱을 한 번 재시작해.

> 오데덕으로 최근 공영도매시장 양파 가격을 시장별로 비교해줘. 등급·단위·기간을 맞추고, 비교할 수 없는 자료와 출처도 표시해줘. 필요한 API는 신청하고 승인 상태를 확인해줘.

보고·비교·계산을 맡기면 AI가 `advance_goal`로 목표 실행을 시작해. 의미 검색과 결과 검토에는
[추가 설정](docs/advanced-usage.md#목표에서-결과까지-실행하기--experimental)이 필요해.
인증 조회가 필요하면 [로그인과 활용신청](#활용신청과-데이터-조회)으로 이어가.

가볍게 탐색부터 해보려면 “우리 회사가 신청할 만한 지원사업 자료를 찾아줘”라고 맡겨도 돼.
**자료 후보·출처를 얻은 단계와, 요청한 비교표·보고서가 완성된 단계는 따로 확인해.**

## Agent Skills

에이전트에 오데덕 사용 절차를 설치할 수도 있어. **목표를 받으면 `solve`·`advance_goal`로,
개별 자료가 필요하면 검색·검사·신청·호출로 안내하는 사용자용 Skill**이야.

```sh
npx skills@latest add JungHoonGhae/odeduck --skill odeduck
```

Node.js·`npx`가 필요해. 사용할 에이전트와 설치 범위를 고르면 돼.
Skill 설치와 실행 바이너리·MCP 연결은 별개야. 빠진 설정은 Skill의 설치 안내를 따라 준비해.
MCP만으로도 사용할 수 있어.

[설치·갱신·로컬 사용법](docs/agent-skills.md) · [Skill 원본](skills/odeduck/SKILL.md)

## 실행 기반: 탐색·신청·인증·호출

**목표에 필요한 데이터를 쓰려면, 접근권한과 실제 호출까지 이어져야 해. 오데덕이 이 절차를 맡아.**
Codex·Claude·Gemini·Cursor에 연결해서 쓸 수 있어.

![데이터 탐색 → 내용 확인 → 활용신청 → 승인 확인 → 인증키 자동 입력 → 데이터 조회. 사람은 data.go.kr에 한 번 직접 로그인.](docs/assets/odeduck-api-workflow.png)

포털 로그인은 직접 한 번 해줘. 그 뒤 오데덕이 필요한 API를 각각 확인하고,
미신청 API는 신청한 뒤 승인된 데이터를 가져와. 인증키도 알아서 넣어.

AI 앱의 설정에 따라 신청서 제출 전에 확인을 요청할 수 있고, 기관 심의가 필요한 API는 승인을 기다려야 해.
온비드·나라장터·공영도매시장·중소기업 지원사업 데이터는 [실제 계정으로 신청·조회까지 확인했어](docs/validation-summary.md).

## 왜 이 흐름을 묶었나

공공데이터는 열려 있어도, 자료를 쓰기까지는 검색어·신청·승인·인증키·호출 방법을 챙겨야 해.
필요한 자료가 하나 늘 때마다 이 준비가 반복돼.

![공공데이터 이용 전반의 병목: 검색어를 바꿔가며 자료 찾기, 필요한 API마다 신청과 승인 확인 반복, 인증키 입력 방식과 호출 직접 설정.](docs/assets/odeduck-manual-bottlenecks.png)

오데덕은 이 절차를 에이전트의 목표 실행에 연결해.

| 필요한 일 | 오데덕이 맡는 부분 |
| --- | --- |
| 데이터 이름을 몰라도 시작하기 | 목표에서 검색축을 만들고 API·FILE·LINK 후보를 탐색 |
| 실제 쓸 수 있는 자료인지 확인하기 | 원천의 입력값·항목·제공형과 지원 파일의 컬럼·표본 검사 |
| 데이터를 사용할 권한 준비하기 | 선택한 API의 활용신청·승인 확인·인증키 재사용 |
| 질문에 필요한 결과 만들기 | 목표 실행기에서 취득·계산·재탐색하고 산출물·근거·공백을 평가 |

뜻이 가까운 설명을 찾는 의미 검색은 [로컬 인덱스 설정](docs/advanced-usage.md#cli에서-목표로-탐색하기) 후 쓸 수 있어.
검색·상세·호출 자체를 제공하는 기존 도구도 있어. 조사한 공개 구현과 오데덕의 지원 범위는
[경쟁 워크플로 감사의 2026-09-12 재확인](docs/research/competitive-workflow-audit.md#2026-09-12-재확인)에 기록했어.

## 활용 예시

필요한 데이터의 이름 대신, **무엇을 하려는지** 말해봐.

| 이렇게 맡겨봐 | 함께 찾아볼 자료 |
| --- | --- |
| “카페를 열 자리를 고르고 있어. 손님이 올 만한 곳인지 확인할 자료를 찾아줘.” | 생활인구, 업종별 매출, 주변 점포 |
| “우리 회사가 신청할 만한 지원사업을 찾아줘.” | 지원사업 공고, 신청 자격, 접수 기간 |
| “식당 재료비를 줄이고 싶어. 최근 도매가격과 반입량을 볼 자료를 찾아줘.” | 품목별 도매가격, 반입량, 거래 지역 |

오데덕이 관련 자료를 찾고, 필요한 항목이 있는지 확인해서 **데이터 후보와 출처**를 가져와.
찾은 값을 맞춰 비교표를 만드는 기능은 [목표 실행](#목표에서-산출물까지-solve)이 맡아. 현재 지원 범위와 검증 상태는 아래 설명을 확인해.

### 공매 예시: 가격과 위험을 함께 확인

로그인과 AI 연결을 마쳤다면 아래 요청을 붙여 넣어봐.

> 싸게 나온 부산 공매 부동산을 보고 있어. 비교할 가격과 놓치기 쉬운 위험을 찾아줘. 필요한 API는 신청·조회해서 함께 볼 자료와 확인할 점을 정리해줘.

![부산 공매 부동산의 가격과 위험을 살펴보는 예시. 실거래가, 상권 변화, 토양오염 조사를 후보로 탐색하고, 필요한 API의 신청·승인 확인·인증키 자동 입력·조회까지 이어가는 흐름.](docs/assets/odeduck-unexpected-connections.png)

이 질문으로 실제로 찾아보니 **실거래가, 부산 상권 변화, 토양오염 조사**가 후보로 나왔어.
가격을 비교하려다 주변 수요와 환경을 살펴볼 단서까지 찾은 거야. 자료 이름을 미리 전부 알 필요는 없어.

함께 찾은 자료라도 물건 종류·지역·기간이 맞는지 확인해야 해.
토양 조사 기록만으로 특정 물건이 오염됐다고 단정할 수는 없어.
[실제 발견 기록과 원천 확인](docs/research/connection-discovery-evaluation.md#readme-연결-예시--2026-09-12)에 근거를 남겨뒀어.

## 활용신청과 데이터 조회

### 1. 포털에 로그인하기

터미널에서 아래 명령을 실행해. 열린 브라우저에서 data.go.kr 로그인을 직접 마쳐줘.

```sh
odeduck login
```

### 2. AI에 신청과 조회 맡기기

로그인이 끝나면 AI 대화에 아래 요청을 붙여 넣어.

> 방금 찾은 데이터의 내용을 확인해줘. 활용신청이 필요하면 신청하고, 승인 상태를 확인한 뒤 조회해줘.

오데덕이 신청·승인 상태를 확인하고, 승인된 API에서 값을 가져와.
**인증키는 채팅창에 붙여 넣지 않아도 돼.** data.go.kr의 계정 인증키를 재사용해서 자동으로 입력해.
기관 심의 중이면 승인될 때까지 조회를 기다려야 해.

### 자료별 지원 범위

자동 신청은 data.go.kr에서 직접 제공하는 REST API를 지원해.

| 자료 종류 | 오데덕이 하는 일 |
| --- | --- |
| data.go.kr API | 필요한 활용신청, 승인 확인, 인증키 입력과 조회 |
| 파일·표준데이터 | 지원되는 CSV·엑셀 등의 항목과 일부 데이터를 확인 |
| 외부 제공기관의 데이터 | SafetyKorea·FoodSafetyKorea·VWorld 등 지원 기관은 별도 인증키로 조회. 미지원 기관은 공식 이용 경로 안내 |

직접 명령으로 신청하거나 조회하려면 [상세 사용법](docs/advanced-usage.md#활용신청과-첫-호출)을 보면 돼.

## 전체 아키텍처

**모델은 목표와 다음 행동을 제안하고, Go 실행기는 실제 원천에서 데이터를 취득하고 계약에 따라 실행해.**
이 역할을 내 컴퓨터에서 연결하는 구조를 *local control plane*이라고 불러.

| 구성 | 맡는 일 |
| --- | --- |
| Agent Skill | 목표 실행·개별 조회의 진입과 필요한 설정을 안내 |
| MCP / CLI | Codex·Claude·Gemini·Cursor 또는 터미널을 공통 실행기에 연결 |
| `solve` / `advance_goal` | 목표·출력 정의, 탐색·취득·계산·재계획과 결과 평가 |
| Go 실행 모듈 | 원천 계약 검사, 세션·인증키 취급, 신청·호출, 실제 행의 계산과 실행 검증 |

![CLI와 MCP가 같은 엔진을 사용하는 전체 구조. 공통 탐색과 내용 확인 후 파일을 관측하고 API의 신청·인증·호출을 수행.](docs/assets/odeduck-system-overview.png)

인증키는 실행기가 내부에서 사용해. 모델이 만든 행이나 성공 선언으로 실제 취득·계산을 대체하지 않아.
자료의 의미와 목표 충족은 별도 검토 대상이고, 모델 검토는 현장 검증을 보장하지 않아.
CLI와 MCP는 같은 실행 기준을 사용하며, `solve`의 CLI 계획기는 Codex·Claude·Gemini를 지원해.

[상세 아키텍처](ARCHITECTURE.md) · [실행·검증 계획](docs/specs/goal-driven-completion-plan.md)

## 로그인 정보와 데이터 공유

로그인 정보와 인증키는 내 컴퓨터에 저장돼.
AI에 연결하면 질문과 조회 결과는 사용하는 AI 앱이 받아.
공유하면 안 되는 내용은 질문에 넣지 마. 자료별 이용조건도 확인해.

저장된 로그인 정보와 인증키를 지우려면 아래 명령을 실행하면 돼.

```sh
odeduck logout
```

자세한 범위는 [로그인과 데이터 취급 안내](docs/advanced-usage.md#인증과-데이터-취급)에 적어뒀어.

## 질문·오류 제보

사용법은 [질문 게시판](https://github.com/JungHoonGhae/odeduck/discussions)에,
오류는 [이슈](https://github.com/JungHoonGhae/odeduck/issues)에 남겨줘.

찾으려던 자료와 나온 오류 메시지를 같이 적어주면 확인하기가 수월해.
인증키나 로그인 정보는 빼고 올려줘.

[상세 사용법](docs/advanced-usage.md) · [업데이트 내역](CHANGELOG.md) · [기여 안내](CONTRIBUTING.md) · [보안 제보](SECURITY.md)

## 라이선스

[MIT](LICENSE).
