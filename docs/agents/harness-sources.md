# Agent harness source review

검토일: 2026-09-04

이 문서는 `.agents/skills/`의 프로젝트 전용 하네스를 만들 때 검토한 외부 자료와 채택 범위를 기록한다.
하네스 실행에는 아래 저장소, 패키지 또는 서비스가 필요하지 않다. 외부 스킬 원문을 설치하거나 호출하는
대신 오대덕의 계약과 검증 단계에 맞게 원칙을 다시 작성했다.

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

외부 지표는 선택 당시의 참고값일 뿐 하네스 동작 조건이 아니다. 프로젝트 계약이 바뀌거나 하네스가
반복적으로 잘못된 결론을 만들 때만 원문을 다시 조사한다. 외부 업데이트를 자동 반영하지 않는다.
