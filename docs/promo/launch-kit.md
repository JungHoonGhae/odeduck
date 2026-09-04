# 오데덕 launch kit

## 하나의 메시지

공공데이터는 없어서 못 쓰는 경우보다, 이름을 몰라서 못 찾고 찾은 뒤에도 신청·파일·명세가 흩어져
있어서 못 쓰는 경우가 많다. 오데덕은 일상적인 질문을 실제 데이터 후보와 다음 행동으로 바꾼다.

## GeekNews Show GN

### 제목

> Show GN: 장마철 카페를 물었더니 우천 비교군과 침수 이력을 찾아온 오데덕

### 본문

이 질문 한 줄을 던졌습니다.

> 장마철에도 매출이 덜 흔들릴 동네 카페 후보를 찾고 싶어. 어떤 데이터를 같이 봐야 하는지 찾아줘.

약 1분 뒤 서로 다른 자료가 나란히 놓였습니다.

- 서대문구 상권별 업종 이용률·기상현황 `강수없음` 자료
- 같은 기관·같은 구조의 `강수있음` 비교군
- 시간대별 유동인구
- 침수와 도로통제 이력

도구는 `상권 + 업종 + 날짜 + 시간대` 같은 예상 결합키도 보여줬습니다. 다만 실제 파일의 match rate와
cardinality를 확인하지 않았으므로 “비 오는 날 잘되는 카페”라는 결론 대신 **연결 후보**라고 멈췄습니다.

이 데모를 만들게 된 문제는 더 평범했습니다. 가게를 열기 전에 사람이 늘고 있는지, 비슷한 가게는
얼마나 버티는지, 평일 낮과 주말 밤 중 언제 돈이 쓰이는지 궁금했습니다.

그런데 공공데이터포털은 `사람이 늘어나는 동네`보다 `생활인구`를 잘 알아듣습니다. 매출은
`추정매출`, 가게의 생존은 `점포이력`과 `개폐업`으로 나뉩니다. 데이터가 없는 게 아니라, 먼저 데이터가
붙인 이름을 알아야 찾을 수 있었습니다.

그래서 오데덕을 만들었습니다. 한국 공공데이터 9.6만+건의 REST·LINK·FILE을 함께 찾고, 실제 명세와
파일 컬럼을 검사하고, 필요하면 활용신청부터 첫 API 호출까지 이어주는 오픈소스 CLI/MCP입니다.
설치 명령과 MCP 서버의 기술 이름은 호환성을 위해 `odeduck`을 유지합니다.

첫 검색은 data.go.kr 계정이나 API 키 없이 바로 됩니다.

```sh
curl -fsSL https://github.com/JungHoonGhae/odeduck/releases/download/v0.17.2/install.sh | sh

odeduck catalog search \
  "서울에서 작은 가게 후보를 좁힐 자료" \
  --concept 생활인구 --concept 추정매출 \
  --concept "상권 점포" --concept "상권 개폐업" \
  --limit 8 --semantic=false -f table
```

Codex·Claude·Gemini·Cursor 중 하나가 이미 설치되어 로그인돼 있다면 검색축도 직접 적을 필요가 없습니다.

```sh
odeduck catalog discover \
  "장마철에도 매출이 덜 흔들릴 동네 카페 후보를 찾고 싶어. 어떤 데이터를 같이 봐야 하는지 찾아줘." \
  --connections --limit 12 --semantic=false -f table
```

실제 활용신청과 호출은 정부 SSO를 우회하지 않습니다. 사람이 data.go.kr에 한 번 로그인하면 이후
신청·승인 확인·키 주입·첫 호출을 잇습니다. 지원하지 않는 외부 기관은 엔드포인트를 지어내지 않고 공식
경로를 알려주고 멈춥니다.

검색 1위를 정답이라고 부르지 않는 것도 중요한 원칙입니다. 연결 후보는 실제 필드와 값의 교집합을
확인하기 전까지 후보로만 표시합니다.

GitHub: https://github.com/JungHoonGhae/odeduck

## LinkedIn

### 게시물

“장마철에도 매출이 덜 흔들릴 동네 카페 후보를 찾고 싶어.”

질문 한 줄을 던졌다.

약 1분 뒤 AI가 꺼내 온 것은 카페 추천 목록이 아니었다.

1. 서대문구 상권별 업종 이용률·기상현황 `강수없음`
2. 같은 기관·같은 구조의 `강수있음` 비교군
3. 시간대별 유동인구
4. 침수와 도로통제 이력

그리고 `상권 + 업종 + 날짜 + 시간대`라는 예상 결합키.

흥미로웠던 건 결론보다 멈추는 방식이었다. 실제 파일의 match rate와 cardinality를 확인하지 않았으니
“비 오는 날 잘되는 카페”라고 말하지 않고 **연결 후보**라고 표시했다.

데이터는 있었다. 답은 흩어져 있었다.

“사람이 늘어나는 동네”를 검색하면 원하는 데이터가 잘 나오지 않는다. 포털이 알아듣는 말은
`생활인구`이기 때문이다. 가게 매출은 `추정매출`, 생존은 `점포이력`, 진입과 퇴장은 `개폐업`으로
흩어져 있다.

사람이 해야 하는 일은 질문이 아니라 공공기관의 데이터 이름을 먼저 배우는 일이었다.

그래서 오데덕을 만들었다.

한 문장을 여러 검색축으로 나눠 한국 공공데이터 9.6만+건의 REST·LINK·FILE을 함께 찾는다. 실제 파일과
컬럼을 열어 보고, REST가 필요하면 활용신청·승인 확인·키 주입·첫 호출까지 이어준다. 지원하지 않는
기관은 그럴듯한 API를 만들지 않고 공식 경로에서 멈춘다.

첫 검색에는 데이터포털 계정과 API 키가 필요 없다. 자연어 검색 계획은 이미 로그인된
Codex·Claude·Gemini·Cursor 중 하나를 사용한다. 실제 활용신청과 호출 단계에서만 data.go.kr 로그인이
필요하다.

이번에 공개하는 건 “AI가 사업을 골라주는 도구”가 아니다. 감으로 끝나던 질문을 어떤 데이터로 검증할
수 있는지 보여주고, 그 데이터를 실제로 쓸 수 있는 곳까지 데려가는 오픈소스다.

한 분야에 답이 없으면, 오데덕은 다른 서랍을 연다.

https://github.com/JungHoonGhae/odeduck

#opensource #opendata #MCP #CLI #공공데이터

### 첨부 이미지

- `docs/assets/odeduck-linkedin-demo.png` — 1200×627, 게시물의 실제 데모를 보여주는 기본 이미지

## 공개 직후 프로필 최종 문구

```markdown
- 🔎 **[오데덕 (odeduck)](https://github.com/JungHoonGhae/odeduck)** — ask an everyday question; get the Korean public datasets, file schemas, access applications, and first API calls needed to test it
```

## 게시 직전 확인

- 비로그인 브라우저에서 저장소·README·릴리스 링크가 열린다.
- README의 macOS/Linux와 Windows 설치 명령이 최신 release를 받는다.
- 설치 직후 `catalog info`에 9.6만 건 이상이 나온다.
- 기본 카탈로그 검색이 로그인과 외부 AI 없이 재현된다.
- 자연어 한 문장 데모가 사용할 AI 제공자를 명시하고 연결 후보의 검증 전 경계를 유지한다.
- 저장소 공개 직후 GitHub social preview에 `docs/assets/odeduck-social-preview.png`를 등록한다.
- 이미 켜 둔 immutable releases 설정을 재확인하고, `v0.16.1`에서 `install.sh`·`install.ps1`·checksum을 확인한다.
- 공개 직후 `main`에 PR·필수 CI·force-push 차단 ruleset을 적용한 뒤 외부 링크를 배포한다.
- `Public launch smoke` workflow를 `v0.16.1`로 실행해 비로그인 설치·카탈로그·첫 검색을 확인한다.
- GitHub 프로필의 odeduck 링크를 비로그인 상태에서 확인하고 `public release in progress` 표기를 제거한다.
- GitHub private vulnerability reporting을 켜고 Security 탭의 비공개 신고 링크를 확인한다.
- LinkedIn 게시물 공개 범위를 `Anyone`으로 둔다.
- GeekNews는 일반 뉴스가 아니라 Show GN으로 등록한다.
