# GitHub 프로필 포지셔닝: 현재 주목받는 빌더에게서 무엇을 가져올 것인가

조사일: 2026-09-03
범위: 당사자의 GitHub 프로필·저장소, 개인 사이트와 직접 쓴 글만 사용했다. 관찰은 조사 시점 기준이며,
추천은 출처의 문구를 옮기지 않은 해석이다.

## 한 줄 결론

JungHoon은 “AI로 여러 가지를 만드는 사람”보다 **공식 제품이 끝내지 않은 작업 흐름을, 사람과
에이전트가 실제로 조작할 수 있는 도구로 완성하는 빌더**로 기억되어야 한다.

현재 헤드라인은 이미 좋다.

> I build the command the product forgot to ship.

이를 받치는 ownable philosophy는 **Software should have handles**다. 제품이 기술적으로 존재하더라도
browser-only 화면, 빠진 API, 반복 클릭에 갇혀 있으면 실제로 다룰 수 없다. JungHoon은 한 층 더 내려가
그 막힌 지점을 CLI, MCP, 로컬 자동화와 adapter라는 **잡고 움직일 수 있는 손잡이**로 만든다.

## 레퍼런스에서 가져올 패턴

| 빌더 | 현재 프로필·철학에서 확인한 패턴 | JungHoon에게 가져올 것 | 피할 것 |
| --- | --- | --- | --- |
| **Peter Steinberger** | OpenClaw를 시작한 빌더이며, 현재 profile README는 짧은 정체성 → `Start Here` → 분야별 프로젝트 → 활동·철학 순서다. 반복 수작업을 루프를 닫을 기회로 보고 CLI부터 만들어 에이전트가 호출·검증하게 한다. [OpenClaw README](https://github.com/openclaw/openclaw), [프로필](https://github.com/steipete/steipete/blob/main/README.md), [작업기, 2025-12-28](https://steipete.me/posts/2025/shipping-at-inference-speed) | 가장 강한 3–5개를 `막힌 작업 → 작동하는 결과`로 설명하고 하나의 세계관으로 묶기 | 긴 프로젝트 백과사전, badge·별명·농담의 과잉, 그의 ship 구호를 변형한 문구 |
| **Matt Pocock** | GitHub README는 현재 AI Hero 배너 하나로 집중됐고 pins가 현재 AI 작업을 증명한다. AI Hero는 코드 생성이 싸질수록 공학적 기본기의 가치가 커진다는 한 주장을 반복한다. [프로필](https://github.com/mattpocock/mattpocock/blob/main/readme.md), [AI Hero](https://www.aihero.dev/), [skills](https://github.com/mattpocock/skills) | 방문자가 “지금 거는 한 가지 베팅”을 기억하게 하고, 공개 후 odeduck을 flagship으로 올리기 | 기존 청중이 필요한 배너-only 프로필, 추상적인 AI 전문가 포지셔닝 |
| **DHH** | GitHub는 극도로 비워 두지만 개인 사이트는 Rails·Omarchy·37signals 등 만든 결과를 한 문장에 쌓는다. 최근에는 AI와 오픈소스가 고정된 시스템을 사용자가 바꿀 수 있게 한다는 세계관을 펴고, Basecamp에는 챗봇 대신 API+CLI+skill로 전체 작업을 열었다. [프로필](https://github.com/dhh), [사이트](https://dhh.dk/), [malleable computer, 2026-04-15](https://world.hey.com/dhh/the-malleable-computer-7c187a9b), [agent accessibility, 2026-03-25](https://world.hey.com/dhh/basecamp-becomes-agent-accessible-3ae6b949) | 서로 다른 도구를 기억 가능한 하나의 철학으로 묶고 곧바로 실제 작품으로 증명하기 | 긴 경력의 권위 없이 선언적 자신감만 흉내 내기 |
| **Armin Ronacher** | profile은 Flask, Earendil, Pallets·Sentry라는 현재·과거 증거를 짧게 잇는다. 그의 도구 기준은 빠른 응답, 명확한 오류, 오용을 견디는 경계, 관찰 가능성이다. CLI로 충분하면 CLI를 쓰고 MCP는 대안이 불안정할 때 쓴다. [프로필](https://github.com/mitsuhiko), [Agentic Coding, 2025-06-12](https://lucumr.pocoo.org/2025/6/12/agentic-coding/), [Earendil Purpose](https://earendil.com/purpose/) | 비공식 통합을 `local-first, inspectable, bounded, reversible, verified`라는 신뢰 기준으로 차별화하기 | MCP 자체를 목적이나 정체성으로 만들기 |
| **Simon Willison** | 한 문장 현재 focus 뒤에 최근 release·글·TIL이 자동 갱신된다. 형용사 대신 실제 shipping이 현재성을 증명한다. [프로필](https://github.com/simonw/simonw/blob/main/README.md), [구현 설명, 2020-07-10](https://simonwillison.net/2020/Jul/10/self-updating-profile-readme/) | 기존 별 수 자동 갱신에 `Recently shipped` 3개를 더해 살아 있는 증거 만들기 | 꾸준한 글 채널 없이 세 개 feed를 그대로 복제하기 |
| **Mitchell Hashimoto** | 사이트는 현재 역할·Ghostty·과거 작업·인간적 한 줄만 둔다. Ghostty는 빠름·기능·native UI 중 하나를 포기하게 하는 기존 선택을 거부했고, 약 2년간 검증한 뒤 1.0으로 공개했다. [사이트](https://mitchellh.com/), [Ghostty 1.0, 2024-10-22](https://mitchellh.com/writing/ghostty-is-coming), [README](https://github.com/ghostty-org/ghostty/blob/main/README.md) | 수식어보다 완성도와 사용자가 더는 감수하지 않아도 되는 타협 보여주기 | 이미 인지도가 높은 사람만 가능한 극단적 미니멀리즘 |

가장 적합한 조합은 **Peter의 프로젝트 계층 + Matt의 단일한 현재 주장 + DHH의 세계관 + Armin의
신뢰 기준 + Simon의 자동 증거 + Mitchell의 절제**다. 이들의 이름을 실제 프로필에서 언급할 필요는 없다.

## 현재 프로필에서 바꿀 것

[현재 프로필](https://github.com/JungHoonGhae)은 헤드라인과 `Toss Securities / KakaoTalk / Korean
public data` 세 예가 강하다. 별 수 자동 갱신도 유효하다. 다만 다음은 고유성을 흐린다.

- `git diff ~/.philosophy`의 빠른 출시·완벽보다 반복 같은 문장은 흔한 빌더 격언이다.
- `Start Here` 뒤 일곱 분야에서 프로젝트를 다시 모두 나열해 대표작을 희석한다.
- 방문자 badge, 경고문과 여러 숨은 농담은 금융·메시징 비공식 통합에 필요한 신뢰보다 연출을 앞세운다.
- odeduck은 공개 검증 전까지 외부 링크가 404이므로, 공개 후에만 `Currently building` 첫 자리로 올린다.

권장 구조는 아래 한 화면이다.

```text
builder thesis
3문장 philosophy: 문제 → 접근 → 품질 기준
Currently building: odeduck + 실제 demo
Selected work 4개: odeduck / tossinvest-cli / openkakao-cli / agent tool 1개
Operating principles 4줄
Recently shipped 3개 (자동 갱신)
인간적인 한 문장 + contact
```

프로젝트 설명은 `사용자의 막힘 → 끝내는 작업 → 신뢰 조건` 순서로 쓰고, 별 수는 대표작에만 둔다.

## 바로 쓸 수 있는 문안

헤드라인 아래 설명:

> Software can technically exist and still be unusable. Useful workflows get trapped behind
> browser-only screens, missing APIs, and repetitive clicks. I go one layer deeper and turn those
> dead ends into inspectable, composable tools that people and agents can actually use.

철학 네 줄:

> **Start with the blocked workflow, not the API.**
> Go one layer deeper when the official surface stops early.
> Use AI for breadth; use deterministic tools to cross the last mile.
> A tool is finished when a real person or agent can complete—and verify—the job.

GitHub bio:

> I turn browser-only and unfinished product workflows into open-source CLIs, MCP servers, and
> native tools for people and agents.
