---
name: odeduck-graph-engineering
description: "오대덕에서 서로 다른 공공데이터의 entity resolution, graph 모델, 식별자, provenance, 시간 유효성 또는 연결 신뢰도를 설계·검토할 때 사용한다. 그래프 DB 도입 여부와 무관하게 source record에서 검증 가능한 edge까지의 계약을 고정한다."
---

# 오대덕 그래프 엔지니어링

그래프 저장소를 고르기 전에 **어떤 질문에 어떤 증거로 답할지**를 고정한다. 같은 이름은 같은 대상이
아니며, 연결은 사실 자체가 아니라 특정 원천과 시점에서 관찰된 주장이다.

## 1. 질문을 먼저 고정한다

그래프가 답해야 할 실제 사용자 질문을 최소 다섯 개 쓴다. 각 질문에 시작점, 탐색 경로, 필요한 시간
범위, 허용 가능한 오탐을 붙인다. 질문 없이 node와 edge부터 제안하지 않는다.

완료 조건은 각 질문이 고객 행동 또는 제품 계약으로 이어지고, 표나 검색 색인만으로 충분한지까지
판정된 상태다. 경로 탐색이 핵심이 아니면 그래프 DB를 선택하지 않는다.

## 2. 네 층을 분리한다

| 층 | 역할 | 안정 키 |
| --- | --- | --- |
| Source | 데이터셋과 제공기관의 계약 | provider + dataset ID + revision |
| Record | 원천이 실제로 준 행·문서·응답 | source + record ID 또는 content hash |
| Entity | 여러 record가 가리킬 수 있는 실제 대상 | 오대덕 내부 canonical ID |
| Claim/Event | 누가 무엇을 언제 주장하거나 관찰했는지 | source record + relation + time |

원천 값을 canonical entity에 덮어쓰지 않는다. 속성별 survivorship 규칙과 원천별 값을 함께 보존한다.
사건, n항 관계, 시간에 따라 바뀌는 관계는 독립적으로 조회해야 하므로 node 또는 동등한 1급 record로
표현한다.

## 3. 식별 계약을 쓴다

연결마다 다음 중 하나를 붙인다.

1. `exact`: 법인·사업자·인허가·바코드·인증·모델처럼 범위가 정의된 식별자
2. `deterministic`: 둘 이상의 필드와 정규화 규칙이 모두 맞는 재현 가능한 규칙
3. `probabilistic`: 점수와 근거가 있는 후보 연결
4. `unresolved`: 후보는 있으나 확정할 수 없음

이름만 같은 연결은 `probabilistic` 이상으로 승격하지 않는다. 식별자의 발급기관, 적용 범위, 재사용
가능성, 폐기·변경 규칙을 기록한다. downstream이 확률 연결을 사실처럼 소비하지 못하도록 상태를
계약에 포함한다.

## 4. edge에 증거와 시간을 붙인다

각 edge 또는 claim에는 다음이 추적 가능해야 한다.

- 원천 dataset과 source record
- 추출한 원문 필드와 정규화 버전
- match 방법, confidence, 검토 상태
- `validFrom/validTo`: 현실에서 효력이 있었던 시간
- `observedAt`: 오대덕이 원천에서 본 시간
- 정정·삭제·대체된 claim과의 관계

공개일과 사건일을 섞지 않는다. 현재 원천에서 사라진 record도 이전 결정을 재현할 수 있도록 tombstone
또는 revision으로 남긴다. 원천 coverage 밖의 `미발견`은 부정 사실이 아니다.

## 5. 관계의 의미를 좁힌다

node는 정체성이 있는 명사, edge는 방향이 있는 동사로 이름 짓는다. `RELATED_TO`, `HAS`, `Entity`,
`Thing`처럼 질문을 설명하지 못하는 이름은 사용하지 않는다. 관계 방향은 읽었을 때 문장이 되어야 한다.

초고차수 node가 될 국가, 상태, 카테고리, 날짜는 탐색 대상이 아니라면 속성이나 별도 색인으로 둔다.
한 node의 예상 fan-out과 검색 expansion budget을 기록하고, 무제한 전이 추론은 허용하지 않는다.

## 6. gold set으로 검증한다

실제 원천 record에서 다음을 포함한 평가셋을 만든다.

- 명백한 동일 대상과 중복 record
- 이름은 같지만 다른 hard negative
- 주소·대표자·상호가 바뀐 temporal case
- 부모회사·사업장·브랜드·제품을 혼동하기 쉬운 case
- 정정·취소·삭제된 사건
- 식별자가 없어 unresolved로 남아야 하는 case

precision과 recall을 전체 평균만으로 보고하지 않는다. match 등급과 고객 행동별로 나눈다. 자동 신청,
판매 중지, 위험 경보처럼 행동 비용이 큰 경로는 별도의 precision 통과선을 정하고, 미달하면 사람 검토로
보낸다.

## 7. 저장 기술은 마지막에 고른다

다음 중 실제로 필요한 능력으로 선택을 설명한다.

- key lookup과 bounded join만 필요: 기존 저장소·색인 유지
- 반복적인 가변 길이 경로와 관계 중심 질의: property graph 후보
- 공개 표준 vocabulary와 외부 의미 호환: RDF 계열 후보
- 사건 재현과 정정 순서가 핵심: append-only event/claim store 후보

선택이 어렵게 되돌릴 수 있고 놀라우며 실제 trade-off가 있을 때만 ADR을 쓴다. 기술을 선택했다는 이유로
도메인 계약을 그 기술의 node/label 문법으로 축소하지 않는다.

## 강제 탈락 패턴

- 제목이나 설명의 공통 단어를 영구 edge로 저장한다. 검색 유사도는 관계 증거가 아니다.
- 같은 상호·기관·제품명을 하나의 canonical node로 합친다. 이름은 후보 생성에만 쓴다.
- confidence 숫자만 남기고 source record와 match 방법을 버린다. 점수는 provenance를 대체하지 않는다.
- 최신 값을 canonical 속성에 덮어써 정정 전 판단을 재현하지 못하게 한다.
- graph UI가 보기 좋다는 이유로 graph database를 선택한다. 저장 선택은 고정 질의와 benchmark가 한다.
- 모든 edge를 전이시켜 간접 관계를 사실로 표현한다. relation별 허용된 경로만 탐색한다.

## 결과 형식

1. 답해야 할 다섯 질문과 허용 오탐
2. entity, record, claim/event 사전
3. 식별자와 match 등급 표
4. edge별 provenance·시간 계약
5. 불확실성·정정·coverage 표현
6. gold set과 행동별 통과선
7. 저장 선택 또는 현 구조 유지 결정

용어가 확정되면 `CONTEXT.md`에 구현 세부 없이 즉시 반영한다. 되돌리기 어려운 저장·식별 결정만
`docs/adr/`에 남긴다. 설계 참고 자료는 `docs/agents/harness-sources.md`에 기록돼 있으며 실행에는 외부
스킬이나 그래프 제품이 필요하지 않다.
