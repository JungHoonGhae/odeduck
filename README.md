<!-- brand:start -->
<p align="center">
  <img src="docs/assets/brand-symbol.svg" width="190" alt="흩어진 공공데이터의 연결을 찾는 오데덕 캐릭터">
</p>

<h1 align="center">오데덕</h1>

<p align="center"><em>오픈데이터 덕후, 오데덕.</em></p>
<p align="center">질문에서 서로 먼 데이터를 찾고, 활용신청부터 데이터 조회까지 이어갑니다.</p>
<!-- brand:end -->

<!-- Keep the introduction animation visible. Core capabilities and architecture must also remain visible, not collapsed. -->
<p align="center">
  <a href="docs/assets/odeduck-hero.mp4"><img src="docs/assets/odeduck-hero.gif" width="800" alt="안경을 쓰지 않은 아주 작은 오데덕이 거대한 캐비닛 사이를 뛰어다니며 서로 떨어진 공공데이터를 찾아 하나의 연결망으로 잇는 애니메이션"></a>
</p>
<p align="center">
  <sub>서로 상관없어 보이는 조각도, 함께 보면 질문의 나머지가 된다. · <a href="docs/assets/odeduck-hero.mp4">원본 영상</a></sub>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/works%20with-Codex%20%C2%B7%20Claude%20%C2%B7%20Gemini%20%C2%B7%20Cursor-111111?style=flat-square" alt="Works with Codex, Claude, Gemini, and Cursor">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-111111?style=flat-square" alt="MIT license"></a>
</p>

<p align="center">
  <strong>공공데이터 96,000+개 &middot; 활용신청부터 데이터 조회까지</strong><br>
  <sub>data.go.kr의 API·파일·외부 제공기관 자료를 함께 찾아봅니다.</sub>
</p>

---

**어떤 데이터를 찾아야 할지 막막할 때, 질문부터 건네세요.**
오데덕은 Codex·Claude·Gemini·Cursor에 연결해 쓰는 공공데이터 도구입니다.
서로 다른 분야의 자료를 찾고, 필요한 데이터의 활용신청과 조회까지 이어줍니다.

[할 수 있는 일](#이런-일에-써보세요) · [빠른 시작](#빠른-시작) · [신청·조회](#활용신청과-데이터-조회) · [전체 구조](#전체-아키텍처)

## 이런 일에 써보세요

필요한 자료의 정확한 이름을 몰라도 됩니다. AI에 찾고 싶은 것을 말해보세요.

| 이렇게 물어보세요 | 함께 찾아볼 자료 |
| --- | --- |
| “동네 카페 후보를 비교하려면 어떤 자료를 봐야 할까?” | 생활인구, 업종별 매출, 교통 접근성 |
| “폭염 때 어르신이 쉴 곳을 살펴보고 싶어.” | 고령 인구, 쉼터 위치와 운영시간, 주변 교통 |
| “식재료 가격 변화를 살펴볼 자료를 찾아줘.” | 가격, 반입량, 작황, 날씨 |

오데덕은 약 9.6만 건의 데이터 목록에서 후보를 찾고, 실제로 어떤 항목을 제공하는지 확인합니다.
검색 결과에는 자료의 출처가 함께 나옵니다. 여러 자료를 연결해 분석하는 기능은 [실험 단계](#데이터를-연결해-분석하기--실험-기능)입니다.

## 탐색부터 신청·호출까지

![질문에 맞는 데이터 탐색과 내용 확인, 활용신청, 승인 확인, 인증키 자동 입력, 데이터 조회로 이어지는 흐름. 사람은 data.go.kr에 한 번 로그인합니다.](docs/assets/odeduck-api-workflow.png)

데이터를 찾은 뒤 매번 포털에 들어가 신청하고 인증키를 복사할 필요가 없습니다.
**data.go.kr에 한 번 로그인하면, AI가 필요한 활용신청 → 승인 확인 → 인증키 입력 → 데이터 조회를 이어갑니다.**
사용하는 AI 앱의 실행 승인 설정에 따라 제출 전에 확인을 요청할 수 있습니다.
기관의 심의가 필요한 데이터는 승인을 기다려야 합니다.

온비드·나라장터·공영도매시장·중소기업 지원사업 데이터는 실제 계정으로 신청과 조회까지 확인했습니다.
[확인한 범위](docs/validation-summary.md)

## 빠른 시작

**설치 → 첫 검색 → AI 연결 → 질문**, 네 단계로 시작합니다.
터미널에 아래 명령을 차례로 입력하세요. 이미 설치했다면 [2번](#2-첫-검색-해보기)부터 진행하면 됩니다.

### 1. 설치하기

macOS·Linux:

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/download/v0.19.0/install.sh | sh
```

<details>
<summary>Windows 설치 명령</summary>

```powershell
irm https://github.com/JungHoonGhae/odeduck/releases/download/v0.19.0/install.ps1 | iex
```

</details>

검색에 필요한 데이터 목록도 함께 설치됩니다.

### 2. 첫 검색 해보기

로그인이나 API 키 없이 바로 실행할 수 있습니다. AI를 연결하지 않아도 됩니다.

```sh
odeduck catalog search "건축물" --limit 5 --semantic=false -f table
```

**데이터 목록이 나오면 첫 검색 성공입니다.** `건축물`을 원하는 검색어로 바꿔보세요.
질문으로 자료를 찾으려면 다음 단계에서 AI를 연결합니다.

### 3. 사용하는 AI에 연결하기

Codex를 사용한다면:

```sh
codex mcp add odeduck -- odeduck mcp
```

<details>
<summary>Claude·Gemini 연결 명령</summary>

사용하는 AI의 명령 하나만 실행하세요.

```sh
# Claude
claude mcp add odeduck -- odeduck mcp

# Gemini
gemini mcp add --scope user odeduck odeduck mcp
```

</details>

<details>
<summary>Cursor·Claude Desktop 연결 설정</summary>

앱의 MCP 설정에 아래 내용을 추가하세요. MCP는 AI 앱에 오데덕 같은 도구를 연결하는 방식입니다.

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

앱의 MCP 도구 목록에 `odeduck`이 보이면 연결된 상태입니다. Codex에서는 `codex mcp list`로 확인할 수 있습니다.

### 4. 질문 건네기

AI 앱에서 새 대화를 열고 아래 질문을 붙여 넣으세요. 연결이 보이지 않으면 앱을 재시작하세요.

> 오데덕으로 장마철에도 매출이 덜 흔들릴 동네 카페 후보를 살펴보고 싶어. 어떤 데이터를 같이 봐야 하는지 찾아줘.

AI가 검색어를 정하고 자료를 살펴본 뒤, **함께 볼 데이터 후보와 출처**를 제시합니다.
마음에 드는 후보가 있으면 지역이나 기간을 더 구체적으로 말해보세요.

## 활용신청과 데이터 조회

검색한 데이터의 실제 값을 조회하려면, 먼저 터미널에서 로그인합니다.
열린 브라우저에서 data.go.kr 로그인을 직접 마치세요.

```sh
odeduck login
```

이후 AI에 이어서 요청하세요.

> 방금 찾은 데이터의 내용을 확인해줘. 활용신청이 필요하면 신청하고, 승인 상태를 확인한 뒤 조회해줘.

인증키는 오데덕이 자동으로 입력합니다. 채팅창에 인증키를 붙여 넣을 필요가 없습니다.
자동 신청은 data.go.kr에서 직접 제공하는 REST API를 지원합니다.

| 자료 종류 | 오데덕이 하는 일 |
| --- | --- |
| data.go.kr API | 필요한 활용신청, 승인 확인, 인증키 입력과 조회 |
| 파일·표준데이터 | 지원되는 CSV·엑셀 등의 항목과 일부 데이터를 확인 |
| 외부 제공기관의 데이터 | SafetyKorea·FoodSafetyKorea·VWorld 등 지원 기관은 별도 인증키로 조회. 미지원 기관은 공식 이용 경로 안내 |

기관 승인을 기다리는 동안에는 바로 조회되지 않을 수 있습니다.
직접 명령으로 신청하거나 조회하려면 [상세 사용법](docs/advanced-usage.md#활용신청과-첫-호출)을 참고하세요.

## 전체 아키텍처

![터미널에서 직접 사용하거나 AI 앱에 연결해 같은 오데덕을 이용합니다. 데이터를 찾고 내용을 확인한 뒤, 파일은 읽고 API는 필요한 신청과 인증을 거쳐 조회합니다.](docs/assets/odeduck-system-overview.png)

터미널에서 직접 명령하는 방식이 **CLI**, AI 앱에 연결하는 방식이 **MCP**입니다.
어느 쪽이든 같은 오데덕이 데이터 탐색과 내용 확인, 신청·인증·조회를 맡습니다.
파일은 읽을 수 있는 내용을 확인하고, API는 필요한 신청을 거쳐 조회합니다.

## 데이터를 연결해 분석하기 — 실험 기능

찾은 자료를 연결하고 계산해, 질문의 답과 출처까지 얻는 기능을 실험하고 있습니다.
근거가 부족하면 다른 자료를 찾고, 해결하지 못한 부분은 미완료로 남깁니다.

![데이터를 찾고 확인한 뒤 연결·계산하고 근거를 검토합니다. 부족하면 다시 탐색하며, 결과와 출처 또는 미완료 사유를 남깁니다.](docs/assets/odeduck-goal-flow.png)

아직 모든 질문에 대해 분석을 끝내는 기능은 아닙니다. 결과를 읽을 때는 사용한 자료의 지역·기간과 빠진 항목을 함께 확인하세요.
이 실험 기능은 자동 활용신청을 하지 않으며, AI 이용 비용과 추가 설정이 필요합니다.
시도해보려면 [실험 기능 사용법](docs/advanced-usage.md#목표에서-결과까지-실행하기--experimental)을 확인하세요.

## 로그인과 데이터 공유

로그인 정보와 인증키는 내 컴퓨터에 저장됩니다. AI에 연결하면 질문과 조회 결과를 사용하는 AI 앱이 받습니다.
공유하면 안 되는 내용을 질문에 넣지 말고, 자료별 이용조건을 확인하세요.

저장된 로그인 정보와 인증키를 지우려면:

```sh
odeduck logout
```

자세한 저장·공유 범위는 [로그인과 데이터 취급 안내](docs/advanced-usage.md#인증과-데이터-취급)에 있습니다.

## 도움이 필요하면

사용법은 [질문 게시판](https://github.com/JungHoonGhae/odeduck/discussions)에,
오류는 [이슈](https://github.com/JungHoonGhae/odeduck/issues)에 남겨주세요.
찾으려던 자료와 나온 오류 메시지를 함께 적으면 문제를 확인하기 쉽습니다. 인증키나 로그인 정보는 빼주세요.

[상세 사용법](docs/advanced-usage.md) · [업데이트 내역](CHANGELOG.md) · [기여 안내](CONTRIBUTING.md) · [보안 제보](SECURITY.md)

## 라이선스

[MIT](LICENSE).
