# 상세 사용법

처음 설치하고 AI에 연결하려면 [README의 빠른 시작](../README.md#빠른-시작)을 따른다.
이 문서는 명령을 직접 실행하거나 검색·분석 설정을 조정할 때 참고한다.

[직접 신청·조회](#활용신청과-첫-호출) · [질문으로 탐색](#cli에서-목표로-탐색하기) ·
[목표 실행](#목표에서-결과까지-실행하기--experimental) · [Agent Skill](agent-skills.md) · [로그인·데이터 취급](#인증과-데이터-취급)

## 활용신청과 첫 호출

MCP에서는 에이전트가 검사 결과에 따라 `apply`와 `call_api`를 이어 간다. 직접 실행하려면 아래 CLI 명령을 사용한다.
고른 데이터의 `PK`는 검색 결과에서 확인한다.

```sh
odeduck inspect <PK> --observe

# 활용신청·인증 호출이 필요할 때 data.go.kr에 한 번 로그인한다.
odeduck login

# 신청이 필요한 API만 실행한다. CLI는 제출 전 y/N을 묻는다.
odeduck apply <PK> --purpose "공공데이터 비교 분석" --category research

# 검사 결과에 맞는 operation과 필수 파라미터를 사용한다.
odeduck call --pk <PK> --op <OPERATION> --param <NAME>=<VALUE>
```

`call`은 인증키를 내부에서 넣는다. 자동승인 후 게이트웨이 반영이 늦으면 `--wait 10m`으로
새 키 발급이나 재신청 없이 기다릴 수 있다.

| 제공형 | 취득 방식 |
| --- | --- |
| `REST` | 포털의 공식 operation·필수 파라미터·승인 상태를 확인해 호출 |
| `LINK` | SafetyKorea·FoodSafetyKorea·VWorld 등 검증된 adapter로 호출. 미지원 기관은 공식 경로 안내 |
| `FILE` | 지원되는 다운로드 자산에서 CSV·SHP의 DBF·XLSX 컬럼과 제한된 표본을 관찰 |

## CLI에서 목표로 탐색하기

검색어를 직접 정하기 어려우면 `catalog discover`에 질문을 전달한다. 설치·로그인된
Codex·Claude·Gemini·Cursor 중 하나가 검색 계획을 만든다.

```sh
odeduck catalog discover \
  "장마철에도 매출이 덜 흔들릴 동네 카페 후보를 찾고 싶어. 어떤 데이터를 같이 봐야 하는지 찾아줘." \
  --connections --limit 12 --semantic=false -f table
```

<details>
<summary>실제 검색 예시: 강수·유동인구·침수 기록을 함께 찾은 결과</summary>

아래는 실제 실행에서 찾은 연결 후보다. 강수 비교군·유동인구·침수 기록을 함께 제시했고
예상 결합키와 검증 전 한계를 남겼다. 매출 안정성이나 실제 데이터 결합을 입증한 결과는 아니다.

<p align="center">
  <img src="assets/odeduck-linkedin-demo.svg" width="900" alt="실제 catalog discover 실행에서 강수 비교군, 시간대별 유동인구, 침수 기록을 서로 다른 연결 후보로 찾고 각 데이터의 PK와 예상 결합키, 검증 전 한계를 표시한 결과">
</p>
<p align="center">
  <sub>v0.16.0 · 2026-09-03 · <code>catalog discover --connections</code> · data.go.kr 로그인·API 키·Ollama 없이 실행 · <a href="research/launch-readiness-linkedin-geeknews-2026-09.md">실행 조건과 후보 PK</a></sub>
</p>

</details>

`--semantic=false`는 Ollama 없이 실행하는 검색이다. 의미 검색을 함께 쓰려면 로컬 Ollama를
실행한 상태에서 `odeduck catalog semantic-build`로 인덱스를 만든다. 이후 `--semantic=false`를 빼면 된다.
의미 검색이 반드시 필요한 조사에는 `--require-semantic`을 사용한다. 인덱스나 Ollama에 문제가 있으면
일반 검색으로 조용히 대체하지 않고 실패한다.

## 목표에서 결과까지 실행하기 — experimental

`solve`는 오데덕의 핵심 목표 실행 경로다. 목표를 역할·범위·필수 출력으로 나누고 검색·검사·취득·연결·계산을 반복한다.
결과와 근거를 검토하다 빠진 자료가 드러나면 같은 목표와 예산 안에서 대안을 찾는다.
MCP에서는 `advance_goal`로 host가 같은 실행기를 진행한다. `experimental`은 현재 구현의 안정성
표시다. 보고·비교·계산과 그 근거에 도달하는 것이 이 실행기의 제품 목표다.
설정 전 `odeduck version`·`odeduck solve --help` 또는 실행 중인 MCP의 `odeduck://guide`로
지원 기능을 확인한다. 이 문서의 개발 중 기능이 설치된 바이너리에 있다고 가정하지 않는다.

![설명용 가상 예시: 공매 물건에 비교할 실거래 기록을 붙여 가격을 나란히 놓고, 비교 자료가 부족한 물건과 출처·비교 조건도 함께 남긴다.](assets/odeduck-goal-flow.png)

그림의 물건과 금액은 설명용 가상 예시다. 지원하는 연산과 허용된 검토 안에서 실행하며,
공매 분석이나 분야 간 자율 완주를 검증한 성공 사례를 뜻하지 않는다.
[다이어그램 원본](assets/odeduck-goal-flow.html)

설치·로그인된 Codex·Claude·Gemini 중 하나와 로컬 Ollama가 필요하다. Cursor는 `solve`의 계획기로
지원하지 않는다. 아래 명령은 실험 기능을 실행하는 예시이며 목표 완주를 보장하지 않는다.

```sh
odeduck catalog semantic-build
odeduck solve "부산 공매 아파트의 최소입찰가와 비교할 만한 실거래 기록을 찾아 표로 정리해줘. 비교 조건과 출처, 자료가 부족한 물건도 표시해줘"
```

**실행 뒤에는 JSON의 결과·근거·미완료 사유를 확인한다.** 결과가 만들어져도 질문 전체가 해결됐다는 뜻은 아니다.

- **실행 한도:** 기본 32단계. agent CLI의 비용·호출 한도가 적용된다.
- **취득 조건:** 의미 검색이 실제로 쓰이지 않으면 기본적으로 멈춘다. 자동 활용신청은 하지 않는다.
- **계산 범위:** API·CSV·ZIP 내부 CSV·XLSX·포털 STD의 제한된 표본을 조회·연결·집계한다.
- **완료 조건:** 필수 출력과 지역·식별자·기간의 의미가 충족돼야 한다. 필요한 검토가 끝나지 않으면 미완료로 반환하고 CLI는 실패 코드로 종료한다.

원천 값 공유와 별도 모델 검토는 **기본으로 꺼져 있다.** 위 기본 명령은 취득·계산까지 진행할 수 있지만,
현재 완료 판정에는 별도 검토가 필요하므로 검토를 켜지 않은 실행은 `output_ready`로 끝나지 않는다.
완료 검토를 사용하려면 아래 설정을 명시적으로 켜야 한다.
이는 모델 검토이며 현장·사람 검증을 뜻하지 않는다.

### 결과 검토까지 실행하기

예를 들어 Codex를 수신자로 선택해 원천 공유와 보고·계산 검토를 허용한다면 아래처럼 실행한다.
선택한 원천 값이 Codex 계획기와 별도 검토 요청에 전달되며, 해당 CLI의 이용 비용·한도가 적용된다.

```sh
odeduck solve "부산 공매 아파트의 최소입찰가와 비교할 만한 실거래 기록을 찾아 표로 정리해줘. 비교 조건과 출처, 자료가 부족한 물건도 표시해줘" \
  --agent codex --share-evidence --review-analyses
```

이 설정은 결과 검토 경로를 열며, 요청한 목표의 성공을 보장하지 않는다. 신청이 필요한 API는
[일반 신청 경로](#활용신청과-첫-호출)로 권한을 먼저 준비한다.

<details>
<summary>공유·검토 권한과 MCP 서버 설정</summary>

외부 CLI 계획기에는 기본적으로 원천 값을 보내지 않는다. 선택한 행·필드를 근거로 읽게 하려면
`--agent=claude --share-evidence`처럼 수신 모델 하나를 명시한다. 자동 모델 전환은 허용하지 않는다.
MCP에서는 서버를 `mcp --share-goal-evidence`로 시작해야 하며 모델이 tool 입력으로 켤 수 없다.
세션당 최대 8개/64 KiB의 선택 근거만 허용한다. 개인정보·전송 권한은 사용자가 확인해야 하며,
이 설정은 기존 MCP 실행 결과나 `call_api` 원문 출력을 차단하는 기능이 아니다.

출력 형식이 맞아도 지역·식별자·시간의 의미가 검증되지 않으면 `review_required`로 남기고 같은 목표·예산
안에서 추가 근거와 대안을 찾는다. 제한된 원천 보고는 CLI의 `--review-source-reports`(명시한 agent와
`--share-evidence` 필수), MCP의 `--review-goals-with=claude`와 `--share-goal-evidence`로 별도 검토를
켤 수 있다. typed 관계·계산은 CLI `--review-analyses` 또는 MCP 시작 설정 `--review-goal-analyses`로
추가 허용한다. 원래 질문·출력별 지지와 관계·기간·측정·범위를 통과해야 성공 종료한다. 모델 검토이지
현장·사람 검증은 아니며 공간·인과·사업 가설 및 범용 목표 완주는 미완료다. 결과와 검토는 실행 revision에 묶인다.
[원천 보고 검토의 범위와 공개 정책](specs/goal-source-report-review-v1.md)을 확인할 수 있다.

</details>

<details>
<summary>지원 연산과 원천 추적 범위</summary>

실제 결합은 API·직접 CSV·ZIP 내부 CSV·XLSX 셀 범위·포털 STD의 제한된 표본을 지원한다. 먼저 필수 역할·범위·출력을 고정하고,
빠진 항목이 있으면 부분 결과로 남겨 탐색을 계속한다. 한 원천의 필드 조회·집계도 가능하며 결합은 선택 연산이다.
`sample_executed`는 표본 실행 결과이며, 필수 요구 충족이나 의미 검증과는 별개다.

출처, 요청 조건, 원본 위치와 연결하지 못한 기록을 추적한다. 원천에 적힌 값, 계산 결과, 해석과 가설은 구분한다.
직접 CSV는 선택적으로 전체를 순차 검사하고, 관측한 좌표를 기준으로 전체 일치 레코드의 최근접 후보를
계산할 수 있다. 이 구면 거리는 실제 이동 경로나 현재 접근 가능성을 뜻하지 않는다.
전체 CSV 검사에서는 `sample.whereIn`으로 여러 지역·연령처럼 정확한 값 목록을 함께 고를 수 있다.
파일 전체 검사와 일치 행의 실제 보관, 요청한 집단 전체의 확보는 각각 구분해 보고한다.

검사한 FILE의 등록된 공식 HTML 설명은 `sample`의 `delivery:"document"`로 별도 관측할 수 있다.
선택 공개한 원문만 `composition.support`로 해석 검토에 연결하며 계산 행이나 모집단 증거로 자동
승격하지 않는다. 현재 지원은 행안부 월간 통계 도움말과 등록된 KOSIS 공식 답변이며 임의 URL·PDF는
지원하지 않는다. [보조 문서 읽기 계약](specs/source-document-acquisition-v1.md)에 범위를 명시했다.
[실행 계약과 한계](specs/goal-driven-composition-v1.md)를 확인할 수 있다.

</details>

<details>
<summary>연결 판정을 장부에 남기는 조건</summary>

연결 판정은 MCP의 `record_connection_assessment`로 로컬 장부에 남길 수 있다. 출처·필드·관측 시점·
표본 집계를 보존하며 API 응답 원문이나 인증정보는 저장하지 않는다. `sample_verified`는 같은 MCP
세션의 최근 `call_api` profile 영수증과 일치해야 한다. `solve`·`advance_goal`은 장부에 자동 기록하지 않는다.
저장·판정 조건은 [연결 근거 장부 명세](specs/connection-evidence-ledger-v1.md)에 있다.

</details>

## 인증과 데이터 취급

`odeduck login`은 사람이 정부 SSO 로그인을 마치면 쿠키를 저장하고 브라우저를 닫는다.
정부 SSO나 제공기관의 심의승인을 우회하지 않는다. data.go.kr의 계정 키와 외부 provider 키는 구분하며
확인한 호출 범위에서만 사용한다. 인증키는 MCP 입력·출력이나 로그에 내보내지 않는다.

세션과 키는 운영체제의 사용자 설정 디렉터리에 파일 권한으로 보호해 저장하며 암호화되지는 않는다.
`odeduck logout`은 세션, data.go.kr 키, provider 키와 Chrome 프로파일을 지운다.

MCP의 `apply`는 확인 대화 없이 실제 활용신청을 만들 수 있다. 제출 전에 직접 확인하려면
`y/N`을 묻는 CLI를 사용한다. 포털 HTML 변경은 `odeduck doctor`와 실호출 점검으로 감시한다.

`catalog discover`와 `solve`는 자연어 목표를 선택한 AI 제공자에게 전달한다. `discover`는
해당 에이전트의 로그인 토큰을 읽거나 저장하지 않는다. Cursor는 격리된 초기 검색 계획에만 사용하며
실제 카탈로그 메타데이터를 보내지 않는다. 미공개 정보를 목표에 포함하기 전에 전송 범위를 확인해야 한다.

목표 실행기의 선택 근거 공개 설정과 일반 MCP 응답은 별개다. `call_api`의 원문 결과는 MCP host가
받는다. 원천 데이터의 이용조건과 전송 권한은 각 제공기관 정책을 따른다.
자세한 경계는 [ADR](adr/)과 [provider adapter guide](provider-adapters.md)에 있다.

## Go 소스 설치와 카탈로그 동기화

Go 1.26.6 이상:

```sh
go install github.com/JungHoonGhae/odeduck/cmd/odeduck@latest
odeduck catalog sync
```

릴리스 설치기는 checksum을 검증하고 같은 릴리스의 카탈로그 스냅샷을 설치한다.
소스 설치에는 이 prebuilt 카탈로그가 포함되지 않으므로 첫 검색 전에 한 번 동기화한다.
전체 동기화의 웹 보강은 수십 분 걸릴 수 있다. 빠른 월간 CSV 목록만 필요하면
`odeduck catalog sync --source official-file`을 사용한다. 이 빠른 목록에는 일부 API·FILE 복수
제공형이 빠질 수 있으므로, 완전한 제공형 탐색에는 릴리스 카탈로그를 권장한다.
