# 오대덕 graph database 적합성 재평가

평가일: 2026-09-04

## 결정

**현재 semantic/planned retrieval을 graph database로 교체하지 않는다.** 둘은 대체 관계가 아니다.
semantic/lexical 검색은 아직 관계를 모르는 96,866개 카탈로그 항목에서 후보를 찾고, graph는 식별과
검증이 끝난 관계를 반복 탐색하는 데 적합하다. 지금 영구 저장할 수 있는 것은 대부분 metadata 기반
`candidate`이며, 이를 edge로 물질화하면 아직 확인하지 않은 field·grain·값 교집합을 사실처럼 굳힌다.

향후 제품 구조는 필요해질 경우 다음 두 층을 병행한다.

```text
goal → lexical/planned/semantic retrieval → inspect + sample verification
                                           ↓ verified only
                              provenance-aware claim graph
```

현재 판정은 `현 구조 유지`다. graph database 도입은 아래 전환 gate를 모두 통과한 뒤 별도 ADR로
결정한다.

## 먼저 답해야 할 사용자 질문

| 질문 | 시작점과 경로 | 현재 가장 적합한 구조 | 오탐 비용 |
| --- | --- | --- | --- |
| 이 목표에 관련된 공공데이터는 무엇인가? | 자연어 목표 → 역할별 dataset 후보 | lexical + planned + optional semantic retrieval | 관련 자료 누락 또는 잡음 |
| 전혀 다른 분야에서 보완할 dataset은 무엇인가? | Anchor → 역할별 검색 → 최대 3개 선택 | query-time bounded composition | 약한 조합을 유용한 연결로 오인 |
| 두 dataset이 실제로 join 가능한가? | dataset → operation/file schema → field와 sample overlap | `inspect_dataset` + bounded sample | 잘못된 코드·공간·시간 grain 결합 |
| 이 연결의 공식 근거는 무엇인가? | 후보 → source record → 검증 표본 | provenance가 포함된 connection record | 출처 없는 관계를 사실로 사용 |
| 한 공급업체의 제품·사업장·사건·조치는 무엇인가? | customer entity → facility → product → event → action | **미래 claim graph 후보** | 다른 회사·제품에 판매중지/CAPA 적용 |
| 정정 전 판단을 재현할 수 있는가? | claim → source revision → observed/valid time | **미래 temporal claim store 후보** | 삭제·정정 뒤 감사 경로 상실 |

앞의 네 질문은 현재 제품 계약이고, 뒤의 두 질문은 고객 원장과 검증된 edge가 생긴 뒤의 업무다. 지금은
뒤의 두 질문에 사용할 고객 record, canonical entity, 검증 edge가 없다.

## 현재 구조에서 확인한 사실

- `odeduck catalog info --format json`은 96,866개 항목을 보고했다: FILE 84,553, REST 7,223,
  LINK 4,784, 미확인 306. snapshot은 stale이 아니었다.
- 로컬 `catalog.json`은 약 113MB다. `catalog-semantic.gob`은 존재하지 않았다.
- `식품 공급업체 리콜 부적합` 검색은 기본 설정에서도 `semantic.status=not-indexed`와 경고를 내고
  lexical 결과로 폴백했다. 의미 검색을 사용했다고 가장하지 않는다.
- 같은 바이너리의 단일 관찰에서 기본 경로는 2.07초, `--semantic=false`는 1.44초였고 최대 RSS는 각각
  약 405MB였다. 반복 benchmark가 아니므로 성능 주장으로 일반화하지 않는다.
- 과거 11,902개 카탈로그 평가에서 connection discovery의 대부분 지연은 세 차례 agent CLI 호출
  97~108초에서 발생했다. graph lookup이 지배 병목이라는 증거가 없다.
- 95,951개 semantic index의 이전 실험은 무변경 refresh 1.9초와 약 2.9GiB RSS를 관찰했다. 기존
  ADR은 다음 저장 seam을 graph/vector database가 아니라 packed mmap으로 잡고 있다.

현재 단순 검색의 메모리·시작 지연은 개선 여지가 있다. 그러나 이는 113MB JSON 전체 decode와 선택적
vector 파일 표현의 문제다. key/value lookup, inverted index, packed mmap 또는 embedded SQL을 서로
benchmark하는 것이 graph database 도입보다 직접적인 다음 실험이다.

## graph database가 지금 품질을 높이지 않는 이유

### 1. graph는 모르는 edge를 발견하지 않는다

현재 discovery의 값은 호스트 모델이 목표를 2~8개 역할로 분해하고, lexical/semantic recall로 서로 다른
dataset을 찾는 데 있다. graph traversal은 이미 저장된 관계만 따라간다. metadata에 공통 단어가 없고
검증 edge도 없는 두 dataset을 graph database가 스스로 연결하지 않는다.

### 2. 핵심 병목은 entity resolution이다

전국 등록공장처럼 회사명과 주소만 있고 사업자번호가 없는 원천, 리콜처럼 제품·모델·바코드 계약이
분야마다 다른 원천을 먼저 해결해야 한다. 같은 이름을 하나의 node로 합치면 graph가 검색보다 더
자신 있게 틀린 결과를 반환한다. 저장 엔진은 canonical entity, match 등급, survivorship 규칙을 대신
정하지 않는다.

### 3. 현재 edge는 의도적으로 candidate다

`ConnectionCandidate`는 `entity/spatial/temporal/proxy`와 예상 key를 담지만, 실제 field와 값 교집합을
증명하지 않았다고 계약에 명시한다. 현재 spec도 최종 카드를 최대 세 개로 제한하고
`inspect_dataset → sample`을 요구한다. 전역 graph는 이 증거 경계를 약화한다.

### 4. 외부 실행 의존성이 제품 원칙과 충돌한다

Neo4j 같은 server graph database는 별도 설치, process, schema migration, backup과 장애 seam을 만든다.
공식 [Neo4j 모델링 가이드](https://neo4j.com/docs/getting-started/data-modeling/guide-data-modeling/)도
사용 사례와 질문에서 모델링을 시작한다. 현재는 graph가 필요한 반복 질의와 검증 edge가 먼저 존재하지
않는다.

### 5. provenance는 graph 제품 없이도 먼저 설계할 수 있다

[W3C PROV-O](https://www.w3.org/TR/prov-o/)의 핵심처럼 entity, activity, agent와 파생 관계를 구분하는
계약은 유용하지만 RDF 저장소를 요구하지 않는다. 오대덕은 source record, canonical entity,
claim/event, observed/valid time을 Go 구조체와 기존 로컬 저장소에서도 먼저 보존할 수 있다.

## 도입 전환 gate

다음을 모두 만족할 때만 graph database 또는 embedded graph store를 비교 구현한다.

1. 유료 고객 workflow가 `entity → facility → product → event → action` 같은 3-hop 이상 경로를 반복
   질의하고, bounded join 구현이 실제 복잡성 또는 지연 병목이 된다.
2. metadata 후보가 아니라 source record와 표본값으로 검증된 edge가 최소 100,000개 쌓인다.
3. 다섯 개 이상의 고정 graph query와 행동별 오탐 허용치가 gold set으로 존재한다.
4. exact/deterministic/probabilistic/unresolved match 계약과 정정·시간 유효성 규칙이 안정됐다.
5. 별도 server 설치 없이 기존 CLI/MCP 계약, Windows/macOS/Linux 배포, atomic snapshot과 rollback을
   유지할 구현 후보가 있다.
6. 현 구조와 비교해 query correctness를 유지하면서 p95, peak RSS 또는 구현 복잡성 중 측정 가능한
   이득을 보인다.

이 gate를 통과하더라도 semantic retrieval은 유지한다. graph는 검증된 사실의 projection이고,
semantic은 아직 모델링되지 않은 후보를 찾는 recall surface다.

## 지금 할 일

1. 검색 품질은 lexical/planned/semantic의 gold query로 계속 측정한다.
2. 실제로 inspect와 sample verification을 통과한 connection을 source record와 함께 기록할 작은
   `verified connection ledger` 계약부터 설계한다.
3. catalog 시작시간과 메모리가 사용자 문제로 확인되면 JSON, packed mmap, embedded indexed store를
   같은 query corpus로 benchmark한다.
4. 식품 CAPA 파일럿처럼 고객 원장이 들어오는 첫 workflow에서 entity resolution gold set을 만든다.

graph UI나 database부터 만들지 않는다. 검증된 ledger가 쌓이면 동일 데이터를 표, embedded store,
property graph로 각각 projection해 실제 질의 비용을 비교할 수 있다.

## 관련 결정

- [ADR 0002 — model-planned hybrid retrieval](../adr/0002-model-planned-hybrid-catalog-search.md)
- [ADR 0004 — bounded composition](../adr/0004-broad-catalog-bounded-composition.md)
- [ADR 0005 — incremental semantic index](../adr/0005-incremental-semantic-index-distribution.md)
- [교차 데이터 연결 발견 v1 평가](connection-discovery-evaluation.md)
- [semantic search 평가](semantic-search-evaluation.md)
- [그래프 엔지니어링 하네스](../../.agents/skills/odeduck-graph-engineering/SKILL.md)
