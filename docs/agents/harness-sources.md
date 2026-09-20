# Agent harness source review

검토일: 2026-09-04

이 문서는 `.agents/skills/`의 프로젝트 전용 하네스를 만들 때 검토한 외부 자료와 채택 범위를 기록한다.
하네스 실행에는 아래 저장소, 패키지 또는 서비스가 필요하지 않다. 외부 스킬 원문을 설치하거나 호출하는
대신 오대덕의 계약과 검증 단계에 맞게 원칙을 다시 작성했다.

## 사용자용 Skill 배포 구조 — 2026-09-16

사용자 요청으로 gstack과 Matt Pocock의 설치·구성·갱신 방식을 검토했다. 외부 원본은 수정하거나
프로젝트에 설치하지 않았다. 아래에서 채택한 부분은 `skills/odeduck/`과 [사용 안내](../agent-skills.md)에 반영했다.

| 원천 | 확인한 구조 | 오데덕에 적용한 범위 |
| --- | --- | --- |
| [gstack 설치 안내](https://github.com/garrytan/gstack/blob/85b8c038fc0002a1549789ea018e924c1d335de4/README.md) · [session update](https://github.com/garrytan/gstack/blob/85b8c038fc0002a1549789ea018e924c1d335de4/bin/gstack-session-update) | Skill과 실행 도구를 함께 배포하고, team mode의 SessionStart hook이 명시된 `auto_upgrade` 설정 아래 갱신한다. 시간 제한·잠금·실패 처리가 별도 구현이다. | 버전별 실행 계약은 런타임에서 읽고 설치·갱신은 별도 절차로 둔다. 단순 `latest` URL을 자동 업데이트라고 설명하지 않는다. 전용 hook·상태 저장·자동 교체 구현은 도입하지 않았다. |
| [Matt Pocock 설치 안내](https://github.com/mattpocock/skills/blob/959a8e9f1edc3adbe2f7e3054bb6fbefa6696260/README.md) · [plugin manifest](https://github.com/mattpocock/skills/blob/959a8e9f1edc3adbe2f7e3054bb6fbefa6696260/.claude-plugin/plugin.json) | 작은 조합 가능한 Skill. `skills.sh`로 설치한 파일은 명시적으로 갱신하며, Claude의 관리형 플러그인은 별도 배포 경로다. | 사용자용 Skill을 개발용 하네스와 분리하고, 짧은 진입·판단 원칙에서 필요한 reference만 읽는다. `--skill odeduck`으로 사용자용 항목만 설치하고, 현재 경로의 갱신 의미를 명시한다. |
| [Skills CLI](https://github.com/vercel-labs/skills#install-a-skill) · [update 구현](https://github.com/vercel-labs/skills/blob/main/src/update.ts) | GitHub·로컬 경로 발견, 특정 Skill 설치, 이름별 `update`, 로컬 경로의 원격 버전 추적 제외 | 자체 설치 관리자를 추가하지 않고 기존 배포 도구를 사용한다. 임시 프로젝트 설치로 Skill과 reference 동봉을 확인한다. |

실제 최신 CLI help와 [명령 라우팅](https://github.com/vercel-labs/skills/blob/main/src/cli.ts)을 확인했다.
현재 `check`는 `update`와 같은 구현으로 전달되므로 읽기 전용 사전 점검으로 안내하지 않는다.
문서의 갱신 명령은 `update odeduck --project`이며 전역 설치는 `--global`을 사용한다.

`solve`·`advance_goal`은 보고·비교·계산 요청의 주요 진입점으로 유지한다. 배포 구조를 정리했다고
범용 목표 완주나 자동 신청 통합이 완료된 것으로 바꾸지 않는다. 원천 공유·결과 검토 권한과
실행·완료 계약은 실행 중인 CLI/MCP가 제공한다.

## 개발용 Skill 구조 — 2026-09-16

[OpenAI의 스킬·프롬프트 가이드](https://developers.openai.com/blog/rethinking-skills-and-prompts-for-gpt-6-astra)
(2026-09-11)를 기준으로 프로젝트 소유 지침을 재검토했습니다. 짧고 구체적인 호출 설명, 필요한
상세 문서만 읽는 진입 구조, 변경 영향에 맞는 맥락·검증을 적용했습니다.

사업 검증과 데이터 설계는 호출 목적이 달라 두 진입점을 유지했습니다. 공통 작업 원칙은 저장소
지침과 [확장 안내](../agent-skills.md#개발용-skills를-확장할-때)에 두고, 각 본문의 전문 절차는
작업별 reference로 옮겼습니다. 기존 식별·근거·평가 기준과 진행 중이던 실증 지침을 보존했습니다.
사업 조사에서는 고정 경쟁사 수와 일률적인 실험 기간 대신 실제 대안과 구매 주기를 기준으로 합니다.

특정 모델·제품 버전이나 설치된 서버 상태를 스킬에 고정하지 않습니다. 제품 계획기의 실행 지침과
개발 스킬의 책임을 구분하며, 본문 정리가 실행 능력 향상을 입증한다고 주장하지 않습니다.
새 외부 패키지·스킬 설치나 자동 업데이트 서비스는 추가하지 않았습니다.

## Credits and licenses

아래 프로젝트의 아이디어와 공개 지침을 참고했다. 원문을 프로젝트에 vendor하지 않았고 실행 의존성도
없지만, 기여자가 분명히 보이도록 저작권 표시와 라이선스 원문을 남긴다.

- **Marketing Skills / customer-research** — Copyright © 2025 Corey Haines,
  [MIT License](https://github.com/coreyhaines31/marketingskills/blob/main/LICENSE),
  [source](https://github.com/coreyhaines31/marketingskills)
- **Lenny Skills** — Copyright © 2025 Refound AI,
  [MIT License](https://github.com/RefoundAI/lenny-skills/blob/main/LICENSE),
  [source](https://github.com/RefoundAI/lenny-skills)
- **ADHD divergent ideation skill** — Copyright © 2026 ADHD contributors,
  [MIT License](https://github.com/UditAkhourii/adhd/blob/main/LICENSE),
  [source](https://github.com/UditAkhourii/adhd)
- **Neo4j Skills / modeling** — Copyright © 2026 Neo4j Contrib,
  [MIT License](https://github.com/neo4j-contrib/neo4j-skills/blob/main/LICENSE),
  [source](https://github.com/neo4j-contrib/neo4j-skills)
- **PROV-O** — W3C Provenance Working Group,
  [W3C Recommendation](https://www.w3.org/TR/prov-o/)

검토했지만 채택하지 않은 자료도 아래 표에 남긴다. 해당 자료의 저작권이나 보증을 오대덕에 이전한다는
뜻은 아니다. 링크된 원문과 라이선스가 우선한다.

## 사업 검증

| 자료 | 당시 공개 지표 | 채택한 원칙 | 제외한 부분 |
| --- | --- | --- | --- |
| [Corey Haines customer-research](https://skills.sh/coreyhaines31/marketingskills/customer-research) | skills.sh 88.9K installs, GitHub 46,851 stars, MIT | 실제 고객 언어, 촉발 사건, 현재 대안, confidence 등급 | 특정 리뷰 플랫폼과 마케팅 후속 스킬 의존 |
| [RefoundAI Lenny skills](https://github.com/RefoundAI/lenny-skills) | GitHub 1,306 stars, MIT | 문제 긴급도, demand-side 경쟁, can't/won't, 수작업 첫 결제 | 인용문과 외부 artifact library 의존, $100M 획일 기준 |
| [Udit Akhourii ADHD](https://skills.sh/uditakhourii/adhd/adhd) | skills.sh 5.6K installs, GitHub 4,044 stars, MIT | 생성과 평가 분리, 격리 관점 발산, 함정 제거 후 수렴 | 고정 10회 agent 호출과 Node CLI 의존 |
| [AI Labs startup-validator](https://skills.sh/ailabs-393/ai-labs-claude-skills/startup-validator) | skills.sh 1.4K installs, GitHub 445 stars, MIT | 채택 없음 | 검색 횟수와 TAM 산출을 품질 대용치로 강제해 제외 |

`ayghri/i-have-adhd`도 검토했으나 이는 독자의 실행을 돕는 출력 형식 스킬이다. 후보의 새로움이나 사업
근거를 강화하지 않으므로 사업 하네스에는 넣지 않았다.

## 그래프 엔지니어링

| 자료 | 당시 공개 지표 | 채택한 원칙 | 제외한 부분 |
| --- | --- | --- | --- |
| [Neo4j modeling skill](https://skills.sh/neo4j-contrib/neo4j-skills/neo4j-modeling-skill) | skills.sh 752 installs, GitHub 106 stars, MIT | query-first 모델링, 의미 있는 관계, n항 관계, uniqueness, supernode 경계 | Neo4j·Cypher·Aura에 종속된 DDL과 버전 지침 |
| [Master data and entity resolution](https://skills.sh/vaquarkhan/data-engineering-agent-skills/master-data-and-entity-resolution) | skills.sh 8 installs, GitHub 40 stars, MIT | canonical entity, match 단계, survivorship, unresolved 경로 | 낮은 채택도 때문에 독립 스킬로 설치하지 않음 |
| [W3C PROV-O](https://www.w3.org/TR/prov-o/) | W3C Recommendation | entity/activity/agent provenance와 파생 관계를 보존한다는 기준 | RDF/OWL 저장 형식을 기본값으로 강제하지 않음 |

`omer-metin/skills-for-antigravity@graph-engineer`는 skills.sh 설치 수가 28이었고, 저장소의 다수
`SKILL.md` frontmatter가 파싱되지 않았다. 오대덕의 자동 호출 안정성을 해칠 수 있어 제외했다.

## 갱신 규칙

### 2026-09-08 INTENT 기준 방법론 비교

[방법론 적용 검토](../research/intent-aligned-methodology-review-2026-09-08.md)와
[온톨로지 1차 자료](../research/ontology-methods-primary-sources-2026-09-08.md)에 채택·유보 이유를 기록했다.
질문 중심 모델링, SKOS의 개념/식별 구분, DCAT/SOSA의 제공형·관측 구분, SHACL의 명시적 제약,
Aurum의 컬럼 수준 후보 발견, LBD의 가설/증명 분리, EDA의 탐색 순서를 설계 참고로 채택한다.
전면 RDF/OWL/GraphRAG runtime, 전역 동일성 자동 병합, 특정 질문 정답 recipe는 지금 도입하지 않는다.
위 표준 준수나 도입 효과를 검증한 것은 아니며 외부 스킬/패키지를 설치하지 않았다.

같은 검토의 §8에는 사용자 요청으로 Karpathy의 [LLM Wiki](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f)와
[autoresearch](https://github.com/karpathy/autoresearch)를 추가했다. 원천과 파생 지식 분리, 재사용·모순 점검,
수정 대상과 평가 분리, 동등한 결과의 단순화 우대를 설계 참고로 채택한다. LLM 요약의 사실 자동 승인,
무제한 실행·권한 해제와 실측 없는 유지비 주장은 채택하지 않는다. 런타임이나 실행 정책은 변경하지 않았다.

### 2026-09-07 사업 조사에 적용한 외부 근거

[공공입찰 사업 재검토](../research/public-data-business-pick-2026-09-07.md)에서는
[조달청 통합공개 계약](https://www.data.go.kr/data/15129459/openapi.do)의 원천 번호 관계와
누락 경고를 채택하고, 제목 유사도만으로 단계 연결을 확정하는 접근을 제외했다.
[비드프로](https://www.bidpro.co.kr/main.asp)와 [나입찰](https://naipchal.kr/)의 공개 제품·요금은
대체재와 상품 존재의 근거로 채택했다. 공급사 홍보를 성능·매출 증명으로, 경쟁사 가격을 우리 제품의
지불의사로 사용하는 해석은 제외했다. 외부 스킬 설치나 하네스 실행 정책 변경은 하지 않았다.

외부 지표는 선택 당시의 참고값일 뿐 하네스 동작 조건이 아니다. 프로젝트 계약이 바뀌거나 하네스가
반복적으로 잘못된 결론을 만들 때만 원문을 다시 조사한다. 외부 업데이트를 자동 반영하지 않는다.

### 2026-09-20 활용신청 상태 검증

[browser-use/jev-ultrafast의 실행 설계](https://github.com/browser-use/jev-ultrafast/blob/1231850a0bf1a0c0341fe408ef1668dbbfdfac46/docs/design.md)를 참고해
관측한 폼의 제출 직전 재검증, 1회 실행, 상태에 따른 제한시간 대기를 채택했다.
`internal/portal/apply_state.go`는 이 원칙을 기존 결정적 신청 경로에 구현하며 숨은 필드값을 브라우저 밖으로 반환하지 않는다.
외부 모델·브라우저 패키지, 확률적 대상 선택은 도입하지 않았다. 로컬 REST/ODCloud fixture로 검증하며
외부 프로젝트의 속도 측정값을 오데덕 성능 근거로 사용하지 않는다.
