<!-- brand:start -->
<p align="center">
  <img src="docs/assets/brand-symbol.svg" width="190" alt="문틈에서 조용히 얼굴을 내민 oddsock 캐릭터">
</p>

<h1 align="center">oddsock</h1>

<p align="center"><em>말은 없다. 질문 하나를 던진다. 데이터와 함께 돌아온다.</em></p>
<p align="center">공개돼 있었다. 찾기 쉽다는 뜻은 아니었다.</p>
<!-- brand:end -->

<p align="center">
  <img src="https://img.shields.io/badge/works%20with-Codex%20%C2%B7%20Claude%20%C2%B7%20Gemini%20%C2%B7%20Cursor-111111?style=flat-square" alt="Works with Codex, Claude, Gemini, and Cursor">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-111111?style=flat-square" alt="MIT license"></a>
</p>

<p align="center">
  <strong>데이터 96,663개 &middot; 실계정 E2E 4종 &middot; provider canary 11개</strong><br>
  <sub>REST·LINK·FILE을 함께 찾고, 실제 명세와 파일을 검사해, 활용신청부터 첫 호출까지 잇는다.</sub>
</p>

---

공공데이터를 써 본 사람이라면 이런 장면을 안다.

찾고 싶은 건 분명하다. 검색어는 모르겠다. `폐업`을 넣으면 안 나오고 `휴·폐업`을 넣으면 나온다.
하나는 API, 하나는 엑셀, 하나는 다른 기관 사이트로 가는 링크다. 파일은 받아 보기 전까지 컬럼을
모른다. API는 신청하고, 승인받고, 키를 복사하고, 필수 파라미터를 맞춘 뒤에야 빈 응답을 받을 수 있다.

그쯤 되면 데이터보다 탭이 더 많이 열려 있다.

문틈에서 빼꼼 얼굴을 내민 이 사람은 어디서 뭘 찾았는지 길게 설명하지 않는다. 정확한 검색어를
몰라도 포털을 뒤지고, API와 파일과 외부 링크를 가려내고, 필요하면 신청서까지 쓴다. 잠시 사라졌다
돌아올 때는 데이터와 출처를 함께 내민다.

**oddsock은 이 조용한 탐색자를 당신의 AI 에이전트 안에 앉혀 둔다.**

## Before / after

이 질문에 답하고 싶다고 해보자.

> 산업재해나 환경위반 이력이 있는 기업이 그 뒤에도 정부와 계약했을까?

### 지금까지

에이전트에게 맡겨도 검색어를 모르면 별수 없다. `산업재해 기업 명단`, `중대재해 사업장`, `계약`,
`낙찰`, `조달`을 번갈아 검색한다. 결과마다 상세페이지를 열고, CSV와 XLS를 내려받고, API마다
활용신청서를 쓴다. 회사명과 주소와 날짜 형식은 서로 다르다.

결과가 0건이어도 이유를 모른다. 정말 없는 건지, 검색어가 틀린 건지, 파라미터 하나를 빼먹은 건지.

### oddsock이라면

```text
"산업재해나 환경위반 이력이 있는 기업이 이후에도 정부와 계약했는지 확인해줘."

catalog_search → inspect_dataset → (apply) → call_api
```

AI는 한 문장을 재해·행정처분·계약·낙찰·기업 식별자로 나눈다. oddsock은 약 9.6만 개의
API·파일·외부 링크에서 후보를 찾는다. 에이전트는 고른 후보의 실제 명세와 파일 컬럼을 검사하고,
필요하면 활용신청을 낸 뒤, 인증키를 보여주지 않고 호출한다.

질문은 한 줄이었다. 뒤에서는 여러 기관의 데이터 계약이 만난다.

## Numbers

그럴듯한 데모 대신 실제 계정과 실제 포털에서 확인했다.

| 확인한 것 | 결과 |
| --- | --- |
| 통합 카탈로그 | 2026-09-01 기준 96,663개 노드. REST·LINK·FILE과 복수 제공형 보존 |
| 실계정 end-to-end | 온비드 공매·나라장터 입찰·공영도매시장 경매·중소기업 지원사업 신청→승인→호출 |
| 외부 제공기관 | SafetyKorea·FoodSafetyKorea·VWorld typed 호출, 서울 열린데이터광장 계약 검사 |
| drift 감시 | provider adapter 4개, live canary 11개, 주간 CI |
| 실패 경계 | 미지원 LINK·불안전한 전송·불충분한 결합 근거에서 멈춤 |

검색→상세→호출 MCP 자체는 이미 있다. oddsock의 차이는 그 앞뒤다. **정확한 데이터 이름을 모르는
상태에서 후보를 찾고, 파일을 실제로 열어 보고, 활용신청과 키 관리까지 첫 호출로 잇는다.**

비교 방법과 근거는 [경쟁 워크플로 조사](docs/research/competitive-workflow-audit.md)에 있다.

## 검색보다 귀찮은 것

공공데이터포털에서 API 하나를 처음 부를 때 사람이 하던 일:

| 사람이 하던 일 | oddsock |
| --- | --- |
| 포털이 알아듣는 검색어 추측 | 자연어 목표를 3~8개의 검색축으로 나눔 |
| 상세페이지를 돌며 API·FILE·LINK 판별 | 실제 명세·다운로드 자산·컬럼 검사 |
| 활용목적 작성, 기능 선택, 신청 제출 | `apply`가 포털의 실제 신청 폼 제출 |
| 승인 상태 재확인 | 승인 결과와 심의 유형 확인 |
| serviceKey 복사 | 모델에 노출하지 않고 계정 키 주입 |
| XML 파싱과 엔드포인트별 호출 코드 작성 | `call_api`가 `pk`로 호출하고 JSON 반환 |

data.go.kr REST라면 사람은 정부 SSO에 한 번 로그인한다. 그다음 검색 → 검사 → 활용신청 → 승인 확인 →
키 획득 → 첫 호출은 CLI나 MCP 에이전트가 잇는다. LINK는 제공기관마다 별도 신청과 키가 필요할 수 있다.

자동승인이 곧바로 호출 가능하다는 뜻도 아니다. 게이트웨이 반영에는 실측으로 보통 7~10분이 걸렸다.
`call --wait 10m`은 키를 바꾸거나 다시 신청하는 대신 가만히 기다린다. 생각보다 드문 기능이다.

## How it works

oddsock은 검색 1위를 정답이라고 부르지 않는다. 다음 단계를 차례로 밟는다.

```text
1. 무슨 데이터가 필요한가?     → 질문을 서로 다른 역할의 검색축으로 나눈다
2. 이름이 틀렸을 수 있나?       → 96,663개 통합 카탈로그를 함께 뒤진다
3. 정말 쓸 수 있나?             → API 명세와 FILE의 실제 컬럼을 본다
4. 권한이 필요한가?              → 활용신청·승인·provider 키를 처리한다
5. 서로 연결되는가?              → 필드·범위·값 교집합을 확인한다
6. 근거가 부족한가?              → 억지로 엮지 않고 abstention한다
```

탐색은 넓게 한다. 주장은 좁게 한다.

자연어 검색은 데이터를 발견하는 장치이지, 상관관계나 인과관계를 자동으로 증명하는 장치가 아니다.
전체 계약은 [교차 데이터 연결 발견 명세](docs/specs/cross-domain-connection-discovery-v1.md), 실제 평가는
[연결 발견 평가](docs/research/connection-discovery-evaluation.md)에 있다.

## Install

저장소와 릴리스는 현재 비공개다. 먼저 [GitHub CLI](https://cli.github.com/)로 접근 권한이 있는
계정에 로그인한다.

```sh
gh auth login
gh api -H 'Accept: application/vnd.github.raw+json' \
  repos/JungHoonGhae/oddsock/contents/install.sh | sh
```

Windows:

```powershell
gh auth login
(& gh api -H "Accept: application/vnd.github.raw+json" repos/JungHoonGhae/oddsock/contents/install.ps1) |
  Out-String | Invoke-Expression
```

또는 저장소를 clone한 상태에서 Go 1.26 이상이 있다면:

```sh
go install ./cmd/oddsock
oddsock catalog sync
```

소스 설치에는 릴리스의 prebuilt 카탈로그가 포함되지 않으므로 첫 검색 전에 한 번 동기화한다.

### AI 에이전트에 연결

```sh
codex mcp add oddsock -- oddsock mcp
claude mcp add oddsock -- oddsock mcp
gemini mcp add --scope user oddsock oddsock mcp
```

Cursor·Claude Desktop처럼 JSON 설정을 쓰는 호스트:

```json
{
  "mcpServers": {
    "oddsock": {
      "command": "oddsock",
      "args": ["mcp"]
    }
  }
}
```

이제 데이터 이름 대신 궁금한 것을 말하면 된다.

```text
우리 동네 빈 상가가 늘어나는 이유를 유동인구, 폐업, 교통 변화로 같이 확인해줘.
내가 산 제품 중 국내나 해외에서 리콜된 것이 있는지 찾아줘.
정부 지원을 받은 기업이 실제 공공조달 매출까지 만들었는지 확인해줘.
```

## CLI

```sh
# 브라우저가 한 번 열린다. data.go.kr에 로그인한다.
oddsock login

# 정확한 데이터 이름을 몰라도 된다.
oddsock catalog discover "지역 소멸로 생길 사업 기회"

# API 명세 또는 FILE의 실제 자산·컬럼을 확인한다.
oddsock inspect <PK> --observe

# API가 필요하면 활용신청한다. CLI는 제출 전 y/N을 묻는다.
oddsock apply <PK> --purpose "지역 사업 기회 분석" --category research

# 엔드포인트와 serviceKey를 직접 다루지 않고 호출한다.
oddsock call --pk <PK> --param numOfRows=5
```

## 다 같은 데이터가 아니다

| 유형 | oddsock이 하는 일 |
| --- | --- |
| `REST` | 포털의 공식 operation과 필수 파라미터를 확인하고 신청·호출 |
| `LINK` | SafetyKorea·FoodSafetyKorea·VWorld 등 검증된 adapter만 typed 호출 |
| `FILE` | 다운로드 링크만 보여주지 않고 작은 표본과 실제 스키마를 관찰 |

API처럼 보이는 링크라고 엔드포인트를 지어내지 않는다. 파일이라고 사람에게 다운로드를 떠넘기지도 않는다.
검증된 계약이 없는 제공기관은 공식 경로를 알려 주고 멈춘다.

## 마법은 아니다

- 정부 SSO 로그인과 심의승인을 우회하지 않는다.
- 모든 LINK 제공기관을 하나의 키로 부르지 않는다.
- 연결 후보는 실제 필드·범위·값 교집합을 확인하기 전까지 후보다.
- 활용신청 자동화는 포털 HTML에 의존한다. 포털이 바뀌면 깨질 수 있고, `oddsock doctor`와 canary가 이를 감시한다.
- data.go.kr 밖의 이용조건은 각 제공기관 정책을 따른다.

## Security

`oddsock login`은 로그인이 끝나면 브라우저를 닫고 세션을 운영체제의 사용자 설정 디렉터리에 저장한다.
인증키는 MCP 입력·출력이나 로그에 내보내지 않고 호출 직전에만 넣는다. `oddsock logout`은 세션,
data.go.kr 키, provider 키와 Chrome 프로파일을 지운다.

세션과 키는 파일 권한으로 접근을 막지만 암호화되지는 않는다. 공용 머신에서는 쓰지 않는 편이 좋다.
MCP의 `apply`는 확인 대화 없이 실제 활용신청을 만들 수 있다. 직접 확인하고 싶다면 제출 전 `y/N`을
묻는 CLI를 쓰면 된다.

MCP에서는 호스트 AI가 검색축을 만들고 `catalog_search`에 구조화해 넘긴다. 반면 독립 CLI의
`catalog discover`는 설치되어 로그인된 Codex·Claude·Gemini·Cursor 중 하나를 별도 프로세스로 실행해
입력한 자연어 목표를 stdin으로 전달한다. oddsock은 그 로그인 토큰을 읽거나 저장하지 않지만, 목표는
선택된 AI 제공자에게 전송되므로 미공개 사업계획·개인정보 같은 민감한 내용은 넣지 않는다. 검색 뒤의
카탈로그 문장은 untrusted data로 표시하고 Codex·Claude·Gemini의 도구를 끈다. no-tools 격리가 없는
Cursor는 초기 검색 계획에만 쓰며 실제 카탈로그 metadata를 보내지 않는다.

보안 경계와 남는 위험은 [ADR](docs/adr/)과 [provider adapter guide](docs/provider-adapters.md)에 있다.

## 이전 이름

`opendatactl`과 `gongctl`은 전환 기간 동안 같은 엔진을 실행한다. 기존 설정·로그인 세션·인증키도
복사 없이 다시 찾는다. 새 자동화에는 `oddsock` 명령과 `oddsock://guide` MCP 리소스를 쓰면 된다.

비공개 저장소 전환 뒤 공개 Homebrew cask는 더 이상 갱신되지 않는다. v0.12.0 이상은 위의 인증된
`gh api` 설치 경로를 사용한다. 파일로 저장해 둔 v0.8 Windows 설치 스크립트도 저장소 이름 변경을
따라가지 못하므로, 위의 최신 PowerShell 명령을 한 번 실행해 기존 설치 폴더의 `oddsock.exe`,
`opendatactl.exe`, `gongctl.exe`를 함께 갱신한다. 설정과 로그인 상태는 그대로 유지된다.

## Development

새 provider adapter, 재현 가능한 교차 데이터 질문, 실패 사례를 환영한다. adapter는 공식 문서,
고정된 credential scope, contract test와 canary가 있어야 호출 가능 상태가 된다.

```sh
go test ./...
go vet ./...
go build ./...
```

구현 규칙은 [provider adapter guide](docs/provider-adapters.md), 설계 결정은 [ADR](docs/adr/)에서 시작한다.

## License

[MIT](LICENSE). 데이터의 짝을 찾는 데 가장 짧은 라이선스.
