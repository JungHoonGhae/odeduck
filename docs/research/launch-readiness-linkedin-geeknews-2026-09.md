# LinkedIn·GeekNews 공개 런칭 조사

조사일: 2026-09-03

## 결론

odeduck의 런칭 소재는 “공공데이터 9.6만 건” 자체가 아니라, 일상어로 던진 질문이 공공기관의 데이터
용어로 바뀌고 서로 다른 자료까지 한 화면에 놓이는 순간이다. 첫 체험은 계정 생성이나 data.go.kr 로그인
없이 끝나야 한다. 활용신청과 호출은 그 다음 단계로 분리한다.

현재 외부 공개를 막는 가장 큰 요인은 제품 기능이 아니라 저장소 visibility다. 프로필에는 odeduck의
실제 URL을 미리 연결하고 `public release in progress`로 표시했다. 소유자에게는 정상 링크지만 비로그인
방문자에게는 아직 404다. 공개 전환 직후 비로그인 상태에서 링크를 확인하고 진행 중 표기를 제거한다.

## 플랫폼이 명시한 조건

### GeekNews

- 직접 만든 서비스나 오픈소스는 일반 뉴스가 아니라 `Show GN`으로 등록해야 한다.
- 가입이나 이메일 제출 없이 바로 써볼 수 있는 상태가 권장된다.
- 같은 프로젝트를 너무 자주 다시 올리는 것은 피하고, 큰 변화가 있을 때 다시 소개한다.
- Show GN 본문 링크로 YouTube를 쓰지 않는다.
- 과장된 제목보다 무엇을 만들었고 왜 만들었는지 분명한 제목이 맞다.

근거: [GeekNews 커뮤니티 가이드라인](https://news.hada.io/guidelines),
[Show GN 목록](https://news.hada.io/show), [GeekNews 시작 안내](https://news.hada.io/start)

해석: `catalog search`를 로그인 없는 첫 체험으로 내세우고, data.go.kr SSO가 필요한 `apply`·`call`은
확장 기능으로 설명해야 한다. “AI가 사업 아이디어를 찾아준다”보다 “일상어와 공공데이터의 용어 차이를
메운다”가 첫 문장에 더 적합하다.

### LinkedIn

- 일반 게시물 본문은 최대 3,000자다.
- 링크 미리보기 이미지는 1.91:1 비율, 1200×627이 권장된다.
- 공개 범위를 `Anyone`으로 둔 게시물은 LinkedIn 밖과 검색엔진에도 노출될 수 있다.
- 이미지·영상·문서를 붙이거나 게시 시간을 예약할 수 있다.

근거: [LinkedIn 링크 공유 안내](https://www.linkedin.com/help/linkedin/answer/a525301),
[게시물 작성 안내](https://www.linkedin.com/help/linkedin/answer/a528176),
[LinkedIn 밖 게시물 노출 안내](https://www.linkedin.com/help/linkedin/answer/a529065)

해석: 기능 목록을 3,000자까지 채우는 것보다, “사람이 늘어나는 동네”라는 일상어가 `생활인구`라는
공식 용어를 만나기까지의 실패 경험을 짧게 보여주고 저장소 링크로 보낸다. 1200×627 전용 카드를
첨부하고 공개 범위는 `Anyone`으로 둔다.

### GitHub

- 저장소 social preview는 다른 플랫폼에서 프로젝트를 식별하는 데 쓰인다.
- 이미지는 PNG/JPG/GIF, 1MB 미만이어야 한다.
- 최소 640×320, 권장 1280×640이다.
- 새 social preview의 첫 업로드는 public 저장소에서만 가능하다. private 저장소는 과거에 이미 이미지를
  올린 경우에만 다시 업로드할 수 있다.
- 저장소가 public일 때만 social preview가 외부 공유에 쓰인다.

근거: [GitHub 저장소 social preview 문서](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/customizing-your-repositorys-social-media-preview)

## 로컬에서 검증한 첫 체험

격리된 임시 설정 디렉터리에서 v0.16.0 릴리스 바이너리와 `odeduck-catalog.json.gz`만 사용했다.

- snapshot 설치: 96,683건
- 로그인: 없음
- API 키: 없음
- 외부 AI 호출: 없음 (`--semantic=false`)
- 결과: 생활인구, 추정매출, 상권 점포, 상권 개폐업 축에서 273건 발견

대표 명령은 README의 `먼저 로그인 없이, 그다음 한 문장`에 기록했다.

## 자연어 한 문장 데모

같은 격리 환경과 v0.16.0 릴리스에서, 로그인된 Codex CLI를 검색 계획기로 사용했다. data.go.kr 로그인,
provider key, Ollama semantic index는 사용하지 않았다.

```sh
odeduck catalog discover \
  "장마철에도 매출이 덜 흔들릴 동네 카페 후보를 찾고 싶어. 어떤 데이터를 같이 봐야 하는지 찾아줘." \
  --agent codex --connections=true --max-connections 3 \
  --limit 12 --semantic=false -f table
```

전체 실행은 약 62초였다. 검색 계획기는 카페 매출·시간대 유동인구·강수량·승하차·사업체·개폐업·임대료
축을 만들었고, 실제 1차 결과를 본 뒤 우천 비교군·공간 정합·달력 교란·보행 접근·침수 위험 축으로
확장했다.

최종 연결 후보는 다음 세 개였다.

| 역할 | Anchor | Bridge | 예상 결합키 | 후보가 추가하는 판단 |
| --- | --- | --- | --- | --- |
| 우천 비교군 | `15097163` 강수없음 | `15097164` 강수있음 | 상권·업종·날짜·시간대 | 동일 조건의 우천 감소율 비교 |
| 수요 | `15097163` | `15097166` 유동인구 | 상권·기준연월·시간대 | 비가 와도 남는 시간대 수요 구분 |
| 침수·통행 단절 | `15097163` | `15130537` 침수 기록 | 발생일·구간 좌표·상권 | 평소 비와 집중호우 위험 분리 |

CLI는 이 결과를 verified join이 아니라 candidate로 표시하고, `describe`와 실제 표본으로 key·match
rate·cardinality를 확인하라고 경고했다. 런칭 문구의 “찾았다”는 데이터와 연결 후보를 발견했다는 뜻이며,
카페의 매출 안정성을 입증했다는 뜻이 아니다.

## 공개 전 안전 점검

Gitleaks v8.30.1로 현재 파일과 Git 전체 이력을 `--redact` 상태에서 검사했다. 탐지된 값은 모두 다음
세 종류로 분류됐다.

- provider 호출 테스트의 명시적 dummy key
- Go module checksum이 남은 로컬 비추적 review diff
- WebSocket RFC 예제 nonce

이후 영상 brief에서 오래된 `Style key` 문구도 제거해 공개 diff의 불필요한 secret-like 문자열을 줄였다.

실제 자격증명으로 분류된 항목은 없었다. 공개 직전에는 같은 검사를 한 번 더 수행하고, GitHub의
secret scanning 설정도 public 전환 후 확인한다.

## 런칭 순서

1. README·installer·공유 카드 변경을 CI가 통과한 PR로 합친다.
2. immutable releases를 켠 뒤 `v0.16.1` 태그 릴리스에서 바이너리·설치 스크립트·checksum·카탈로그
   asset을 확인한다.
3. 저장소를 public으로 전환한다.
4. 저장소를 public으로 바꾸자마자 `main`에 PR·필수 CI·force-push 차단 ruleset을 적용한다.
5. 로그아웃 상태에서 README 설치 명령과 저장소 링크를 다시 검증한다.
6. GitHub social preview를 1280×640 카드로 설정하고 private vulnerability reporting을 켠다.
7. GitHub 프로필 링크를 비로그인 상태에서 확인하고 `public release in progress` 표기를 제거한다.
8. GeekNews Show GN과 LinkedIn을 같은 날이 아니라 순차적으로 게시해 유입과 실패를 관찰한다.
