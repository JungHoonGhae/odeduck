> 이 README는 오데덕이 직접 안내하는 설정이라, 일부러 반말로 썼습니다. 편하게 읽어주세요.

<!-- brand:start -->
<p align="center">
  <img src="docs/assets/brand-symbol.svg" width="190" alt="흩어진 공공데이터의 연결을 찾는 오데덕 캐릭터">
</p>

<h1 align="center">오데덕</h1>

<p align="center">
  공공데이터는 열려 있습니다.<br>
  찾고, 신청하고, 가져오는 건<br>
  이 덕후가 합니다.
</p>

<p align="center"><strong>공공데이터 덕후, 오.데.덕.</strong></p>
<!-- brand:end -->

<!-- Keep the introduction animation visible. Core capabilities and architecture must also remain visible, not collapsed. -->
<p align="center">
  <a href="docs/assets/odeduck-hero.mp4"><img src="docs/assets/odeduck-hero.gif" width="800" alt="거대한 캐비닛 사이를 뛰어다니는 오데덕. 좌측 하단에 투명한 빼꼼 로고, 궁서체 오.데.덕., GitHub 주소가 세로로 배치되어 있다."></a>
</p>
<p align="center">
  <sub><a href="docs/assets/odeduck-hero.mp4">영상 다운로드</a> · <a href="docs/assets/odeduck-hero-brand.svg">투명 벡터 로고</a></sub>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/works%20with-Codex%20%C2%B7%20Claude%20%C2%B7%20Gemini%20%C2%B7%20Cursor-111111?style=flat-square" alt="Works with Codex, Claude, Gemini, and Cursor">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-111111?style=flat-square" alt="MIT license"></a>
</p>

<p align="center">
  <strong>공공데이터 96,000+개 &middot; 활용신청부터 데이터 조회까지</strong><br>
  <sub>data.go.kr에 등록된 API·파일·외부 제공기관 자료를 함께 찾아봐.</sub>
</p>

[빠른 시작](#빠른-시작) · [자동 신청·조회](#활용신청과-데이터-조회) · [활용 예시](#활용-예시) · [전체 구조](#전체-아키텍처)

---

## 공공데이터를 쓰기 어려운 이유

자료 하나 쓰려는데 검색어부터 알아야 해. 찾았으면 API마다 신청하고,
승인을 확인하고, 인증키와 호출 방법도 챙겨야 해. 자료가 더 필요하면 또 반복이야.

**데이터는 열어놨는데, 쓰기 전 준비는 전부 내 몫이네.**

![공공데이터 이용 전반의 병목: 검색어를 바꿔가며 자료 찾기, 필요한 API마다 신청과 승인 확인 반복, 인증키 입력 방식과 호출 직접 설정.](docs/assets/odeduck-manual-bottlenecks.png)

파일로만 있는 자료는 따로 열어봐야 하고, 있는 줄도 모르는 자료는 검색할 생각조차 못 하잖아.
이렇게 필요한 자료를 찾고, 확인하고, 가져오는 일을 **오데덕한테 맡기면 돼.**

## 오데덕이 맡는 탐색·신청·조회

**하려는 일을 말하면, AI와 오데덕이 필요한 자료를 찾고 API 신청부터 실제 조회까지 이어가.**
Codex·Claude·Gemini·Cursor에 연결해서 쓸 수 있어.

![데이터 탐색 → 내용 확인 → 활용신청 → 승인 확인 → 인증키 자동 입력 → 데이터 조회. 사람은 data.go.kr에 한 번 직접 로그인.](docs/assets/odeduck-api-workflow.png)

포털 로그인은 직접 한 번 해줘. 그 뒤 오데덕이 필요한 API를 각각 확인하고,
미신청 API는 신청한 뒤 승인된 데이터를 가져와. 인증키도 알아서 넣어.

AI 앱의 설정에 따라 신청서 제출 전에 확인을 요청할 수 있고, 기관 심의가 필요한 API는 승인을 기다려야 해.
온비드·나라장터·공영도매시장·중소기업 지원사업 데이터는 [실제 계정으로 신청·조회까지 확인했어](docs/validation-summary.md).

## 다른 MCP·CLI와의 차이

MCP는 AI 앱에 도구를 연결하는 방식이야. 오데덕도 MCP로 연결해.
다른 도구도 검색이나 API 호출을 지원하지만, 활용신청과 인증키 준비는 사용자가 해야 하는 경우가 있어.

“찾아드렸습니다. 신청은 직접 하세요.” 그러면 나 다시 포털 가야 되잖아.
**오데덕은 검색 전 준비부터 신청 후 조회까지, 이어서 맡길 수 있어.**

- **검색어를 고르는 일부터 맡겨.** 하려는 일을 말하면 AI가 자료와 검색어를 골라 찾아봐.

- **목록을 따로 수집할 필요 없어.** 약 9.6만 건의 API·파일 목록이 함께 설치돼. 첫 검색에는 로그인이나 API 키가 필요 없어. 원본 데이터는 필요할 때 확인해.

- **API와 파일의 내용도 확인해.** API의 입력값·제공 항목을 살펴보고, 지원되는 CSV·엑셀 등은 항목과 일부 값을 읽어 필요한 내용이 있는지 확인해.

- **신청부터 조회까지 이어가.** 필요한 활용신청, 승인 확인, 인증키 자동 입력을 거쳐 실제 데이터를 가져와.

같은 단어가 없어도 뜻이 가까운 설명을 찾는 기능은 [추가 설정](docs/advanced-usage.md#cli에서-목표로-탐색하기) 후 쓸 수 있어.

목록 검색이나 파일 구조 확인은 다른 도구에도 있어. 이 일을 어디까지 이어서 처리하는지가 차이야.
[공개 MCP·CLI 6개와 비교한 근거](docs/research/competitive-workflow-audit.md#2026-09-12-재확인)에서 도구별 범위를 볼 수 있어.

## 활용 예시

필요한 데이터의 이름 대신, **무엇을 하려는지** 말해봐.

| 이렇게 맡겨봐 | 함께 찾아볼 자료 |
| --- | --- |
| “카페를 열 자리를 고르고 있어. 손님이 올 만한 곳인지 확인할 자료를 찾아줘.” | 생활인구, 업종별 매출, 주변 점포 |
| “우리 회사가 신청할 만한 지원사업을 찾아줘.” | 지원사업 공고, 신청 자격, 접수 기간 |
| “식당 재료비를 줄이고 싶어. 최근 도매가격과 반입량을 볼 자료를 찾아줘.” | 품목별 도매가격, 반입량, 거래 지역 |

오데덕이 관련 자료를 찾고, 필요한 항목이 있는지 확인해서 **데이터 후보와 출처**를 가져와.
찾은 값을 맞춰 비교표를 만드는 기능은 [실험 단계](#찾은-자료로-비교표-만들기--실험-기능)야.

### 공매 예시: 가격과 위험을 함께 확인

로그인과 AI 연결을 마쳤다면 아래 요청을 붙여 넣어봐.

> 싸게 나온 부산 공매 부동산을 보고 있어. 비교할 가격과 놓치기 쉬운 위험을 찾아줘. 필요한 API는 신청·조회해서 함께 볼 자료와 확인할 점을 정리해줘.

![부산 공매 부동산의 가격과 위험을 살펴보는 예시. 실거래가, 상권 변화, 토양오염 조사를 후보로 탐색하고, 필요한 API의 신청·승인 확인·인증키 자동 입력·조회까지 이어가는 흐름.](docs/assets/odeduck-unexpected-connections.png)

이 질문으로 실제로 찾아보니 **실거래가, 부산 상권 변화, 토양오염 조사**가 후보로 나왔어.
가격을 비교하려다 주변 수요와 환경을 살펴볼 단서까지 찾은 거야. 자료 이름을 미리 전부 알 필요는 없어.

함께 찾은 자료라도 물건 종류·지역·기간이 맞는지 확인해야 해.
토양 조사 기록만으로 특정 물건이 오염됐다고 단정할 수는 없어.
[실제 발견 기록과 원천 확인](docs/research/connection-discovery-evaluation.md#readme-연결-예시--2026-09-12)에 근거를 남겨뒀어.

## 빠른 시작

**먼저 터미널에서 설치 명령을 실행해.** 이미 설치했으면 [2번](#2-첫-검색-해보기)부터 시작해.
아래 네 단계를 마치면 AI에 자료 찾기를 맡길 수 있어.

### 1. 설치하기

macOS·Linux라면 이 명령을 실행해.

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/download/v0.19.0/install.sh | sh
```

<details>
<summary>Windows 설치 명령</summary>

```powershell
irm https://github.com/JungHoonGhae/odeduck/releases/download/v0.19.0/install.ps1 | iex
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

### 4. AI에 자료 찾기 맡기기

AI 앱에서 새 대화를 열고 아래 문장을 붙여 넣어. 연결이 안 보이면 앱을 한 번 재시작해.

> 오데덕, 우리 회사가 신청할 만한 지원사업을 찾고 있어. 신청 자격과 접수 기간을 확인할 자료를 찾아줘.

**데이터 후보와 출처가 나오면 첫 탐색 완료.** 실제 값을 가져오려면 아래 로그인 단계로 이어가.

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

**터미널에서 쓰는 CLI와 AI 앱에 연결하는 MCP는 같은 엔진을 사용해.**

![CLI와 MCP가 같은 오데덕 엔진을 사용하는 전체 구조. 공통 데이터 탐색과 내용 확인 후, 파일은 읽고 API는 필요한 신청과 인증을 거쳐 조회하는 흐름.](docs/assets/odeduck-system-overview.png)

공통 탐색과 내용 확인을 거친 뒤, 파일은 지원되는 내용을 읽고 API는 필요한 신청·인증을 거쳐 조회해.

## 찾은 자료로 비교표 만들기 — 실험 기능

**여러 자료의 값을 맞춰 비교표와 설명을 만드는 기능이야.** 현재는 지원하는 자료와 계산 범위 안에서 실행해.

> 이 공매 아파트, 비슷한 거래보다 싼지 비교해줘.

이 요청이라면 단지·면적·거래 시점이 맞는 실거래 기록을 찾아 공매 물건과 비교해.

![설명용 가상 예시: 공매 물건의 최소입찰가와 비교할 실거래가를 나란히 놓은 표. 물건 A는 3억 원과 3.4억 원의 비교 자료가 있고, 물건 B는 자료 부족으로 빈칸과 이유를 표시. 출처와 비교 조건도 함께 제공.](docs/assets/odeduck-goal-flow.png)

그림의 물건과 금액은 **설명용 가상 예시**야. 공매 분석을 끝까지 수행한 실제 결과는 아니야.

- **자료가 부족하면:** 다른 자료를 찾고, 끝내 확인하지 못한 항목은 이유와 함께 미완료로 남겨.
- **사용하려면:** AI 이용 비용과 추가 설정이 필요해.
- **자동 신청은 제외:** 앞의 일반 MCP 신청·조회 흐름과 달리, 이 실험 기능은 활용신청을 자동으로 제출하지 않아.

[실험 기능 사용법](docs/advanced-usage.md#목표에서-결과까지-실행하기--experimental)에서 준비 방법을 확인해.

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
