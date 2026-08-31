# 교차 분야 데이터 연결 발견에서 노이즈를 줄이는 방법

## 조사 질문과 결론

데이터셋 검색·join discovery·data-lake recommendation 시스템은 semantic similarity가 놓치는 보완적 연결을 어떻게 찾고, 잘못된 join과 조합 폭발을 어떻게 억제하는가?

결론은 **전체 데이터셋의 의미 유사도**와 **두 데이터셋을 잇는 Connection Edge**를 분리해야 한다는 것이다. 교차 분야 연결은 두 노드의 주제가 비슷해서가 아니라, 좁은 엔티티·공간·시간 키를 공유하면서 서로 다른 속성이나 역할을 제공할 때 생긴다. 따라서 semantic similarity는 후보 회수 신호일 뿐이고, Verified Connection은 방향성 있는 값 포함률, 키 의미, 공간·시간 grain, cardinality와 실제 표본 join을 모두 통과해야 한다.

OpenDataCTL의 약 1.2만 카탈로그에는 외부 vector DB나 전역 all-pairs 그래프가 필요하지 않다. 현재의 모델 계획 + 결정적 로컬 검색으로 역할별 후보를 좁힌 뒤, 소수 후보만 `describe_api`와 `call_api`로 검증하는 bounded cascade가 맞다. 다만 현재 카탈로그에는 출력 column과 값 profile이 없으므로, **카탈로그만 보고 `structurally_verified` 또는 `sample_verified`라고 부르는 것은 불가능**하다. 이 증거가 없으면 `candidate` 또는 `blocked`로 남기고 Abstention해야 한다.

## 1. 문헌에서 반복되는 발견 원리

### 1.1 여러 신호를 한 점수로 뭉개지 않고 역할을 나눈다

[Aurum](https://raulcastrofernandez.com/papers/icde18-aurum.pdf)은 column을 노드로 삼고 schema-name similarity, content similarity, PK/FK candidate를 서로 다른 edge로 저장한다. 한 번 읽어 만든 MinHash·TF-IDF·cardinality·distribution·type profile에서 LSH로 후보를 만들고, content-similar 후보 중 uniqueness가 1에 가까운 column만 PK/FK 후보로 승격한다. 핵심은 similarity edge와 join edge가 같은 주장이 아니라는 점이다.

[D3L](https://arxiv.org/abs/2011.10427)은 column name의 q-gram, 값 token, 값 format 정규식, word embedding, 수치 분포를 별도 evidence로 색인하고 결합한다. embedding은 다섯 신호 중 하나일 뿐이다. 또한 먼저 찾은 top-k 표 밖에서도 값 overlap과 subject-attribute 조건이 있는 edge를 따라 join path를 확장해, 직접 유사도가 약하지만 target의 새 attribute를 채우는 표를 발견한다. 즉 Bridge Node는 Anchor Node와 문서 전체가 비슷할 필요가 없다.

[SANTOS](https://doi.org/10.1145/3588689)는 column의 의미만 맞추는 것보다 column 간 **relationship semantics**까지 보존해야 같은 맥락의 표를 찾을 수 있음을 보였다. intent column에서 시작하는 semantic tree를 후보 표의 subtree와 비교하고, 흔한 상위 type은 덜 구체적이므로 감점한다. 외부 지식베이스의 open-data coverage가 낮을 때는 같은 column 값과 값 쌍의 co-occurrence로 synthesized KB를 만들되, 모든 column pair를 관계로 취급하지 않고 unary functional dependency가 있는 pair만 의미 있는 관계 후보로 남긴다.

SANTOS 자체는 table union search이므로 이 신호를 join validity 증명으로 전용할 수는 없다. 여기서 가져올 수 있는 것은 column 하나의 유사도보다 **관계의 문맥**을 함께 보라는 후보 생성 원리뿐이다.

이 세 결과를 OpenDataCTL 언어로 옮기면 다음과 같다.

- semantic similarity와 schema-name similarity는 Data Node 회수에 쓴다.
- Connection Edge 후보는 별도로 만들며, edge 종류를 `entity`, `spatial`, `temporal`, `proxy`로 명시한다.
- 전체 주제 유사도보다 **join key 쪽의 좁은 의미 일치**를 요구한다.
- Bridge Node의 payload와 역할은 Anchor Node와 달라야 한다. 같은 사실을 다른 기관이 복제한 것은 보완성이 아니다.
- 흔한 type, 흔한 값, 제목·기관·category 중복은 연결 근거가 아니라 감점 또는 중복 제거 신호다.

### 1.2 보완성은 “공통 key + 새 payload”로 찾는다

[JOSIE](https://www.cs.toronto.edu/~fnargesian/JOSIE_Overlap_Set_Similarity_Search_for_Finding_Joinable_Tables_in_Data_Lakes.pdf)는 한 query column과 가장 많은 distinct value를 공유하는 column을 top-k로 찾는다. 논문의 예시는 대기오염 시설과 정치후원 데이터를 우편번호로 연결한다. 표의 주제는 다르지만 join key의 교집합이 있어 새 분석이 가능하다. 이 사례가 semantic-nearest-neighbor가 아니라 Connection Edge가 필요한 이유를 잘 보여준다.

[Juneau](https://www.cis.upenn.edu/~zives/research/Finding_Related_Tables_in_Data_Lakes_for_Interactive_Data_Science.pdf)는 search task별 relatedness를 다르게 정의한다. linkable-data 검색에서는 key/FK 관계와 row overlap을 찾고, feature-extraction 검색에서는 row overlap과 **new column rate**를 함께 본다. 평가에서 keyword search와 단순 LSH는 겹치는 내용만 돌려주어 complementary data가 있는 joinable result를 top-k에 내지 못했지만, relation mapping·profile·provenance를 조합한 Juneau는 의미 있는 linkable table을 반환했다.

따라서 OpenDataCTL의 complementarity는 별도 gate여야 한다.

1. 적어도 하나의 유효한 Connection Edge가 있다.
2. Bridge Node에는 Anchor Node에 없는 측정값·제약·선행/결과 신호·공급/수요 관점이 있다.
3. 그 새 payload가 사용자의 결정이나 연구 측정을 어떻게 바꾸는지 한 문장으로 설명할 수 있다.
4. 2와 3이 없으면 단순 joinable copy이지 Connection Hypothesis가 아니다.

상관이나 downstream 성능은 task가 정해졌을 때의 강한 usefulness 신호다. [Correlation Sketches](https://arxiv.org/abs/2104.03353)는 join key를 공유하는 표 중 query measure와 상관된 새 numerical column을 찾아, overlap만으로 순위를 매긴 것보다 목적에 맞는 후보를 찾는다. 그러나 범용 discovery에는 항상 target variable이나 학습 task가 있는 것이 아니므로, correlation이나 model lift를 보편적 자동 usefulness 점수로 사용할 수는 없다.

## 2. 잘못된 join을 거르는 증거

### 2.1 schema와 column semantics

같은 문자열 값이 있다는 사실만으로 같은 개념은 아니다. `2026`, `서울`, `중구`처럼 흔한 값은 서로 무관한 column에서도 만난다. 후보 key마다 다음을 기록해야 한다.

- 공식 field 이름·설명·data type과 어느 operation의 입력/출력인지
- 값이 나타내는 semantic type: 사람/사업체/시설/행정구역/좌표/날짜 등
- identifier의 namespace와 version: 법정동코드인지 행정동코드인지, 사업자 ID인지 시설 ID인지
- exact equality인지, normalization인지, crosswalk·집계·근접 매칭을 쓰는 proxy인지
- proxy라면 변환식, 정보 손실, 예상 cardinality와 검증 방법

D3L의 name/value/format/embedding/distribution 결합과 Juneau의 type·unique values·range·pattern을 쓰는 data profile은 서로 다른 표기와 결측 metadata를 견디기 위한 **후보 신호**다. 반면 SANTOS의 intent-rooted relationship match는 개별 column type이 우연히 같은 false positive를 줄이는 정밀 신호다. OpenDataCTL에서도 “field 이름이 비슷함”과 “같은 code system임”을 서로 다른 evidence로 보존해야 한다.

### 2.2 inclusion dependency와 값 overlap

query/anchor key 집합을 $A$, bridge key 집합을 $B$라고 하면 최소한 다음 방향성 지표가 필요하다.

- `distinct_overlap = |A ∩ B|`
- `anchor_containment = |A ∩ B| / |A|`
- `bridge_containment = |A ∩ B| / |B|`
- `jaccard = |A ∩ B| / |A ∪ B|`

JOSIE가 강조하듯 join으로 실제 매치되는 distinct key 수인 intersection size는 직관적인 top-k 회수 기준이고, query가 고정됐을 때 containment와 같은 순위를 낸다. 반면 크기가 크게 다른 표에서는 Jaccard가 작은 집합을 편향하거나 recall을 잃을 수 있다. D3L도 exact IND의 전역 발견은 all-against-all column 공간 때문에 비현실적이라 보고, LSH overlap으로 partial IND 후보를 만든다.

하지만 높은 containment도 충분조건은 아니다. [NextiaJD](https://arxiv.org/abs/2305.19629)는 containment만 높으면 `Store`와 백만 건 규모의 영화 `Movie`처럼 몇 개의 우연한 동명이 높은 점수를 받는 예를 보이고, 다음 distinct-cardinality 비율을 함께 사용해 이런 false positive를 낮춘다.

\[
K(A,B)=\frac{\min(|A|,|B|)}{\max(|A|,|B|)}
\]

OpenDataCTL은 이 값을 hard truth로 쓰기보다, containment가 높은데 $K$가 극단적으로 낮은 후보를 정밀 검증 대상으로 내리거나 거부하는 guard로 써야 한다. 고유 ID의 정상적인 PK→FK 관계도 양쪽 row 수는 크게 다를 수 있으므로 아래 중복·방향성 지표와 함께 해석해야 한다.

### 2.3 cardinality와 join 폭증

Aurum의 approximate-key 검사는 `distinct values / rows`가 1에 가까운지를 본다. Juneau도 approximate key가 한 값당 평균 몇 tuple을 만드는지 측정하고, linkable task에서 one-to-many/FK 방향을 명시한다. 따라서 표본 join 검증은 최소 다음을 기록해야 한다.

- 양쪽 `row_count`, `distinct_count`, `null_rate`, `uniqueness = distinct_count / non_null_rows`
- `matched_rows`, 양방향 distinct/row match rate
- `joined_rows`
- `left_expansion = joined_rows / matched_left_rows`
- `right_expansion = joined_rows / matched_right_rows`
- 관측된 관계: `1:1`, `1:N`, `N:1`, `N:M`

의도하지 않은 `N:M`, 큰 expansion, 높은 null/duplicate 비율은 억지 연결의 전형적인 신호다. 집계 후에만 안전하다면 proxy edge로 내리고 aggregation key·함수·손실을 명시한다. JOSIE도 distinct set overlap은 실제 bag multiplicity와 join 결과 크기를 반영하지 못한다고 한계로 남겼으므로, overlap 점수를 join 폭증 검사 대신 써서는 안 된다.

### 2.4 공간·시간 extent와 grain

공간·시간은 “둘 다 지역/날짜 field가 있다”보다 강한 계약이 필요하다. [W3C DCAT 3](https://www.w3.org/TR/vocab-dcat-3/)는 dataset의 spatial/temporal coverage와 함께 `spatialResolutionInMeters`, `temporalResolution`, update frequency를 별도 속성으로 정의한다. [OGC API - Records](https://docs.ogc.org/is/20-004r1/20-004r1.html)는 bounding box와 datetime **extent가 교차**하는 record를 후보로 회수한다. 이는 좋은 1차 filter이지만 grain 호환성이나 실제 join을 보장하지 않는다.

[Auctus](https://www.vldb.org/pvldb/vol14/p2791-castelo.pdf)는 실제 augmentation search에서 column을 categorical·numerical·spatial·temporal로 profile하고, 공간 geometry·시간 range·값 MinHash로 겹치는 후보를 색인한 뒤 공간·시간 resolution을 맞춰 결합한다. 자동 추론된 type을 사용자가 교정할 수 있게 한 점도 중요하다. 즉 extent/overlap은 회수 신호이고, grain 정렬과 type 확인은 그 다음 검증 단계다.

각 edge에는 다음 계약이 필요하다.

- 공간: coverage, 좌표계 또는 행정구역 code system·version, 관측 단위(점/격자/행정동/시군구), resolution
- 시간: coverage 시작·끝, timezone/calendar, timestamp 의미(발생/접수/집계/수정), resolution(시/일/월/연), update frequency
- roll-up/down: 어느 쪽을 어떤 단위로 집계하는지, 경계 변경·결측 구간을 어떻게 처리하는지

extent가 겹치지 않으면 reject한다. 같은 grain이거나 공식 crosswalk/명시적 집계로 한쪽을 더 거친 grain에 맞출 수 있으면 plausible하다. finer-grain 값을 근거 없이 복제하는 downscaling, 행정동/법정동 혼동, 월별 stock과 일별 event의 직접 equality는 reject한다. 카탈로그 metadata에 이 항목이 없으면 이름에서 추측하지 않고 표본과 공식 명세를 읽을 때까지 `unknown`이다.

## 3. 조합 폭발을 막는 cascade

Aurum은 “한 번 profile → LSH 후보 → 후보 관계 판정”의 two-step build로 all-pairs를 피한다. JOSIE는 inverted index에서 top-k의 현재 k번째 overlap을 threshold로 삼아, 그 점수를 이길 수 없는 posting list와 candidate를 prefix/position bound로 잘라낸다. Juneau는 cheap하고 selective한 description/provenance/profile 신호로 후보를 먼저 줄이고, 비싼 relation mapping과 값 비교를 남은 후보에만 적용한다. 이 공통 구조에서 도출되는 OpenDataCTL용 설계는 다음과 같다.

| 단계 | 하는 일 | 통과 조건 | 시작 예산 |
|---|---|---|---|
| 0. 역할 계획 | 질문을 직접 대상, 선행/결과, 제약, 수요, 공급 등 2–8개 축으로 분해 | 각 축이 서로 다른 역할을 가짐 | 현재 `catalog_search` 계약 유지 |
| 1. 고재현율 회수 | title/org/category/description lexical 검색 + 선택적 flat vector scan | query 축에 근거가 있고 callable 여부가 보임 | 축당 top 5, 중복 제거 후 최대 30 nodes |
| 2. 작은 beam 합성 | Anchor→Bridge만 조합하고 같은 역할·유사 제목·동일 payload clone 제거 | edge kind와 예상 key, Incremental Value를 둘 다 쓸 수 있음 | 역할별 top 3, 총 12 edge hypotheses |
| 3. 구조 검증 | 최대 12 nodes의 공식 명세에서 field/type/key/grain 확인 | 모든 필수 edge가 `structurally_plausible`; proxy는 변환 명시 | 전체 `describe_api` 12회 이하 |
| 4. 표본 검증 | 공통 지역·기간을 먼저 정하고 실제 응답을 bounded join | overlap·match·uniqueness·expansion과 grain gate 통과 | 카드 최대 3개, node당 제한 행 수 |
| 5. 재순위/중단 | evidence gate 통과 후 usefulness, 역할 다양성, serendipity 순 | 유효 카드가 없으면 Abstention | 최종 카드 최대 3개 |

숫자는 문헌의 보편 임계값이 아니라 현재 2–8축·점진적 공개 구조에 맞춘 **검증할 시작 예산**이다. benchmark에서 recall이나 latency가 나쁘면 바꾸되, all-pairs로 되돌아가지는 않는다. 다단계 카드에서는 edge별 상태를 저장하고 카드 상태는 모든 필수 edge의 최저 상태에서 파생해야 한다. 접근·승인 실패는 반증이 아니므로 `blocked`, 불일치는 `rejected`다.

## 4. usefulness·serendipity와 평가

### 4.1 자동으로 평가할 수 있는 것

자동 평가는 주장보다 좁아야 한다.

- **검색**: 봉인된 query별 known relevant node/edge의 Recall@k, Precision@k, MAP@k. SANTOS도 binary ground truth로 MAP@k·Precision@k·Recall@k를 사용한다.
- **schema matching**: gold column pair에 대한 Precision/Recall. [Valentine](https://arxiv.org/abs/2010.07386)은 joinable/unionable 시나리오, schema·instance noise와 정답 column mapping을 분리해 평가한다.
- **edge 품질**: 존재하는 field인지가 아니라 고정 sample fixture에서 normalization/transform 후 `matched_rows`, 양방향 match rate, uniqueness, null rate, expansion, grain compatibility를 검사한다.
- **task가 명시된 경우의 incremental value**: correlation confidence, 예측 성능 delta, 새 row/column rate처럼 task-specific metric을 쓸 수 있다. task가 없으면 자동 점수로 “사업 가치”를 만들지 않는다.
- **안전성**: 존재하지 않는 PK/field 비율, proxy를 exact로 잘못 표기한 비율, no-connection 반례의 Abstention rate, edge 및 카드 전체 precision을 보고한다.

live API의 첫 페이지는 기간·지역이 달라 overlap을 과소평가할 수 있으므로, 두 API가 공유하는 region/time intersection을 먼저 고른 뒤 fixture를 저장해야 한다. exact/proxy 판정 fixture는 모델이 아니라 사람이 검토한 code system·변환 규칙을 사용한다.

### 4.2 사람이 평가해야 하는 것

“뜻밖”은 품질 gate를 대신할 수 없다. 3천 명 이상을 대상으로 한 [대규모 serendipity 사용자 평가](https://www.comp.hkbu.edu.hk/~lichen/download/p240-chen.pdf)는 perceived serendipity가 novelty·unexpectedness뿐 아니라 relevance와 timeliness에도 의존함을 보였다. 그러므로 novelty는 관련성·edge validity·Incremental Value를 통과한 카드 사이의 보조 tie-breaker여야 한다.

봉인 평가에서 두 명 이상의 평가자가 카드를 blind로 보고 다음을 각각 0–3점으로 매긴다.

- 질문 관련성
- 보완성: 둘을 결합해야만 생기는가
- 실행 가능성: key·grain·접근 조건이 구체적인가
- 근거성: 공식 명세와 fixture가 주장을 지지하는가
- 의사결정 usefulness
- serendipity: 관련되고 유용하면서 예상 밖인가

평가자는 카드별 precision과 query별 best-of-three를 함께 보아야 한다. 최고 카드 하나만 보고 나머지 억지 카드를 숨기면 안 된다. 0–3 순서형 평정은 weighted kappa 또는 ICC로 합의를 보고, 개선은 동일 query의 paired delta로 비교한다. no-connection 반례, 같은 주제지만 join 불가능한 hard negative, 값 일부만 우연히 겹치는 hard negative를 반드시 포함한다.

## 5. OpenDataCTL에서 가능한 것과 불가능한 것

### 지금 가능한 최소 메커니즘

현재 [catalog entry](../../internal/catalog/catalog.go)는 PK, title, org, category, 수요·수정일, service type, description을 가진다. [ADR 0002](../adr/0002-model-planned-hybrid-catalog-search.md)가 측정했듯 11,902 × 768 float32는 약 35 MB이고 flat scan이 충분히 빠르다. 따라서 후보 생성에는 외부 vector DB가 필요 없다.

v1은 다음 범위가 현실적이다.

1. 호스트 모델이 역할이 다른 검색축을 만들고 현재 검색기가 최대 수십 개 Data Node를 회수한다.
2. 제목·기관·설명 중복을 제거하고 Anchor→Bridge 후보만 작은 beam으로 구성한다.
3. 선택한 node만 공식 명세로 확대한다. 현재 [Operation 구조](../../internal/apicall/describe.go)는 endpoint와 **요청 parameter**를 표면화하지만 출력 column profile은 보존하지 않는다.
4. 승인·접근 가능한 소수 API만 [실제 응답](../../internal/apicall/call.go)을 공통 지역·기간으로 호출해 host 쪽에서 bounded sample join을 검증한다.
5. 증거에 따라 edge를 `candidate`, `structurally_plausible`, `structurally_verified`, `sample_verified`, `blocked`, `rejected`로 구분한다. 공식 명세가 exact key namespace와 호환 grain을 확인한 edge만 `structurally_verified`, 실제 값 교집합까지 통과한 edge만 `sample_verified`다. 필수 edge가 하나라도 미검증이면 Verified Connection이라 부르지 않는다.

이 접근은 영구 graph나 vector DB 없이도 작동하며, 질의 때 실제로 필요한 node만 읽는다. 검색 recall은 현재 lexical/planned/optional-vector 층이 맡고, connection precision은 명세와 표본이 맡는다.

### 현재 데이터로 불가능한 것

- 카탈로그 전체의 자동 schema/column semantics 비교
- 전역 inclusion-dependency 또는 value-overlap index
- 모든 API pair의 공간·시간 grain 호환성 판정
- 미승인·심의승인·LINK API의 실제 join 검증
- 표본이 아닌 전체 모집단의 IND, match rate, cardinality와 결측률 보장
- task 없는 조합의 사업 성과·인과관계 자동 판정

SANTOS의 약 1.1만 **실제 표** 실험은 규모 자체는 다룰 수 있음을 보여주지만, synthesized KB를 만드는 데 cell values와 FD mining이 필요했고 큰 benchmark의 index는 수 시간과 수 GB를 사용했다. OpenDataCTL의 1.2만 항목은 동일한 1.2만 표가 아니라 metadata/API entry이며, 상당수는 승인 전 값을 읽을 수 없다. 따라서 SANTOS식 전역 relationship graph를 지금 흉내 내면 근거 없는 graph가 된다.

### 나중에 추가할 수 있는 로컬 profile

반복 호출로 증거가 축적되면 외부 vector DB가 아니라 작은 versioned local index로 충분하다. 공식 출력 schema와 검증된 sample에서 column별 type/pattern, distinct·null·uniqueness, value-hash/MinHash, 공간·시간 extent와 grain, code-system version을 저장하고, field token·semantic type·희귀 value hash의 inverted posting을 둔다. 후보 생성은 posting intersection, 정밀 검증은 원 sample 재실행으로 나눈다.

단, sample profile은 모집단 IND가 아니고 API 수정 시 stale해진다. source PK·operation·request slice·수집 시각·normalization version을 함께 보존하고, Verified Connection은 재검증 가능한 fixture에만 붙여야 한다.

## 결정

OpenDataCTL은 **semantic-neighbor recommender나 all-pairs join graph를 만들지 않는다.** 현재 구조 위에 역할 기반 후보 생성 → edge별 schema/grain/cardinality gate → 공통 slice의 실제 sample join → usefulness 평가 순서의 bounded cascade를 명세한다.

가장 중요한 제품 규칙은 다음 세 가지다.

1. Semantic similarity는 Data Node 후보를 찾을 뿐 Connection Edge를 증명하지 않는다.
2. 보완성은 `공통 key + 다른 payload/역할 + 명시된 Incremental Value`로 정의하고, novelty는 모든 hard gate 뒤에서만 쓴다.
3. 현재 카탈로그에 없는 column/value/grain 증거는 추측하지 않는다. 표본 검증할 수 없으면 `blocked` 또는 Abstention이며 Verified Connection이 아니다.
