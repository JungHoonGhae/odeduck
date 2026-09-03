# oddsock launch kit

## 하나의 메시지

공공데이터는 없어서 못 쓰는 경우보다, 이름을 몰라서 못 찾고 찾은 뒤에도 신청·파일·명세가 흩어져
있어서 못 쓰는 경우가 많다. oddsock은 일상적인 질문을 실제 데이터 후보와 다음 행동으로 바꾼다.

## GeekNews Show GN

### 제목

> Show GN: “사람이 늘어나는 동네”를 검색했는데 아무것도 안 나와서 만든 oddsock

### 본문

가게를 열기 전에 궁금한 건 평범했습니다. 사람이 늘고 있는지, 비슷한 가게는 얼마나 버티는지,
평일 낮과 주말 밤 중 언제 돈이 쓰이는지.

그런데 공공데이터포털은 `사람이 늘어나는 동네`보다 `생활인구`를 잘 알아듣습니다. 매출은
`추정매출`, 가게의 생존은 `점포이력`과 `개폐업`으로 나뉩니다. 데이터가 없는 게 아니라, 먼저 데이터가
붙인 이름을 알아야 찾을 수 있었습니다.

그래서 oddsock을 만들었습니다. 한국 공공데이터 96,683건의 REST·LINK·FILE을 함께 찾고, 실제 명세와
파일 컬럼을 검사하고, 필요하면 활용신청부터 첫 API 호출까지 이어주는 오픈소스 CLI/MCP입니다.

첫 검색은 data.go.kr 계정이나 API 키 없이 바로 됩니다.

```sh
curl -fsSL https://raw.githubusercontent.com/JungHoonGhae/oddsock/main/install.sh | sh

oddsock catalog search \
  "서울에서 작은 가게 후보를 좁힐 자료" \
  --concept 생활인구 --concept 추정매출 \
  --concept "상권 점포" --concept "상권 개폐업" \
  --limit 8 --semantic=false -f table
```

실제 활용신청과 호출은 정부 SSO를 우회하지 않습니다. 사람이 data.go.kr에 한 번 로그인하면 이후
신청·승인 확인·키 주입·첫 호출을 잇습니다. 지원하지 않는 외부 기관은 엔드포인트를 지어내지 않고 공식
경로를 알려주고 멈춥니다.

검색 1위를 정답이라고 부르지 않는 것도 중요한 원칙입니다. 연결 후보는 실제 필드와 값의 교집합을
확인하기 전까지 후보로만 표시합니다.

GitHub: https://github.com/JungHoonGhae/oddsock

## LinkedIn

### 게시물

공공데이터는 공개돼 있었다. 찾기 쉽다는 뜻은 아니었다.

“사람이 늘어나는 동네”를 검색하면 원하는 데이터가 잘 나오지 않는다. 포털이 알아듣는 말은
`생활인구`이기 때문이다. 가게 매출은 `추정매출`, 생존은 `점포이력`, 진입과 퇴장은 `개폐업`으로
흩어져 있다.

사람이 해야 하는 일은 질문이 아니라 공공기관의 데이터 이름을 먼저 배우는 일이었다.

그래서 oddsock을 만들었다.

한 문장을 여러 검색축으로 나눠 한국 공공데이터 96,683건의 REST·LINK·FILE을 함께 찾는다. 실제 파일과
컬럼을 열어 보고, REST가 필요하면 활용신청·승인 확인·키 주입·첫 호출까지 이어준다. 지원하지 않는
기관은 그럴듯한 API를 만들지 않고 공식 경로에서 멈춘다.

첫 검색에는 계정도 API 키도 필요 없다.

이번에 공개하는 건 “AI가 사업을 골라주는 도구”가 아니다. 감으로 끝나던 질문을 어떤 데이터로 검증할
수 있는지 보여주고, 그 데이터를 실제로 쓸 수 있는 곳까지 데려가는 오픈소스다.

말은 없다. 질문 하나를 던진다. 데이터와 함께 돌아온다.

https://github.com/JungHoonGhae/oddsock

#opensource #opendata #MCP #CLI #공공데이터

### 첨부 이미지

- `docs/assets/oddsock-linkedin-card.png` — 1200×627

## 공개 직후 프로필 교체 문구

```markdown
- 🔎 **[oddsock](https://github.com/JungHoonGhae/oddsock)** — ask an everyday question; get the Korean public datasets, files, access applications, and first API calls needed to test it
```

## 게시 직전 확인

- 비로그인 브라우저에서 저장소·README·릴리스 링크가 열린다.
- README의 macOS/Linux와 Windows 설치 명령이 최신 release를 받는다.
- 설치 직후 `catalog info`에 9.6만 건 이상이 나온다.
- 대표 검색이 로그인과 외부 AI 없이 재현된다.
- GitHub social preview에 `docs/assets/oddsock-social-preview.png`를 등록한다.
- GitHub private vulnerability reporting을 켜고 Security 탭의 비공개 신고 링크를 확인한다.
- LinkedIn 게시물 공개 범위를 `Anyone`으로 둔다.
- GeekNews는 일반 뉴스가 아니라 Show GN으로 등록한다.
