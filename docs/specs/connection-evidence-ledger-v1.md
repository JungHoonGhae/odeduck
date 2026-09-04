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

- **Source**: 해당 PK의 data.go.kr 공식 상세페이지. provider 계약과 FILE 자산 관찰은 Record에 남긴다.
- **Record**: 특정 API operation의 bounded profile 또는 SHA-256이 있는 파일 관찰.
- **Entity**: v1은 만들지 않는다. 같은 코드처럼 보여도 namespace/version을 확인하기 전에는 합치지 않는다.
- **Claim**: 두 Data Node 사이의 구체적인 relation과 판정 상태. 일반 `RELATED_TO`/`HAS`는 금지한다.

검색의 `candidate`와 명세상 가능성뿐인 `structurally_plausible`은 장부에 넣지 않는다. 허용 상태는
`structurally_verified`, `sample_verified`, `blocked`, `rejected`다. 앞의 두 상태에는 exact 또는
deterministic match만 허용한다. probabilistic/unresolved 결과는 차단 또는 기각 근거로만 남긴다.

## 저장 계약

장부는 사용자 설정 디렉터리의 `connection-evidence.jsonl`에 append-only로 저장하며 프로세스 간 잠금을
사용한다. 같은 assessment는 content hash ID로 멱등 기록된다. 쓰기가 중간에 끊기거나 마지막 JSONL
frame이 손상되면 원인을 확정할 수 없으므로 자동 삭제하지 않고 오류를 반환한다. newline만 빠진 완전한
frame은 보존한 채 구분자를 복구한다. 빈 장부를 조회하는 동작은 설정
디렉터리나 잠금 파일을 만들지 않는다. 판정 수정은 삭제나 덮어쓰기가 아니라 같은 pair의 이전 ID를
`supersedes`하는 새 record다.

저장하는 것은 PK, delivery, operation/asset, non-secret 요청 파라미터의 SHA-256 request hash,
공식 source URL, expected key에 대응하는 field
selector·namespace·type·grain,
표본의 distinct/overlap/join expansion 집계, SHA-256, observed/valid time이다. API 응답의 raw 값과
인증정보는 저장하지 않는다. source URL은 해당 PK의 data.go.kr 공식 상세페이지로 제한하며 query와
fragment를 허용하지 않는다. 설명·변환·필드 문자열에서도 credential 값 형태를 거부한다.

## 판정 gate

- `structurally_verified`: 양쪽 field selector, identifier namespace/version, data type, record grain과
  proxy 변환이 공식 계약으로 확인돼야 한다. MCP는 같은 세션에서 양쪽 `inspect_dataset`이 먼저 성공한
  경우에만 이 상태를 받으며, field의 의미 해석 자체는 검사 결과를 읽은 에이전트의 판정이다.
- `sample_verified`: 구조 검증에 더해 양쪽 evidence hash, 양수 distinct overlap과 joined rows,
  좌우 최대 rows-per-key가 있어야 한다. 각 expected key는 양쪽 field evidence에 명시적으로 대응해야
  하며 count·distinct·duplicate와 sample 합계가 서로 모순되지 않아야 한다. overlap은 어느 한쪽
  distinct 수보다 클 수 없고 joined rows는 overlap과 좌우 최대 expansion이 만드는 범위 안이어야 한다.
  MCP에서는 같은 서버 세션의 `call_api`가 최근 15분 안에 실제로 만든 단일-key API profile 두 개와
  delivery·operation·request hash·전체 value frequency·모든 집계가 정확히 일치할 때만 기록한다.
  영수증은 최대 64개, 각 1 MiB의 value material만 메모리에 유지한다. 복합 key와 FILE 표본은 서버가 tuple/value frequency를 직접 증명할 수 있을
  때까지 `structurally_verified`까지만 기록한다.
- `blocked`/`rejected`: 재시도를 막을 구체적인 reason이 필요하다.
- 모든 기록은 RFC3339 `observedAt`을 갖고, 현실 유효기간이 알려졌다면 `validFrom`/`validTo`를 별도로 둔다.

MCP의 `record_connection_assessment`가 hard gate와 저장을 맡고,
`list_connection_assessments`가 PK/status별 최신 이력을 제한된 개수로 반환한다.

## 다음 전환 조건

장부가 실제 사용자 흐름에서 쌓인 뒤 고정 gold query로 flat scan 비용과 multi-hop 가치를 측정한다.
검증 edge 10만 건, 반복되는 3-hop 유료 workflow, 안정된 identity/time 규칙, 서버 의존성 없는 배포,
현 구조보다 측정 가능한 이점이 모두 확인되기 전에는 graph DB로 옮기지 않는다.
