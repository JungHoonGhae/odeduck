<!-- brand:start -->
<p align="center">
  <img src="docs/assets/brand-symbol.svg" width="190" alt="흩어진 공공데이터의 연결을 찾는 오데덕 캐릭터">
</p>

<h1 align="center">오데덕</h1>

<p align="center"><em>오픈데이터 덕후, 오데덕.</em></p>
<p align="center">서로 상관없어 보이는 공공데이터를 연결하고 질문에 필요한 정보를 찾아온다.</p>
<!-- brand:end -->

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
  <strong>데이터 96,000+개 &middot; 실계정 전체 흐름 4종 &middot; 실호출 점검 11개</strong><br>
  <sub>REST·LINK·FILE을 함께 찾고 실제 명세와 파일을 검사해 활용신청부터 첫 호출까지 잇는다.</sub>
</p>

---

양말 한 짝이 사라지면 서랍만 뒤져서는 찾기 어렵다. 세탁기 뒤, 소파 밑, 어제 입은 바지까지 생활의
맥락을 따라가야 한다. 데이터도 같다. 인구에 관한 질문의 나머지 한 짝이 상권이나 교통 데이터에 있을
수 있다.

**오데덕은 공공데이터에서 그 짝을 찾는다.** 질문을 여러 분야로 나누고 함께 볼 데이터를 찾은 뒤 실제
명세와 컬럼으로 확인한다. 연결 근거가 부족하면 정답처럼 꾸미지 않고 후보에서 멈춘다.

스티브 잡스가 [2005년 스탠퍼드 졸업 연설](https://news.stanford.edu/stories/2005/06/youve-got-find-love-jobs-says)에서
캘리그래피와 매킨토시 타이포그래피를 두고 말한 `connecting the dots`처럼, 당시에는 상관없어 보이는
점도 나중에는 하나의 맥락이 될 수 있다. 오데덕은 그 관점을 공공데이터 탐색에 적용한다.

## 기존 도구는 어디에서 멈췄나

data.go.kr 관련 공개 도구 10개를 실제 도구 등록과 호출 코드로 비교했다. 검색→상세→호출을 지원하는
도구는 있었지만 활용신청 제출과 승인 확인까지 처리한 구현은 없었다.

<p align="center">
  <img src="docs/assets/odeduck-before-after.svg" width="900" alt="감사한 기존 도구들은 키워드, FTS, 고정 도메인처럼 서로 다른 탐색과 호출 범위를 가졌지만 활용신청과 승인 확인은 모두 사람에게 돌려보냈다. odeduck은 질문을 인구, 매출, 점포, 위험 같은 분야별 축으로 나눠 발견하고 실제 명세와 컬럼 검사, data.go.kr REST 활용신청, 승인과 키 확인, 인증 호출까지 잇는다.">
</p>

오데덕은 **다른 분야의 데이터를 한 질문의 후보로 모으고 실제 명세와 파일을 검사한 뒤 기존 도구가
사람에게 돌려보내던 신청·승인·키·호출까지 잇는다.** 비교 대상과 판정 근거는
[경쟁 워크플로 감사](docs/research/competitive-workflow-audit.md)에 공개했다.

## 질문은 하나인데, 답은 한 분야에 있지 않다

이 질문에 답하고 싶다고 해보자.

> 서울에서 작은 가게를 열고 싶어. 사람들이 늘기 시작한 동네와 덜 붐비는 업종을 검토할 자료를 찾아줘.

### 직접 찾으면

공공데이터포털에서는 생활인구·매출·점포·개폐업·공실을 따로 검색하고 상세페이지마다 파일과 API를
확인해야 한다. API라면 활용신청서를 쓰고 승인과 키도 기다린다.

### 오데덕에게 맡기면

```text
"서울에서 작은 가게를 열고 싶어. 사람들이 늘기 시작한 동네와 덜 붐비는 업종을 검토할 자료를 찾아줘."

catalog_search → inspect_dataset ─┬─ FILE: 실제 파일과 컬럼 관찰
                                  ├─ REST: (apply) → call_api
                                  └─ LINK: 검증된 adapter 또는 공식 경로 안내
```

AI가 질문을 생활인구·업종별 매출·점포 생존·개폐업으로 나누면, 오데덕은 약 9.6만 개의 API·파일·외부
링크에서 후보를 찾고 실제 명세와 컬럼을 검사한다. REST는 필요할 때 활용신청과 호출까지 이어 주고
검증되지 않은 외부 제공기관은 공식 경로를 안내한 뒤 멈춘다.

성공할 가게를 대신 골라주지는 않는다. 어떤 데이터로 아이디어를 검토할 수 있는지, 어디까지 연결해
확인했는지를 보여준다.

## 빠른 시작

릴리스에는 검증된 카탈로그가 포함되어 있어 로그인이나 API 키 없이 바로 검색할 수 있다.

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/download/v0.18.0/install.sh | sh

odeduck catalog search \
  "서울에서 작은 가게 후보를 좁힐 자료" \
  --concept 생활인구 \
  --concept 추정매출 \
  --concept "상권 점포" \
  --concept "상권 개폐업" \
  --limit 8 --semantic=false -f table
```

이 검색은 로컬에서만 실행된다. 고른 데이터는 `odeduck inspect <PK> --observe`로 실제 명세와 컬럼을
확인한다. 활용신청과 호출이 필요할 때만 `odeduck login`으로 data.go.kr에 한 번 로그인한다.

검색축을 직접 적는 대신 설치된 Codex·Claude·Gemini·Cursor에 질문을 나눠 달라고 할 수도 있다.

```sh
odeduck catalog discover \
  "장마철에도 매출이 덜 흔들릴 동네 카페 후보를 찾고 싶어. 어떤 데이터를 같이 봐야 하는지 찾아줘." \
  --connections --limit 12 --semantic=false -f table
```

실행 결과에는 분야별 후보, 예상 결합키, 검증 전 한계가 함께 표시된다. 의미 검색이 반드시 필요하면
로컬 Ollama 인덱스를 만든 뒤 `--require-semantic`을 사용한다. 인덱스나 Ollama에 문제가 있으면 일반
검색으로 조용히 대체하지 않고 실패한다.

<p align="center">
  <img src="docs/assets/odeduck-linkedin-demo.svg" width="900" alt="실제 catalog discover 실행에서 강수 비교군, 시간대별 유동인구, 침수 기록을 서로 다른 연결 후보로 찾고 각 데이터의 PK와 예상 결합키, 검증 전 한계를 표시한 결과">
</p>
<p align="center">
  <sub>v0.16.0 · 2026-09-03 · <code>catalog discover --connections</code> · data.go.kr 로그인·API 키·Ollama 없이 실행 · <a href="docs/research/launch-readiness-linkedin-geeknews-2026-09.md">실행 조건과 후보 PK</a></sub>
</p>

## 검증 수치

그럴듯한 데모 대신 실제 계정과 실제 포털에서 확인했다.

| 확인한 것 | 결과 |
| --- | --- |
| 통합 카탈로그 | 2026-09-02 릴리스 기준 96,683개 노드. REST·LINK·FILE과 복수 제공형 보존 |
| 실계정 전체 흐름 | 온비드 공매·나라장터 입찰·공영도매시장 경매·중소기업 지원사업 데이터 4종의 활용신청→승인→호출 |
| 외부 제공기관 | SafetyKorea·FoodSafetyKorea·VWorld의 형식이 고정된 호출, 서울 열린데이터광장 계약 검사 |
| 변경 감시 | 제공기관 어댑터 4개, 실호출 점검 11개, 주간 CI |
| 실패 경계 | 미지원 LINK·불안전한 전송·불충분한 결합 근거에서 멈춤 |

비교 방법과 근거는 [경쟁 워크플로 조사](docs/research/competitive-workflow-audit.md)에 있다.

## 동작 방식

오데덕은 검색 1위를 정답이라고 부르지 않는다. 다음 단계를 차례로 밟는다.

```text
1. 무슨 데이터가 필요한가?     → 질문을 서로 다른 역할의 검색축으로 나눈다
2. 이름이 틀렸을 수 있나?       → 96,000+개 통합 카탈로그를 함께 뒤진다
3. 정말 쓸 수 있나?             → API 명세와 FILE의 실제 컬럼을 본다
4. 권한이 필요한가?              → 활용신청·승인·provider 키를 처리한다
5. 서로 연결되는가?              → 필드·범위·값 교집합을 확인한다
6. 근거가 부족한가?              → 억지로 엮지 않고 멈춘다
```

탐색은 넓게 한다. 주장은 좁게 한다.

data.go.kr REST는 정부 SSO 로그인 한 번 뒤 신청→승인 확인→키 획득→호출을 잇는다. 자동승인 후
게이트웨이 반영이 늦으면 `call --wait 10m`이 새 키를 만들거나 재신청하지 않고 기다린다.

자연어 검색은 데이터를 발견하는 장치이지, 상관관계나 인과관계를 자동으로 증명하는 장치가 아니다.
전체 계약은 [교차 데이터 연결 발견 명세](docs/specs/cross-domain-connection-discovery-v1.md), 실제 평가는
[연결 발견 평가](docs/research/connection-discovery-evaluation.md)에 있다.

검증한 연결은 MCP의 `record_connection_assessment`로 로컬 append-only 장부에 남길 수 있다. 장부에는
공식 출처, field namespace와 grain, 요청·프로필 해시, 표본 집계만 저장하고 API 응답 원문이나 인증정보는
저장하지 않는다. `sample_verified`는 같은 MCP 세션에서 최근 `call_api`가 만든 두 단일-key profile과
집계가 정확히 일치할 때만 허용되며, 근거가 부족하면 `structurally_verified`, `blocked` 또는 `rejected`로
남는다. 자세한 저장·판정 계약은 [연결 근거 장부 명세](docs/specs/connection-evidence-ledger-v1.md)를 본다.

구성요소, 데이터 흐름, 로컬 상태와 신뢰 경계는 [아키텍처 문서](ARCHITECTURE.md)에 정리했다.

## 설치와 에이전트 연결

Windows:

```powershell
irm https://github.com/JungHoonGhae/odeduck/releases/download/v0.18.0/install.ps1 | iex
```

macOS·Linux 설치 명령은 위 빠른 시작에 있다. 설치 스크립트는 checksum을 검증하고 같은 릴리스의
카탈로그 스냅샷을 함께 설치한다. Go 1.26.6 이상에서는 소스로 설치할 수도 있다.

```sh
go install github.com/JungHoonGhae/odeduck/cmd/odeduck@latest
odeduck catalog sync
```

소스 설치에는 릴리스의 prebuilt 카탈로그가 포함되지 않으므로 첫 검색 전에 한 번 동기화한다.

### AI 에이전트에 연결

```sh
codex mcp add odeduck -- odeduck mcp
claude mcp add odeduck -- odeduck mcp
gemini mcp add --scope user odeduck odeduck mcp
```

Cursor·Claude Desktop처럼 JSON 설정을 쓰는 호스트:

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

등록 뒤에는 새 세션을 열거나 클라이언트를 재시작하고 `/mcp` 또는 `codex mcp list`에서 `odeduck`이
활성화됐는지 확인한다.

이제 데이터 이름 대신 궁금한 것을 말하면 된다.

```text
서울에서 새 가게 후보를 좁힐 생활인구·매출·점포·폐업 자료를 함께 찾아줘.
식재료 원가 변화를 검토할 가격·반입량·작황 자료를 함께 찾아줘.
빈집과 방문객 변화로 지역 사업을 검토할 인구이동·관광·소비 자료를 찾아줘.
```

## CLI

```sh
# 브라우저가 한 번 열린다. data.go.kr에 로그인한다.
odeduck login

# 정확한 데이터 이름을 몰라도 된다.
odeduck catalog discover "지역 소멸로 생길 사업 기회"

# API 명세 또는 FILE의 실제 자산·컬럼을 확인한다.
odeduck inspect <PK> --observe

# API가 필요하면 활용신청한다. CLI는 제출 전 y/N을 묻는다.
odeduck apply <PK> --purpose "지역 사업 기회 분석" --category research

# 엔드포인트와 serviceKey를 직접 다루지 않고 호출한다.
odeduck call --pk <PK> --param numOfRows=5
```

## 다 같은 데이터가 아니다

| 유형 | odeduck이 하는 일 |
| --- | --- |
| `REST` | 포털의 공식 operation과 필수 파라미터를 확인하고 신청·호출 |
| `LINK` | SafetyKorea·FoodSafetyKorea·VWorld 등 검증된 adapter만 typed 호출 |
| `FILE` | 다운로드 링크만 보여주지 않고 CSV·DBF·XLSX의 작은 표본과 실제 스키마를 관찰 |

API처럼 보이는 링크라고 엔드포인트를 지어내지 않는다. 파일이라고 사람에게 다운로드를 떠넘기지도 않는다.
검증된 계약이 없는 제공기관은 공식 경로를 알려 주고 멈춘다.

## 마법은 아니다

- 정부 SSO 로그인과 심의승인을 우회하지 않는다.
- 모든 LINK 제공기관을 하나의 키로 부르지 않는다.
- 연결 후보는 실제 필드·범위·값 교집합을 확인하기 전까지 후보다.
- 활용신청 자동화는 포털 HTML에 의존한다. 포털이 바뀌면 깨질 수 있고 `odeduck doctor`와 실호출 점검이 이를 감시한다.
- data.go.kr 밖의 이용조건은 각 제공기관 정책을 따른다.

## 보안

`odeduck login`은 로그인이 끝나면 브라우저를 닫고 세션을 운영체제의 사용자 설정 디렉터리에 저장한다.
인증키는 MCP 입력·출력이나 로그에 내보내지 않고 호출 직전에만 넣는다. `odeduck logout`은 세션,
data.go.kr 키, provider 키와 Chrome 프로파일을 지운다.

세션과 키는 파일 권한으로 접근을 막지만 암호화되지는 않는다. 공용 머신에서는 쓰지 않는 편이 좋다.
MCP의 `apply`는 확인 대화 없이 실제 활용신청을 만들 수 있다. 직접 확인하고 싶다면 제출 전 `y/N`을
묻는 CLI를 쓰면 된다.

`catalog discover`는 선택된 Codex·Claude·Gemini·Cursor에 자연어 목표를 전달해 검색 계획을 만든다.
로그인 토큰은 읽거나 저장하지 않지만 목표 자체는 해당 AI 제공자에게 전송되므로 개인정보나 미공개
사업계획은 넣지 않는다. Cursor는 격리된 초기 검색 계획에만 사용하고 실제 카탈로그 메타데이터는 보내지
않는다.

보안 경계와 남는 위험은 [ADR](docs/adr/)과 [provider adapter guide](docs/provider-adapters.md)에 있다.

## 개발

새 제공기관 어댑터, 재현 가능한 교차 데이터 질문, 실패 사례를 환영한다. 어댑터는 공식 문서,
고정된 인증 범위, 계약 테스트와 실호출 점검이 있어야 호출 가능 상태가 된다.

```sh
go test ./...
go vet ./...
go build ./...
```

전체 구조는 [아키텍처 문서](ARCHITECTURE.md), 구현 규칙은
[provider adapter guide](docs/provider-adapters.md), 설계 결정은 [ADR](docs/adr/)에서 시작한다.
처음 기여한다면 [기여 가이드](CONTRIBUTING.md)를, 보안 문제라면 공개 이슈를 만들기 전에
[보안 정책](SECURITY.md)을 먼저 읽는다.
사용법 질문과 재현 가능한 공개데이터 질문은 [Discussions](https://github.com/JungHoonGhae/odeduck/discussions)에
남길 수 있다.

공개 이름·문구·로고 경로는 [`docs/brand/brand.json`](docs/brand/brand.json)이 기준이다. 값을 바꾼 뒤
`go run ./scripts/sync-brand.go`를 실행하면 위 브랜드 블록이 갱신되고 CI는 `--check`로 동기화를 확인한다.

## 라이선스

[MIT](LICENSE). 데이터의 짝을 찾는 데 가장 짧은 라이선스.
