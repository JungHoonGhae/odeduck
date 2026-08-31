# 교차 공공데이터 연결 발견 v1

상태: implemented for v0.10.0

결정일: 2026-08-31

## 1. 목적과 사용자 작업

OpenDataCTL은 경매나 수익 기회 전용 서비스가 아니다. 사용자가 정부의 데이터 명칭이나 기관 분류를
몰라도 원하는 API를 찾고, 서로 무관해 보이는 공공데이터 중 실제로 함께 검토할 가치가 있는 소수의
연결 후보를 발견하는 범용 계층이다.

v1이 지원하는 작업은 다음과 같다.

- 자연어 목표를 서로 다른 데이터 역할로 해석하고 전체 카탈로그에서 실제 항목을 찾는다.
- 직접 대상인 Anchor와 다른 판단 재료를 제공하는 Bridge를 최대 3개 제안한다.
- 왜 후보인지, 어떤 key로 연결할 것으로 예상하는지, 둘을 함께 볼 때 무엇이 새로 생기는지 설명한다.
- 공식 명세와 작은 응답 표본으로 더 확인해야 할 증거를 명시한다.
- 근거가 부족하면 연결을 만들지 않고 Abstention한다.

v1은 데이터만으로 사업 성공, 매출, 지불의사, 인과관계, 법적 판단을 확정하지 않는다. 그런 문장은
`commercially plausible` 또는 `research useful`한 가설일 수 있지만 외부 검증이 필요하다.

## 2. 용어와 상태

- **Data Node**: 카탈로그의 실제 데이터셋/API 항목과 PK.
- **Anchor Node**: 사용자의 직접 목표를 가장 잘 나타내는 시작 항목.
- **Bridge Node**: Anchor와 다른 payload나 역할을 제공하는 실제 검색 결과.
- **Connection Edge**: 두 노드 사이의 예상 연결. kind는 `entity`, `spatial`, `temporal`, `proxy`다.
- **Incremental Value**: 두 데이터를 함께 봐야만 가능한 새 판단이나 측정.
- **Connection Candidate**: 검색 metadata와 명시적 선택 gate만 통과한 pair.
- **Abstention**: 유효한 후보가 없어 아무 카드도 만들지 않은 정상 결과.

검색 계층이 반환하는 상태는 오직 `candidate`다. 이후 상태를 쓰는 주체는 증거를 직접 본 MCP
호스트이며 의미는 다음과 같다.

| 상태 | 필요한 증거 |
| --- | --- |
| `candidate` | 현재 hits의 실제 PK, 다른 역할, edge 예상 key, Incremental Value, metadata 근거 |
| `structurally_plausible` | 양쪽 공식 명세에 호환 가능성이 있는 field와 grain이 보임 |
| `structurally_verified` | key namespace·type·공간/시간 grain·변환이 공식 명세로 확인됨 |
| `sample_verified` | 공통 slice의 실제 값 overlap·null·distinct·duplicate·join expansion이 gate를 통과 |
| `blocked` | 승인·접근·coverage 부족 때문에 아직 판정할 수 없음 |
| `rejected` | key 의미, extent, grain, cardinality 또는 Incremental Value가 불일치 |

카드 전체 상태는 필수 edge 중 가장 낮은 증거 상태를 넘을 수 없다.

## 3. 공개 인터페이스

도구 유형은 기존 세 진입점을 유지한다.

1. `catalog_search`: 검색과 연결 후보 생성
2. `describe_api`: 선택한 각 PK의 operation, endpoint, 요청변수, 승인 조건 확인
3. `call_api`: 명세 검증, 인증키 주입, bounded 응답 호출과 선택적 field profile

활용승인이 없을 때만 기존 `apply`가 2.5단계로 들어간다. 수천 개 endpoint를 별도 MCP tool로
노출하지 않는다.

`catalog_search`의 기존 `query`, `concepts`, `limit`, `ranking`, `semantic`, `restOnly`는 유지한다.
v1은 다음 additive field를 사용한다.

```json
{
  "query": "사용자의 원래 목표",
  "axes": [{
    "role": "상권 수요",
    "query": "상권 점포 개폐업",
    "contribution": "가격 기회와 쇠퇴 상권을 구분",
    "edge": {
      "kinds": ["spatial", "temporal"],
      "expectedKeys": ["법정동코드", "기준연월"]
    }
  }],
  "anchorPks": ["15157207"],
  "bridgeSelections": [{
    "pk": "15095253",
    "whyCandidate": "제목과 preview의 개업일·폐업일, 부산 한정"
  }],
  "maxConnections": 3
}
```

새 field를 보내지 않는 호출은 기존 lexical/planned search와 같은 결과를 낸다. 반환값에는 additive
`anchors`, `connections`, `warnings`, `abstention`이 들어간다.

`call_api`와 CLI `call`은 `profileFields`/`--profile-field`를 최대 8개 받는다. profile은 응답 안의
leaf field 또는 dotted suffix를 재귀적으로 찾아 raw string 값을 보존하고, count, null, distinct,
duplicate, 실제 path를 반환한다. 한 응답에서 최대 50개 distinct 값을 보여주되 전체 bounded 응답의
count는 유지한다. 같은 leaf가 여러 실제 path에 있으면 `ambiguous=true`로 표시하고 합산 수치를
제공하지 않으므로, 호스트가 surfaced dotted path 중 하나를 다시 지정해야 한다. scalar 배열은 소유
field 아래 값으로 펼친다. OpenDataCTL은 이 public sample을 영구 저장하거나 두 API의 join 결과를 대신
만들지 않는다.

## 4. 검색과 연결 순서

### 4.1 MCP 호스트

호스트는 같은 `catalog_search`를 세 번 점진적으로 사용한다.

1. **Anchor 회수**: 원문을 보존하고 `role=anchor`를 포함한 3~8개 고유 역할 axis를 보낸다.
2. **Bridge 회수**: 첫 hits에서 Anchor PK를 고른 뒤, 실제 결과를 보고 비어 있는 보완 역할을
   `axes`로 만든다. 이때 첫 호출의 원래 anchor axis도 유지해 `anchorPks`와 함께 보낸다. 서버는
   선택 PK가 현재 호출에서도 그 anchor axis로 회수될 때만 Anchor로 인정한다.
3. **정밀도 gate**: 두 번째 hits에서 metadata가 역할을 뒷받침하는 실제 PK만 최대 3개 골라
   `bridgeSelections`로 실제 PK와 후보 근거를 다시 보낸다. 역할·Incremental Value·edge는 두 번째
   hit의 검색 계약에서 서버가 가져온다.

서버는 Anchor와 Bridge 선택 PK가 세 번째 호출의 현재 역할별 hits에 없으면 거부한다. 역할·PK 중복,
빈 후보 근거, 빈 Incremental Value, edge kind/key 누락, 변환 없는 proxy도 거부한다. 검색 점수 1위는
자동으로 카드가 되지 않는다.

그 다음 호스트는 카드의 모든 노드에 `describe_api`를 반복하고, 공통 지역·기간이 있는 소수 pair만
`call_api`로 표본을 가져온다. field profile을 비교해도 자동 join 증명은 아니며, namespace와 grain을
공식 설명으로 먼저 확인해야 한다.

### 4.2 독립 CLI

`catalog discover`는 설치되고 로그인된 Codex, Claude Code, Gemini CLI, Cursor Agent 중 하나로 같은
세 단계를 조정한다. 목표는 stdin으로 전달하고 읽기 전용/질문 모드와 임시 작업 디렉터리를 쓴다.
OpenDataCTL은 호스트의 token을 읽거나 저장하지 않는다. `--agent auto`는 첫 검색 계획 생성에서 실패한
provider 다음의 설치된 provider로 폴백한다. 이후 단계는 일관된 맥락을 위해 처음 성공한 provider를
계속 사용하며, 실패하면 1차 역할축으로 축소하거나 후보 생성을 중단한다. 특정 provider를 지정하면
첫 단계 실패를 그대로 반환한다.

post-retrieval prompt의 title·org·preview는 외부 제공기관이 작성한 untrusted data로 구분한다. Codex,
Claude Code, Gemini CLI는 shell/file/web/MCP 도구를 비활성화한다. Cursor Agent의 ask-mode sandbox는
workspace 밖 읽기를 막지 않으므로 독립 CLI에서 초기 검색 계획까지만 사용하고, 외부 metadata가 들어가는
Expand/Compose는 실행하지 않아 연결 카드를 Abstention한다. subprocess에는 실행·인증에 필요한 최소
환경만 전달하며 Cursor API key 환경변수는 전달하지 않는다.

MCP에서는 대화 중인 모델이 계획기이므로 하위 CLI를 실행하지 않는다. Ollama는 언어 추론기가 아니라
검색 recall을 보완하는 선택적 embedder다. 없거나 index가 stale/unavailable이면 결정적 lexical 검색으로
폴백하고 이 상태를 반환한다.

## 5. 책임 경계

| 책임 | OpenDataCTL 서버/모듈 | MCP 호스트 또는 독립 CLI 계획기 |
| --- | --- | --- |
| 카탈로그 전체 검색·중복 제거·순위·context bound | 소유 | 검색 목표와 역할 제공 |
| 의미 해석과 교차 분야 Bridge 역할 발상 | 소유하지 않음 | 소유 |
| 실제 hits에 selection PK가 있는지 확인 | hard gate 소유 | 후보 선택 |
| candidate 계약과 최대 3개 제한 | 소유 | 근거·Incremental Value 작성 |
| 명세 파싱·필수 파라미터·승인·인증키 주입 | 소유 | 어떤 node/operation을 검토할지 결정 |
| 응답 field의 bounded profile | 소유 | 양쪽 profile·namespace·grain 비교 |
| 사업·리서치 타당성, 인과, 최종 판단 | 주장하지 않음 | 증거와 외부 검증을 분리해 설명 |
| 영구 관계 그래프·사용자 질문 저장 | 소유하지 않음 | 필요 시 별도 제품 범위 |

이 혼합 경계는 provider별 prompt 차이를 작은 JSON 계약 안에 가두고, Windows와 Ollama 없는 환경에서도
같은 검색 결과 타입을 유지한다. 전역 schema graph가 없어도 질의 시 필요한 node만 확대할 수 있다.

## 6. 실행 예산과 안전 조건

- 역할: 첫 계획 3~8개, role 이름은 고유해야 한다.
- 검색: 기본 top 20, MCP/CLI hard max 100. 연결 검토에는 top 12~20을 권장한다.
- Anchor: 최대 3개를 받을 수 있으나 CLI v1은 가장 적합한 1개를 사용한다.
- Bridge selection과 최종 카드: 최대 3개, 역할별 최대 1개.
- CLI 모델 호출: 최초 계획, post-retrieval 확장, PK 선택의 최대 3회. 각 호출 timeout 3분.
- 명세 확대: 최종 카드의 고유 node만 호출하며 시작 예산은 총 12회 이하.
- 표본 호출: 공통 지역·기간을 정한 카드 최대 3개. 최소 row만 요청하고 계정별 rate limit을 따른다.
- profile field: 호출당 최대 8개, distinct raw value 출력 최대 50개.
- 신청: 명세와 목적을 확인한 선택 API만 수행한다. LINK는 제공기관 별도 경로이므로 자동 호출 대상으로
  간주하지 않는다.
- 인증정보: service key는 MCP 출력에 노출하지 않고 공식 HTTPS gateway 요청에만 주입한다.
- 개인정보: 카탈로그 metadata만 agent CLI에 전달한다. public API 응답은 사용자가 요청한 호출 범위에서만
  호스트로 반환하며 서버가 관계 index로 축적하지 않는다.

## 7. 검증 gate

후보가 강한 상태로 승격하려면 다음을 순서대로 확인한다.

1. 공식 field 이름과 의미, type, operation, identifier namespace/version.
2. 공간 coverage·단위·좌표계/행정코드와 시간 coverage·timestamp 의미·resolution.
3. exact/normalization/crosswalk/aggregation 중 연결 방식과 proxy의 정보 손실.
4. 같은 slice에서 양쪽 row count, distinct, null, uniqueness, distinct overlap과 방향별 containment.
5. joined rows와 좌우 expansion, 관측 cardinality. 의도하지 않은 N:M 폭증은 reject한다.
6. Bridge payload가 Anchor에 없는지, 그리고 Incremental Value가 사용자 판단을 실제로 바꾸는지.

접근 실패와 승인 대기는 `blocked`, 명세·값 불일치는 `rejected`다. field가 없거나 확인하지 못했다는
이유로 추측해 승격하지 않는다.

## 8. 출시 기준과 관찰

v0.10.0 출시 기준은 다음이다.

- 기존 `concepts` 호출과 세 MCP 도구 유형이 깨지지 않는다.
- explicit selection이 없으면 connection card가 0개다.
- selection은 현재 hits, 후보 근거, 역할, Incremental Value, edge/key gate를 모두 통과해야 한다.
- proxy transform 누락과 중복 역할은 거부한다.
- 검색 계층은 언제나 `candidate`만 반환하고 근거가 없으면 Abstention한다.
- Ollama 없음·stale·오류가 lexical 폴백을 깨지 않는다.
- 네 agent CLI용 read-only command contract와 provider-neutral response decoder에 회귀 테스트가 있다.
- sample profile은 선행 0, null·distinct·duplicate·truncation, 동일 leaf의 경로 모호성, scalar 배열을
  테스트한다.
- post-retrieval subprocess는 외부 metadata를 untrusted data로 취급하고 도구·불필요한 환경변수 접근을
  막는다.
- 전체 Go test, vet, tidy-diff, build와 release snapshot이 통과한다.

[출시 전 실제 평가](../research/connection-discovery-evaluation.md)는 세 질의에서 자동 역할별 1위보다
명시적 selection의 metadata precision이 좋아졌음을 관찰했다. 작은 비동일비용 표본이므로 수치 자체를
마케팅 주장을 위해 사용하지 않는다.

## 9. v1 밖의 일

- 모든 API pair의 all-pairs graph, 전역 inclusion-dependency mining, 영구 vector DB.
- 자동 schema/column index와 모집단 수준 join 보장.
- 사용자의 연결 카드 저장·알림·협업 UI.
- data.go.kr 밖의 포털 통합.
- 사업 성과·지불의사·인과관계를 데이터 조합만으로 자동 판정.

카탈로그가 수십만 건이 되거나 검증된 sample profile이 반복 축적되어 flat scan과 질의 시 확대가 병목이
될 때만 versioned local relation index/ANN을 별도 ADR로 검토한다.
