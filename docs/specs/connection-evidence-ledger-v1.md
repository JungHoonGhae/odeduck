# 연결 근거 장부 v1

상태: implemented

결정일: 2026-09-04

## 먼저 답해야 하는 질의

1. 이 두 data.go.kr 데이터셋은 공식 구조까지만 확인됐는가, 실제 표본까지 연결됐는가?
2. 어떤 operation 또는 FILE 자산과 field namespace·grain이 판정을 뒷받침하는가?
3. 이 pair가 차단되거나 기각된 이유는 무엇인가?
4. 새 관측이 과거 판정을 대체했는가?
5. 특정 시점에 어떤 근거가 관측됐고, 그 사실의 현실 유효기간은 언제인가?

이 질의는 flat JSONL로 충분하다. 여러 hop traversal이나 중심성 계산이 필요하지 않으므로 graph DB를
도입하지 않는다.

## 경계

- **Source**: data.go.kr 상세페이지, 공식 provider 계약, 관찰한 FILE 자산.
- **Record**: 특정 API operation의 bounded profile 또는 SHA-256이 있는 파일 관찰.
- **Entity**: v1은 만들지 않는다. 같은 코드처럼 보여도 namespace/version을 확인하기 전에는 합치지 않는다.
- **Claim**: 두 Data Node 사이의 구체적인 relation과 판정 상태. 일반 `RELATED_TO`/`HAS`는 금지한다.

검색의 `candidate`와 명세상 가능성뿐인 `structurally_plausible`은 장부에 넣지 않는다. 허용 상태는
`structurally_verified`, `sample_verified`, `blocked`, `rejected`다. 앞의 두 상태에는 exact 또는
deterministic match만 허용한다. probabilistic/unresolved 결과는 차단 또는 기각 근거로만 남긴다.

## 저장 계약

장부는 사용자 설정 디렉터리의 `connection-evidence.jsonl`에 append-only로 저장하며 프로세스 간 잠금을
사용한다. 같은 assessment는 content hash ID로 멱등 기록된다. 판정 수정은 삭제나 덮어쓰기가 아니라
같은 pair의 이전 ID를 `supersedes`하는 새 record다.

저장하는 것은 PK, delivery, operation/asset, 공식 source URL, field selector·namespace·type·grain,
표본의 distinct/overlap/join expansion 집계, SHA-256, observed/valid time이다. API 응답의 raw 값과
인증정보는 저장하지 않는다.

## 판정 gate

- `structurally_verified`: 양쪽 field selector, identifier namespace/version, data type, record grain과
  proxy 변환이 공식 계약으로 확인돼야 한다.
- `sample_verified`: 구조 검증에 더해 양쪽 evidence hash, 양수 distinct overlap과 joined rows,
  좌우 최대 rows-per-key가 있어야 한다. overlap은 어느 한쪽 distinct 수보다 클 수 없다.
- `blocked`/`rejected`: 재시도를 막을 구체적인 reason이 필요하다.
- 모든 기록은 RFC3339 `observedAt`을 갖고, 현실 유효기간이 알려졌다면 `validFrom`/`validTo`를 별도로 둔다.

MCP의 `record_connection_assessment`가 hard gate와 저장을 맡고,
`list_connection_assessments`가 PK/status별 최신 이력을 제한된 개수로 반환한다.

## 다음 전환 조건

장부가 실제 사용자 흐름에서 쌓인 뒤 고정 gold query로 flat scan 비용과 multi-hop 가치를 측정한다.
검증 edge 10만 건, 반복되는 3-hop 유료 workflow, 안정된 identity/time 규칙, 서버 의존성 없는 배포,
현 구조보다 측정 가능한 이점이 모두 확인되기 전에는 graph DB로 옮기지 않는다.
