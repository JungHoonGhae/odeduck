# 시각화 원본과 재사용

README 다이어그램의 **편집 소스·HTML·PNG·글꼴·출력 도구를 모두 저장소에서 관리**한다.
임시 폴더나 개인 컴퓨터의 절대 경로 없이 다시 만들 수 있다.

| 그림 | 브라우저에서 열기 | README·발표 자료에 사용 | 편집할 함수 |
| --- | --- | --- | --- |
| 한 질문에서 예상 밖의 연결까지 | [HTML](odeduck-unexpected-connections.html) | [PNG](odeduck-unexpected-connections.png) | `build_connections()` |
| 탐색부터 신청·호출까지 | [HTML](odeduck-api-workflow.html) | [PNG](odeduck-api-workflow.png) | `build_workflow()` |
| 전체 아키텍처 | [HTML](odeduck-system-overview.html) | [PNG](odeduck-system-overview.png) | `build_architecture()` |
| 목표 기반 연결·분석 | [HTML](odeduck-goal-flow.html) | [PNG](odeduck-goal-flow.png) | `build_goal()` |

각 함수는 [`scripts/diagrams/build.py`](../../scripts/diagrams/build.py)에 있다.
문구·좌표·아이콘·연결선을 여기서 수정한다. HTML에는 SVG와 Pretendard가 내장되어 있어
다운로드한 파일을 브라우저에서 바로 열 수 있다. HTML과 PNG는 생성 결과이므로,
수정 사항을 계속 유지하려면 편집 소스에 반영한다.

## 수정하고 다시 출력하기

저장소 루트에서 실행한다. Python 3.10 이상을 사용한다.

1. `build.py`에서 해당 그림의 함수를 수정한다. 공통 색상은 파일 상단,
   아이콘은 `icon()`, 원래 캐릭터는 [brand-symbol.svg](brand-symbol.svg)가 기준이다.
2. HTML을 다시 만든다. 현재 문구는 저장된 글꼴만으로 오프라인 생성할 수 있다.

   ```sh
   python3 scripts/diagrams/build.py
   ```

3. PNG 출력 환경을 처음 한 번 준비한다. 애플리케이션 실행에는 필요 없는 문서 제작 도구다.

   ```sh
   python3 -m venv scripts/diagrams/.venv
   scripts/diagrams/.venv/bin/python -m pip install -r scripts/diagrams/requirements.txt
   scripts/diagrams/.venv/bin/python -m playwright install chromium
   ```

4. PNG를 다시 만들고 HTML·PNG를 열어 확인한다. 기본 2배 해상도로 출력하며,
   글꼴 로딩·노드 안의 글자 잘림·화살표 라벨과 노드의 겹침·외부 요청을 검사한다.

   ```sh
   scripts/diagrams/.venv/bin/python scripts/diagrams/render.py
   ```

편집 소스와 변경된 HTML·PNG를 함께 커밋한다. 한 그림만 작업할 때는 두 명령 뒤에
`unexpected-connections`, `api-workflow`, `system-overview`, `goal-flow` 중 하나를 붙인다.
`--help`에서 출력 폴더와 PNG 배율 옵션을 볼 수 있다.
Windows에서는 가상환경 실행 파일이 `.venv/Scripts/python.exe`에 있다.

새 문구에 저장되지 않은 글자가 필요하면 생성기가 필요한 글꼴 파일을 알려준다.
다음 명령으로 공식 Pretendard v1.3.9의 해당 subset을 받아 저장한 뒤 다시 출력한다.
추가된 글꼴 파일도 함께 커밋하면 다음 작업부터 오프라인에서 재생성할 수 있다.

```sh
python3 scripts/diagrams/build.py --fetch-fonts
```

## 유지할 스타일과 내용

- 기존 [데모 그림](odeduck-linkedin-demo.svg)과 [비교 그림](odeduck-before-after.svg)의
  크림색 `#F7F3EB`, 검은 선 `#151513`, 보조 글자색 `#625F57`, 둥근 프레임을 따른다.
- 한국어는 Pretendard. [글꼴 파일·공식 CSS](../../scripts/diagrams/fonts/)와
  [SIL Open Font License](../../scripts/diagrams/fonts/OFL.txt)를 함께 보관한다.
  원본은 [Pretendard v1.3.9](https://github.com/orioncactus/pretendard/tree/v1.3.9)이며
  생성된 HTML에도 라이선스와 사용한 파일의 SHA-256을 넣는다.
- 기존 캐릭터를 변형하지 않고 배치한다. 긴 설명은 README에 두고 그림에는 아이콘과 짧은 라벨을 쓴다.
- 활용신청·승인 확인·인증키 주입·호출, CLI/MCP의 공통 엔진, FILE/STD의 별도 관찰 경로를 보존한다.
  기관의 심의승인과 실험적 목표 실행의 한계는 README 설명과 맞춘다.
- README에서 소개 애니메이션과 핵심 기능·전체 아키텍처를 계속 펼쳐 둔다.
- 연결 예시는 [실제 발견 기록과 원천 확인](../research/connection-discovery-evaluation.md#readme-연결-예시--2026-09-12)에 근거한다.
  가지는 필요한 데이터 역할을 뜻한다. 동시 실행·결합 성공·투자 판단의 완료로 표현하지 않는다.

## 기존 소개·홍보 자료

[소개 GIF](odeduck-hero.gif), [영상](odeduck-hero.mp4), [포스터](odeduck-hero-poster.webp),
[소셜 미리보기 SVG](odeduck-social-preview.svg)도 이 폴더에 보관한다.
소개 영상의 제작 기록은 [홍보 영상 문서](../promo/odeduck-agent-explainer.md)에 있다.
위 생성·출력 명령은 목록의 다이어그램 4개만 다룬다.
