# LINK 검색·상세·호출 전후 품질 평가

평가일: 2026-09-01
카탈로그: 2026-08-31 동기화, API 11,902건
비교 대상: 기존 기본값 `REST only` 대 변경 기본값 `REST + LINK`
검색기: 동일한 로컬 카탈로그, 동일한 `embeddinggemma:300m-qat-q4_0` 의미 인덱스

## 결론

이번 변경은 **LINK가 검색에서 사라지는 문제는 해소**했고, 상위 수요 LINK의 상당 부분을
상세 계약과 자동 호출 경로까지 연결했다. 그러나 4,766개 LINK 전체가 호출 가능해졌다는
뜻은 아니다.

- 검색 대상은 REST 7,130건(전체의 59.9%)에서 전체 11,902건으로 늘었다. 기존에 제외되던
  LINK 4,766건이 기본 자연어 검색에 들어온다. 이는 검색 후보가 66.8% 증가한 것이다.
- 고정한 대표 질의 8개에서 “사용자 의도를 제목이 직접 표현하는 데이터가 top 3에 있는가”를
  수동 판정했다. 기존 REST-only는 2/8, LINK 포함 검색은 8/8이었다.
- 활용신청 수 상위 LINK 50건을 다시 상세 조회한 결과, 28건(56%)은 provider 계약을
  식별했다. 이 중 21건(42%)은 typed 자동 호출, 6건(12%)은 서울의 HTTPS 부재로 안전 차단,
  1건(2%)은 VWorld catalog family 미구현이었다. 나머지 22건(44%)은 정직하게
  `inspection_required`로 남았다.
- 따라서 “절반 가까운 데이터가 보이지 않던 문제”와 “지원 provider인데도 호출할 수 없던
  문제”는 크게 줄었지만, LINK 롱테일 자동 호출은 후속 adapter가 계속 필요한 상태다.

## 자연어 검색 고정 시나리오

평가 전에 공공데이터의 서로 다른 용도를 포함하도록 아래 여덟 질의를 고정했다. 두 조건에서
limit 10과 의미 검색을 동일하게 사용했다. 판정 기준은 느슨한 관련성이 아니라 **제목만 읽어도
질의의 핵심 대상과 작업을 직접 수행한다고 볼 수 있는 결과가 top 3에 존재하는가**다.

| 자연어 질의 | 기존 REST-only top 3 | LINK 포함 top 3 | 상세 단계 결과 |
|---|---|---|---|
| 어린이 제품 리콜 안전인증 | 직접 대상 없음 | `제품 안전인증 및 리콜 정보` 1위 | SafetyKorea, 5개 typed operation |
| 연속지적도 필지 경계 | 직접 대상 없음 | `연속지적도` 1위 | VWorld data, typed 호출 |
| 주소 지오코딩 좌표 | 직접 대상 없음 | `지오코더 API` 1위 | VWorld address, typed 호출 |
| 건강기능식품 원료 인정 | 직접 대상 없음 | `건강기능식품 기능성 원료인정 현황` 2위 | FoodSafetyKorea, 공식 표 기반 동적 schema |
| 실시간 지하철 도착 | 직접 대상 없음 | 서울 실시간 도착 1·2위 | 계약 식별, HTTP credential 전송은 차단 |
| 식품 바코드 제품 정보 | 직접 대상 없음 | 유통바코드·바코드연계제품 1·2위 | FoodSafetyKorea, typed 호출 |
| 농산물 경매 가격 | 직접 대상 1위 | 직접 대상 1위 + LINK 대안 | 기존 REST 호출 유지; LINK 롱테일은 검사 필요 |
| 상권 인구 매출 | 직접 관련 결과 존재 | 골목상권 매출 1위 + 지역 유동인구 | REST 호출 가능 후보와 미지원 LINK를 구분 |

REST-only 검색은 정확히 일치하는 LINK가 제거된 뒤 일부 단어만 맞는 REST를 완화 검색해
보호구 인증, 일반 주소 보유 시설, 버스 도착처럼 문장 일부만 겹치는 결과를 상위에 올리는
경우가 있었다. LINK 포함 검색은 여섯 시나리오에서 사용자가 실제로 말한 대상을 1~2위로
복원했다. 농산물 경매와 상권처럼 REST가 이미 강한 영역에서는 기존 후보를 없애지 않고
추가 provider 선택지를 보여줬다.

재현 예시:

```bash
# 변경된 기본 경로
odeduck catalog search "주소 지오코딩 좌표" --limit 10 -f json

# 이전 기본 경로 재현
odeduck catalog search "주소 지오코딩 좌표" --limit 10 --rest-only -f json
```

## 상위 수요 LINK 50건의 실제 진행 가능 상태

2026-08-31 카탈로그에서 `svcType=LINK`를 활용신청 수로 내림차순 정렬한 동일한 상위 50개
PK를 `describe`로 다시 조회했다.

| 상태 | 건수 | 비율 | 의미 |
|---|---:|---:|---|
| `implemented` | 21 | 42% | provider key 설정 후 기존 `call_api(pk, op, params)`로 typed 호출 |
| `blocked_insecure_transport` | 6 | 12% | 서울 일반·지하철 계약은 알지만 HTTPS가 없어 credential 호출 금지 |
| `not_implemented` | 1 | 2% | VWorld 검색목록 page로 개별 service 호출 계약은 아직 없음 |
| `inspection_required` | 22 | 44% | 외부 문서부터 조사할 롱테일; 임의 URL 호출 금지 |

`implemented` 21건은 VWorld data/address/search/OGC와 FoodSafetyKorea dataset family다.
SafetyKorea는 상위 50 표본 밖이지만 별도로 5개 operation이 구현됐다. 기존에는 이 50건이
모두 자동 호출 경로 밖이었다.

## 호출 품질과 안전 경계

실제 provider key는 이 평가 환경에 저장되어 있지 않아서 운영 credential을 이용한 성공 응답은
주장하지 않는다. 대신 공개 seam에서 다음을 확인했다.

- SafetyKorea·FoodSafetyKorea·VWorld는 operation, 필수값, enum, page 범위와 exact HTTPS
  endpoint를 contract test로 고정했다.
- VWorld 2D는 상위 표본의 9개 `svcIde`를 공식 guide의 고정 `data` 코드에 묶어 자동 주입한다.
  미검증 service ID는 호출하지 않고, 공식 문서상 서비스 예정인 `GetFeatureType`도 노출하지 않는다.
- FoodSafetyKorea는 호출 전에 공식 서비스 상세의 `요청인자` 표와 service ID를 검사한다.
  이 평가 과정에서 공식 filter의 underscore(`PRMS_DT`)를 거부하던 parser 결함도 fixture로
  재현해 수정했다.
- header/query/path key는 provider·path scope별로 저장하고 redirect를 따르지 않는다. 오류와
  응답에서는 raw·query escaped·path escaped key를 redaction한다.
- key가 없으면 신청 URL과 `provider-key set` 명령을 안내하고, 서울은 key를 읽기 전에
  `blocked_insecure_transport`로 중단했다.
- HTTP 200도 provider 성공 envelope가 없거나 XML/OGC exception이면 실패한다. 검색 후보가
  늘어난 대가로 호출 실패를 성공처럼 보이게 하지 않는다.

```text
SafetyKorea credential을 얻지 못했습니다: ...
`odeduck provider-key set safetykorea` ...

Seoul Open Data Plaza 호출은 credential을 보호할 HTTPS endpoint가 없어 차단되었습니다
```

## 남은 한계와 다음 판단 기준

이 결과를 “LINK 40%가 모두 활용 가능”으로 해석하면 안 된다.

1. 전체 LINK의 **기본 검색 후보군 포함률은 100%**지만, 이것이 임의의 질의에서
   모든 LINK가 상위에 노출된다는 검색 발견률을 의미하지는 않는다. 상위 수요 표본의 typed
   자동 호출률은 현재 42%다.
2. 상위 50은 수요 우선순위 표본이며 전체 4,766건의 provider 분포를 대표하지 않는다.
3. 기본 검색에 미지원 LINK도 들어오므로 `svcType`, `handoff.state`, `invocationState`를 다음
   단계에서 반드시 확인해야 한다. 임의 endpoint 추측은 허용하지 않는다.
4. adapter 우선순위는 `inspection_required` 결과의 host별 빈도, 검색 노출 빈도, 공식 HTTPS와
   명세 안정성으로 정한다. 단순 endpoint 수를 늘리는 것은 품질 지표가 아니다.
5. 각 신규 adapter는 이 문서의 동일 질의 세트 또는 해당 도메인 고정 질의를 추가하고,
   direct-intent top-3와 상세/호출 상태가 실제로 개선될 때만 coverage 개선으로 기록한다.
