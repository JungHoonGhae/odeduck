<!-- brand:start -->
<p align="center">
  <img src="docs/assets/brand-symbol.svg" width="140" alt="흩어진 공공데이터의 연결을 찾는 오데덕 캐릭터">
</p>

<h1 align="center">오데덕 · odeduck</h1>

<p align="center"><strong>공공데이터 탐색부터 자동 활용신청·분석까지</strong><br>
  AI 에이전트가 필요한 자료를 찾고, 데이터를 가져와 결과와 근거를 만들도록 돕습니다.
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

<p align="center">
  <a href="#빠른-시작">설치와 연결</a> ·
  <a href="#활용신청과-데이터-조회">자동 활용신청</a> ·
  <a href="#목표에서-산출물까지-solve">목표 기반 분석</a> ·
  <a href="#agent-skills">Agent Skill</a> ·
  <a href="#전체-아키텍처">아키텍처</a>
</p>

<!-- Keep the introduction animation visible. Core capabilities and architecture must also remain visible, not collapsed. -->
<p align="center">
  <img src="docs/assets/odeduck-hero.gif" width="800" alt="오데덕이 거대한 캐비닛 사이에서 자료를 찾아 연결하는 소개 애니메이션">
</p>

## 오데덕으로 할 수 있는 일

오데덕은 **AI 에이전트가 한국 공공데이터를 찾아 활용하도록 돕는 오픈소스 CLI·MCP**입니다.
터미널에서 직접 사용하거나, Codex·Claude·Gemini·Cursor에 연결해 자연어로 작업을 요청할 수 있습니다.

공공데이터를 쓰려면 자료 검색뿐 아니라 이용 방법 확인, 활용신청, 승인 확인, 인증키 준비가 필요합니다.
오데덕은 이 과정을 연결하고, 가져온 데이터로 질문에 필요한 결과를 만드는 기능을 제공합니다.

| 하고 싶은 일 | 오데덕이 돕는 방법 |
| --- | --- |
| 필요한 자료 찾기 | 96,000개 이상의 API·파일·외부 제공기관 자료를 검색하고, 항목과 이용 방법을 확인합니다. |
| API 데이터 가져오기 | 필요한 활용신청을 제출하고, 승인 상태를 확인한 뒤 인증키를 자동으로 넣어 호출합니다. |
| 비교표·보고서 만들기 | `solve`·`advance_goal`로 자료 수집과 계산을 진행하고, 결과의 출처와 부족한 근거를 함께 남깁니다. |

**첫 검색은 로그인이나 API 키 없이 시작할 수 있습니다.** 활용신청과 인증이 필요한 조회는 포털 로그인 후 사용할 수 있습니다.
목표 기반 분석의 현재 지원 범위는 [아래 설명](#목표에서-산출물까지-solve)에서 확인할 수 있습니다.

## 빠른 시작

설치 → 첫 검색 → AI 연결 → 요청 순서로 진행합니다.

### 1. 설치하기

macOS·Linux 터미널에서 아래 명령을 실행합니다. 검색에 사용할 데이터 목록도 함께 설치됩니다.

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.sh | sh
```

<details>
<summary>Windows 설치 명령</summary>

PowerShell에서 실행합니다.

```powershell
irm https://github.com/JungHoonGhae/odeduck/releases/latest/download/install.ps1 | iex
```

</details>

### 2. 첫 검색 해보기

아래 명령으로 설치와 검색이 정상적으로 동작하는지 확인합니다. 로그인이나 AI 연결은 필요하지 않습니다.

```sh
odeduck catalog search "건축물" --limit 5 --semantic=false -f table
```

자료 목록이 표시되면 다음 단계로 진행합니다.

### 3. 사용하는 AI에 연결하기

사용하는 AI에 해당하는 명령 하나를 실행합니다.

**Codex**

```sh
codex mcp add odeduck -- odeduck mcp
```

**Claude Code**

```sh
claude mcp add odeduck -- odeduck mcp
```

**Gemini CLI**

```sh
gemini mcp add --scope user odeduck odeduck mcp
```

<details>
<summary>Cursor·Claude Desktop 연결 설정</summary>

앱의 MCP 설정에 아래 내용을 추가합니다.

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

도구 목록에 `odeduck`이 표시되면 연결이 완료된 것입니다.
Codex에서는 `codex mcp list`로 확인할 수 있습니다. 연결이 보이지 않으면 AI 앱을 다시 시작해주세요.

### 4. AI에 목표 맡기기

새 대화를 열고 필요한 작업을 요청합니다.

> 오데덕으로 우리 회사가 신청할 만한 지원사업 자료를 찾아주세요. 신청 자격과 접수 기간을 확인할 수 있는 자료가 필요합니다.

비교나 분석이 필요하다면 원하는 결과와 조건도 함께 알려주세요.

> 오데덕으로 최근 공영도매시장 양파 가격을 시장별로 비교해주세요. 등급·단위·기간을 맞추고, 출처와 비교할 수 없는 자료도 표시해주세요.

자료 탐색은 바로 시작할 수 있습니다. API 접근권한이 필요하면 [활용신청](#활용신청과-데이터-조회)을 진행합니다.
보고서·비교표를 만드는 목표 실행에는 [분석 설정](docs/advanced-usage.md#목표에서-결과까지-실행하기--experimental)이 추가로 필요합니다.

### Agent Skills

에이전트에 오데덕의 사용 절차를 알려주는 **사용자용 Skill**도 설치할 수 있습니다.
자료 검색, API 조회, 목표 기반 분석 중 요청에 맞는 경로를 선택하고 필요한 설정을 안내합니다.

Node.js와 `npx`가 있는 환경에서 실행한 뒤, 사용할 에이전트와 설치 범위를 선택합니다.

```sh
npx skills@latest add JungHoonGhae/odeduck --skill odeduck
```

Skill은 선택 사항이며 MCP 연결만으로도 사용할 수 있습니다. 실제 작업은 설치된 오데덕 CLI 또는 MCP를 통해 실행합니다.
[설치·업데이트 안내](docs/agent-skills.md) · [Skill 원본](skills/odeduck/SKILL.md)

<a id="실행-기반-탐색신청인증호출"></a>

## 활용신청과 데이터 조회

**포털에 한 번 직접 로그인하면, 이후 활용신청·승인 확인·인증키 입력·API 호출은 오데덕이 처리합니다.**
기관 심의가 필요한 API는 승인될 때까지 기다려야 합니다.

<p align="center">
  <img src="docs/assets/odeduck-api-workflow.png" width="800" alt="자료 탐색 → 이용 방법 확인 → 활용신청 → 승인 확인 → 인증키 자동 입력 → 데이터 조회">
</p>

### 1. 포털에 로그인하기

터미널에서 아래 명령을 실행하고, 열린 브라우저에서 data.go.kr 로그인을 완료합니다.

```sh
odeduck login
```

### 2. AI에 신청과 조회 맡기기

AI 대화에서 다음과 같이 요청합니다.

> 방금 찾은 데이터의 내용을 확인해주세요. 활용신청이 필요하면 신청하고, 승인 상태를 확인한 뒤 조회해주세요.

오데덕은 계정 인증키를 재사용하므로 **인증키를 채팅창에 붙여 넣을 필요가 없습니다.**
AI 앱의 설정에 따라 신청서를 제출하기 전에 확인을 요청할 수 있습니다.

온비드·나라장터·공영도매시장·중소기업 지원사업 데이터는 실제 계정으로 신청부터 조회까지 확인했습니다.
[검증 기록](docs/validation-summary.md) · [CLI 신청·조회 명령](docs/advanced-usage.md#활용신청과-첫-호출)

### 자료별 지원 범위

자료를 검색할 수 있는 범위와 자동으로 신청·호출할 수 있는 범위는 다릅니다.

| 자료 종류 | 지원하는 작업 |
| --- | --- |
| data.go.kr에서 직접 제공하는 REST API | 활용신청, 승인 확인, 인증키 입력, API 호출 |
| 파일·표준데이터 | 지원하는 CSV·엑셀 등의 항목과 일부 데이터 확인 |
| 외부 제공기관 자료 | SafetyKorea·FoodSafetyKorea·VWorld 등 지원 기관은 별도 인증키로 조회. 그 외에는 공식 이용 경로 안내 |

<a id="찾은-자료로-비교표-만들기--실험-기능"></a>

## 목표에서 산출물까지: solve

**오데덕은 질문에 필요한 자료를 모아 비교표·보고서와 근거를 만드는 것을 핵심 사용 방식으로 삼고 있습니다.**
터미널에서는 `solve`, MCP로 연결한 AI에서는 `advance_goal`을 사용합니다.

> 이 공매 아파트가 비슷한 거래보다 저렴한지 비교해주세요. 비교 조건과 출처를 적고, 자료가 부족한 물건도 표시해주세요.

오데덕은 목표를 지역·기간·비교 조건과 필요한 자료로 나눕니다. 자료의 항목과 이용 방법을 확인한 뒤,
실제 값을 가져와 계산합니다. 조건이 맞지 않거나 근거가 부족하면 다른 자료나 연결에 필요한 자료를 다시 찾습니다.

| 결과에 포함하는 내용 | 확인할 수 있는 것 |
| --- | --- |
| 비교표·보고 내용·계산한 값 | 요청한 항목과 범위가 채워졌는지 |
| 출처·기간·계산 조건 | 어떤 자료로 어떻게 계산했는지 |
| 비교하지 못한 항목과 부족한 자료 | 아직 판단할 수 없는 부분이 무엇인지 |

<p align="center">
  <img src="docs/assets/odeduck-goal-flow.png" width="800" alt="공매 물건의 최소입찰가와 비교 거래가를 나란히 놓고, 출처·비교 조건·자료가 부족한 물건을 함께 표시한 가상 예시">
</p>

그림의 물건과 금액은 **설명용 가상 예시**이며, 공매 분석을 끝까지 수행한 실제 결과는 아닙니다.

<a id="현재-실행할-수-있는-범위"></a>

### 현재 지원 범위와 준비 사항

- **실행 범위:** API·CSV·ZIP 내부 CSV·XLSX·표준데이터에서 제한된 양의 데이터를 읽어 조회·연결·집계를 수행합니다. 출처와 계산 근거를 기록하고, 별도 모델이 결과를 검토하는 경로도 제공합니다.
- **검증 상태:** 목표 실행은 핵심 개발 기능이며 현재 안정성 표시는 `experimental`입니다. 다양한 목표를 자율적으로 완수하는 능력은 검증 중입니다. 필요한 결과 검토가 끝나지 않으면 미완료로 반환합니다.
- **사용 준비:** 의미 검색·계획 모델·데이터 공유·결과 검토 설정이 필요합니다. `solve` 자체는 활용신청을 자동 제출하지 않으므로, 필요한 API 접근권한은 [신청·조회 경로](#활용신청과-데이터-조회)로 먼저 준비합니다.

[목표 실행 설정과 명령](docs/advanced-usage.md#목표에서-결과까지-실행하기--experimental) · [검증 결과](docs/validation-summary.md) · [개발 목표](INTENT.md)

## 활용 예시

자료 이름을 모두 알지 못해도, 하려는 일과 확인할 내용을 설명하면 탐색을 시작할 수 있습니다.

| 요청 예시 | 함께 찾아볼 자료 |
| --- | --- |
| 카페를 열 지역을 고르고 있습니다. 손님이 올 만한 곳인지 확인할 자료를 찾아주세요. | 생활인구, 업종별 매출, 주변 점포 |
| 우리 회사가 신청할 만한 지원사업을 찾아주세요. | 지원사업 공고, 신청 자격, 접수 기간 |
| 식당 재료비를 줄이고 싶습니다. 최근 도매가격과 반입량 자료를 찾아주세요. | 품목별 도매가격, 반입량, 거래 지역 |

<a id="공매-예시-가격과-위험을-함께-확인"></a>

### 공매 가격을 살피다가 주변 환경까지

부산 공매 부동산의 가격과 위험을 확인할 자료를 요청했을 때, **실거래가·부산 상권 변화·토양오염 조사**가 함께 발견됐습니다.
가격 비교를 시작으로 주변 수요와 환경을 살펴볼 단서를 찾은 사례입니다.

<p align="center">
  <img src="docs/assets/odeduck-unexpected-connections.png" width="800" alt="부산 공매 부동산을 살펴보기 위해 실거래가, 상권 변화, 토양오염 조사 자료를 함께 탐색한 사례">
</p>

함께 발견한 자료를 분석에 사용하려면 물건 종류·지역·기간이 맞는지 확인해야 합니다.
토양 조사 기록만으로 특정 물건이 오염됐다고 판단할 수는 없습니다.
[실제 발견 기록과 출처](docs/research/connection-discovery-evaluation.md#readme-연결-예시--2026-09-12)

<a id="이-흐름을-하나로-연결한-이유"></a>
<a id="왜-이-흐름을-묶었나"></a>

## 전체 아키텍처

오데덕은 **AI가 세운 계획을 실제 데이터 작업으로 연결하는 로컬 실행 기반**입니다.
AI는 필요한 자료와 다음 작업을 제안하고, Go로 작성한 실행 모듈이 신청·인증·호출·계산을 수행합니다.

| 구성 | 역할 |
| --- | --- |
| Agent Skill | 요청에 맞는 사용 절차와 필요한 설정을 안내합니다. |
| MCP / CLI | AI 앱이나 터미널의 요청을 같은 실행 엔진으로 전달합니다. |
| `solve` / `advance_goal` | 목표에 필요한 자료를 찾고, 수집·계산·재탐색과 결과 검토를 진행합니다. |
| Go 실행 모듈 | 실제 API·파일의 규격을 확인하고, 인증정보 처리·신청·호출·계산·실행 검증을 담당합니다. |

<p align="center">
  <img src="docs/assets/odeduck-system-overview.png" width="800" alt="CLI와 MCP가 같은 실행 엔진을 사용하며, 자료 탐색과 검사에서 신청·인증·조회로 이어지는 전체 구조">
</p>

인증키는 실행 모듈 내부에서 사용하며 모델에 전달하지 않습니다.
결과는 실제로 가져온 데이터와 계산 기록을 바탕으로 확인합니다. 자료의 의미와 질문에 충분히 답했는지는 별도로 검토하며,
모델의 검토가 현장 검증을 대신하지는 않습니다.

이 구조를 기술 문서에서는 *local control plane*이라고 부릅니다.
`solve`의 CLI 계획기는 Codex·Claude·Gemini를 지원합니다.
[상세 아키텍처](ARCHITECTURE.md) · [실행·검증 계획](docs/specs/goal-driven-completion-plan.md) · [공개 도구와의 비교](docs/research/competitive-workflow-audit.md)

## 로그인 정보와 데이터 공유

로그인 정보와 인증키는 사용자 컴퓨터에 저장됩니다.
AI에 연결하면 질문과 조회 결과는 사용하는 AI 앱에 전달되므로, 공유할 내용과 자료별 이용조건을 확인해주세요.

저장된 로그인 정보와 인증키는 다음 명령으로 삭제할 수 있습니다.

```sh
odeduck logout
```

[로그인과 데이터 취급 안내](docs/advanced-usage.md#인증과-데이터-취급)

<a id="질문오류-제보"></a>

## 문서와 문의

| 필요한 정보 | 안내 |
| --- | --- |
| 명령과 상세 설정 | [상세 사용법](docs/advanced-usage.md) |
| Skill 설치와 업데이트 | [Agent Skill 안내](docs/agent-skills.md) |
| 확인한 기능과 남은 과제 | [검증 결과](docs/validation-summary.md) · [업데이트 내역](CHANGELOG.md) |
| 기여와 보안 제보 | [기여 안내](CONTRIBUTING.md) · [보안 제보](SECURITY.md) |

사용법은 [질문 게시판](https://github.com/JungHoonGhae/odeduck/discussions)에,
오류는 [이슈](https://github.com/JungHoonGhae/odeduck/issues)에 남겨주세요.
찾으려던 자료와 오류 메시지를 함께 적어주시고, 인증키와 로그인 정보는 제외해주세요.

## 라이선스

[MIT](LICENSE)
