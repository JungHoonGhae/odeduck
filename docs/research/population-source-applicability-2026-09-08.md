# July 인구 CSV의 정의·등록구분·행 범위 적용 조사 — 2026-09-08

July CSV의 **전체 3,619행은 행안부 월간 연령별 통계의 같은 2026년 7월·등록구분 `전체` 관측과
코드별 수치가 일치했다.** 인천 162행의 합계와 11개 구·군별 합계도 해당 공식 화면과 일치했다.
따라서 이번 revision의 등록구분과 상위 집계행 중복 여부는 일반 설명을 빌려 가정하는 단계에서
벗어나, 별도 공식 배포물과의 재현 가능한 대조로 뒷받침된다.

만 나이는 주민등록인구현황 작성기관에 확인한 KOSIS 공식 답변이 근거다. 아래의 전체 행 대조는
그 통계계열의 실제 관측과 July 파일을 연결하는 추가 근거다. **July 파일의 ID·해시를 직접 지정해
만 나이라고 선언한 원천 문구는 발견하지 못했다.** 계열 방법론의 적용 판단과 파일 자체의 명시적
선언을 구분한다.

이 문서는 [기존 full-scope 진단](goal-result-execution-validation-2026-09-08.md)의 후속 원천 조사다.
원래 G4 질문·oracle·benchmark·제품 코드·검토 archive와 완료 판정을 변경하지 않았다.
이 조사 자체는 새 모델 호출이나 목표 완료로 집계하지 않는다. 모델 진단의 최신 분모는 위 실행
검증 문서에서 관리한다.

## 대상 revision과 조사 경로

| 항목 | 고정값 |
| --- | --- |
| 제공기관 / 관리부서 | 행정안전부 / 주민과 |
| 데이터셋 | PK `15097972`, `행정안전부_지역별(행정동) 성별 연령별 주민등록 인구수_20260731` |
| 과거 버전 ID | `uddi:95f114ef-87c9-4669-971d-aab9aff9d7d4/2` |
| 파일 | `지역별(행정동) 성별 연령별 주민등록 인구수_20260731.csv` |
| 실제 bytes / SHA256 | `2568212` / `6920fdafd269554d259e8498301f004c9799f499c38832de9352a0e7def916c5` |
| 레코드 기준일 | 모든 3,619행의 `기준연월 = 2026-07-31` |
| 포털 과거 metadata 날짜 | 등록일 `2026-08-04`, 수정일 `2026-08-18` — 레코드 기준일과 구분 |
| 관측일 | 2026-09-08. 공식 화면 정상 응답 11:49:11–11:49:13 UTC, 공식 CSV 요청 11:49:57 UTC, 독립 대조 마지막 확인 11:54:59 UTC |

포털 검사는 아래의 오데덕 CLI로 수행했다. 과거 popup이 준 metadata와 자산 계약만 사용했고,
현재 August 설명을 July metadata로 상속하지 않았다. `inspect-july.json`의 `observation`에서
원본 해시와 bytes를 재확인했다. 전체 행 독립 계산은 보존된 동일 해시의 공개 원본 bytes를 사용했다.

```sh
go run ./cmd/odeduck inspect 15097972 --delivery file \
  --file-version 'uddi:95f114ef-87c9-4669-971d-aab9aff9d7d4/2' --observe --format json
go run ./cmd/odeduck inspect 3033304 --delivery file --format json
```

정확한 포털 원천 위치:

- [데이터셋 상세](https://www.data.go.kr/data/15097972/fileData.do)의 과거 버전 목록.
- 과거 계약 `POST https://www.data.go.kr/tcs/dss/selectDpkDetailInfo.do`,
  `publicDataDetailPk=uddi:95f114ef-87c9-4669-971d-aab9aff9d7d4`, `publicDataHistSn=2`.
- [CLI가 광고한 July CSV 자산](https://www.data.go.kr/cmm/cmm/fileDownload.do?atchFileId=FILE_000000003807403&dataNm=%ED%96%89%EC%A0%95%EC%95%88%EC%A0%84%EB%B6%80_%EC%A7%80%EC%97%AD%EB%B3%84%28%ED%96%89%EC%A0%95%EB%8F%99%29+%EC%84%B1%EB%B3%84+%EC%97%B0%EB%A0%B9%EB%B3%84+%EC%A3%BC%EB%AF%BC%EB%93%B1%EB%A1%9D+%EC%9D%B8%EA%B5%AC%EC%88%98_20260731&fileDetailSn=1).

## 등록구분: 같은 월의 네 설정을 독립 대조

[PK 3033304의 CLI 검사](https://www.data.go.kr/data/3033304/fileData.do)는 외부 제공 URL로
`https://jumin.mois.go.kr/ageStatMonth.do`를 실제 광고한다. 이 참조는 공식 원천의 진입 근거이며,
그 자체가 PK 15097972 July 파일에 모든 정의를 적용한다는 선언은 아니다.

[행안부 연령별 화면](https://jumin.mois.go.kr/ageStatMonth.do)의 `select#register[name=sltUndefType]`는
빈 값=전체, `Y`=거주자, `N`=거주불명자, `O`=재외국민이다. 같은 화면의 등록구분 도움말은 전체에
세 집단을 포함하고 외국인을 제외한다고 설명한다. `거주자`는 거주지가 분명한 사람이며 재외국민을
제외한다. 원문은 보존 HTML `v2-age-july-incheon-all.html` 1136–1202행에 있다.

원문을 선택할 수 있는 경계는 `form[name=search]` 안에서 `select#register`와 같은 `dl`이다.
PC 설명은 그 안의 `.popoverBox.pop2 .pContent table tbody tr`의 `th`/`td` 쌍이고,
모바일은 `label[for=register]`와 같은 `dt`의 `.popover-layer` 내부 표다. 제목은 모두 `등록구분`이다.
월말 작성기준은 `조회기간` 제목의 `.popover-layer .pContent` 및 `p.searchMsg`에 있다.
**이 ageStatMonth HTML에는 만 나이 선언이 없다.** 그 정의의 원천은 뒤 절의 KOSIS 답변이며,
주민등록 화면의 등록구분 도움말과 혼합해 하나의 원문으로 취급하지 않는다.

이 네 설정 각각에 2026년 7월·인천광역시·1세 단위·0세부터 100세 이상·계/남녀 구분을 지정했다.
응답의 `table#contextTable > tbody`에서 인천 및 11개 구·군 행을 읽었다. 아래 인구는 원문 총인구수,
6–17세는 해당 원문의 1세별 열을 합산한 값이다.

| 등록구분 | 전체 연령 인천 인구 | 6–17세 인천 인구 | July 원본 합계와 관계 |
| --- | ---: | ---: | --- |
| 전체 (`""`) | 3,063,730 | 304,280 | 일치 |
| 거주자 (`Y`) | 3,049,203 | 303,916 | 전체보다 각각 14,527명 / 364명 작음 |
| 거주불명자 (`N`) | 9,305 | 233 | 전체에 포함되는 별도 부분집단 |
| 재외국민 (`O`) | 5,222 | 131 | 전체에 포함되는 별도 부분집단 |

인천 및 11개 구·군 모두에서 July 행을 합산한 309개 표시수치가 `전체` 화면과 일치했다.
거주자·거주불명자·재외국민의 총계/남/여/6–17세 합은 각 지역의 전체와 일치했다.
즉, 이름에 `주민등록`이 들어간다는 이유로 설정을 고른 것이 아니라 같은 월의 구분별 실제 수치를
비교했다. 재외국민이나 거주불명자를 제외한 거주자 통계로 304,280명을 설명하면 이 대조와 어긋난다.

## 행 단위와 coverage: 공식 전체읍면동 내려받기와 대조

화면의 `전체읍면동현황` 선택은 `state=3`이고 CSV 버튼은
[공식 CSV 다운로드](https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3)에
선택 조건을 POST한다. 이 연결은 기본 HTML의 48–60행과 1456–1483행에서 확인했다.
2026년 7월·전체·1세 조건의 내려받기는 6,537,761 bytes, 3,919개 데이터 행이다.
SHA256은 `3f1c7c0000104dc1feb3cf643187c1427d87736c67a05df0cb0b7a2e9e78315c`다.

HTML에서 export까지의 구체적인 연결은 다음과 같다. `#csvDown`의 click handler가
`input[name=state]:checked`를 읽고, 값이 3이면 `form#formXlsDown`의 action을
`downloadCsvAge.do?searchYearMonth=<input[name=category]>&xlsStats=3`으로 설정해 submit한다.
폼의 method는 POST이며, hidden 필드가 응답에 반영된 월·지역·등록구분·연령 범위를 보존한다.
따라서 이 조사에서 내려받기 URL은 같은 공식 HTML이 제공한 동작에서 확인했다. 제품에 임의 외부
URL 호출 권한이나 일반적인 URL 수집 기능이 생겼다는 뜻은 아니다.

두 CSV를 EUC-KR로 해독하고 따옴표/쉼표를 처리했다. 연결 키는 같은 발급기관의 10자리
행정기관코드와 같은 월이다. 이름 유사도나 기존 oracle을 매칭 기준으로 쓰지 않았다.
원본 230열과 공식 export 310열의 헤더 순서를 별도 확인했으며, 변환은 다음으로 제한했다.

- 원본의 남녀 각 0–99세는 같은 성별·나이 열과 직접 대조했다.
- 원본의 100–109세와 `110세이상`은 합쳐 공식 `100세 이상`과 대조했다. 이 이상 연령의 세부
  분포 자체는 공식 화면에서 독립 검증하지 못한다.
- 공식 계의 나이 열은 원본 남녀의 같은 연령 합으로 계산했다. 총계/남/여와 연령구간 합도 대조했다.

원본 **3,619개 코드 모두 존재했고 표시수치 309개 × 3,619행 = 1,118,271개 위치의 불일치는 0개**다.
이는 계와 구간합 등 중복 정보를 포함한 대조 위치 수이며, 독립적인 111만 관측이나 모델 정확도로
세지 않는다. 원본 자체의 행별 남녀 연령 합과 총인구수도 일치했다. 전국 원본 합 51,088,284명은
공식 export의 16개 시·도 상위 행 합과 일치했다.

원본은 3,619개의 고유 코드를 갖고 읍면동명이 없는 행이나 코드 끝 5자리가 `00000`인 상위 행은
없었다. 공식 export에는 원본에 없는 300행이 있는데, 296행은 위 상위 코드 패턴이고 나머지
4행은 인구 0명의 비상위 출장소다. 이 네 행의 코드·원문 이름·위치는 `comparison-summary.json`
의 `official.unmatchedNonAggregateCodes`에 남겼다. 따라서 두 배포물의 전체 행 목록은 같지 않다.

특히 G4 대상인 인천은 공식 export의 **177행 = 원본과 같은 162행 + 인천/11개 구·군 12행 +
인구 0명의 기존 구 출장소 3행**이다. 추가 3행은 `2811400000 중구영종출장소`,
`2811800000 중구용유출장소`, `2826500000 서구검단출장소`다. 이 행들이 현재 행정기관으로
유효하다고 해석하지 않으며, 공식 export에서 0으로 관찰되었다는 사실만 기록한다.

다음 행 번호는 헤더를 1행으로 센 **July 원본 CSV 물리 행**이다. 각 구·군의 309개 표시수치 합이
공식 화면의 해당 구·군 행과 일치한다. 마지막 열은 원본 남녀 6–17세의 독립 합이다.

| 구·군 | July 원본 행 범위 | 원본 행 수 | 6–17세 합 |
| --- | --- | ---: | ---: |
| 제물포구 | 1206–1223 | 18 | 6,558 |
| 영종구 | 1224–1229 | 6 | 16,418 |
| 미추홀구 | 1230–1250 | 21 | 34,921 |
| 연수구 | 1251–1265 | 15 | 53,949 |
| 남동구 | 1266–1285 | 20 | 46,079 |
| 부평구 | 1286–1307 | 22 | 40,747 |
| 계양구 | 1308–1319 | 12 | 21,203 |
| 서해구 | 1320–1335 | 16 | 43,598 |
| 검단구 | 1336–1343 | 8 | 35,599 |
| 강화군 | 1344–1357 | 14 | 4,365 |
| 옹진군 | 1358–1367 | 10 | 843 |
| 합 | 1206–1367 | 162 | 304,280 |

인천의 원본 162행에는 158개 나머지 행과 **읍면 출장소 4행**이 있다. 각 출장소도 공식 export의
같은 코드·같은 수치로 관찰된다. `1357행 서도면볼음출장소` 5명, `1359행 북도면장봉출장소` 9명,
`1362행 대청면소청출장소` 2명, `1366행 자월면이작출장소` 23명이다(모두 6–17세).
이 네 행까지 합쳐야 구·군 및 인천의 공식 합과 맞으며, 출장소 문자열을 이유로 제외하면
39명을 누락한다. 상위 지역 집계행을 같은 합에 추가하면 중복 집계가 된다.

이름의 bytes가 완전히 같다는 주장도 하지 않는다. 원본 시도/시군구/읍면동명을 공백으로 연결한
표기와 공식 이름은 33행에서 달랐다. 인천은 `2812565000`의 원본 `송림3.5동`과 공식
`송림3·5동` 한 건이다. 나머지는 세종의 공백 및 청주 일부 동의 표기 차이다.
원문 차이는 `verification-summary.json.nameDifferences`에 보존했고, 보정 사전이나 전역 alias를
만들지 않았다. 이번 연결 강도는 이름 동일성이 아니라 코드·월·수치의 결정론적 대조다.

## 만 나이와 적용 범위

[KOSIS 2024-06-28 답변](https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=22124)은
주민등록인구현황 작성기관에 확인했다고 명시하며, 주민등록상의 출생월일과 통계 기준 월말로
연령을 산출하고 만 나이를 사용한다고 설명한다. 원문 위치는 보존 HTML 1571–1587행,
핵심 표현은 1582행의 `나이는 ‘만 나이’를 의미`다. 이 답변은 2024년 질의에 대한 계열 방법론이다.

[KOSIS 2019-07-18 답변](https://kosis.kr/civilComplaint/qnaDetail.do?boardIdx=13637)도 작성기관에
확인한 만 나이 정의와, 2019년 6월말 0세의 출생일 구간을 설명한다. 원문 위치는 보존 HTML
1560–1572행이며 행안부 주민과를 작성기관으로 명시한다. 두 답변의 연락처는
[행안부 원천 홈페이지](https://jumin.mois.go.kr/) 담당부서 주민과의 연락처와 같다.
홈페이지 HTML 296행에는 국가통계 승인번호 제110026호가 표시된다.

확인된 연결은 **작성기관의 계열 방법론 → 같은 기관의 월간 연령별 원문 → 같은 월·코드·수치가
전부 일치하는 July CSV**다. 이 경로는 July `6세남자`…`17세여자`에 만 나이 방법론을 적용하는
근거를 강화한다. 다만 파일 ID를 특정한 방법론 선언이나 개인별 생년월일 원장을 확보한 것은
아니므로, 파일 자체에 명시문이 있다고 보고하거나 개별 나이 계산까지 재검증했다고 하지 않는다.
관측상 100세 이상으로 묶인 구간에도 이 구분을 유지한다.

[행안부 화면](https://jumin.mois.go.kr/ageStatMonth.do)은 월별 작성기준을 매월 말일로 안내하며
해당 월을 선택한 응답 헤더는 `2026년 07월`이다. 이는 원본 전 행의 `2026-07-31`과 일치한다.
이 모집단은 주민등록 집계이므로 학교 소재지별 재적 학생수와 동일 집단이라는 근거가 되지 않는다.
학교 기준일·등록된 주소와 학교 소재지·집계 대상 차이는 기존 G4 비교 한계로 남는다.

## 재현 조건과 보존물

원본·공식 응답·독립 스크립트는 다음 `mktemp` 디렉터리에 보존했다. 원자료를 수정하거나 삭제하지
않았으며, 이 임시 경로는 Git에 들어간 장기 archive가 아니다.

`/tmp/odeduck-population-applicability.KmYDur/`

정상 화면의 정확한 요청 URL은 `POST https://jumin.mois.go.kr/ageStatMonth.do`다.
HTML의 선택값을 읽어 보낸 폼은 다음과 같다. 등록구분만 `""`, `Y`, `N`, `O`로 바꾼 네 응답을
각각 보존했다. 쿠키·로그인·SSO·인증키가 필요하지 않았다.

```json
{
  "tableChart": "T", "sltOrgType": "2", "sltOrgLvl1": "2800000000", "sltOrgLvl2": "A",
  "sltUndefType": "", "searchYearMonth": "month",
  "searchYearStart": "2026", "searchMonthStart": "07",
  "searchYearEnd": "2026", "searchMonthEnd": "07",
  "sum": "sum", "gender": "gender", "sltOrderType": "1", "sltOrderValue": "ASC",
  "sltArgTypes": "1", "sltArgTypeA": "0", "sltArgTypeB": "100"
}
```

다운로드는 `POST https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3`다.
위 폼에서 `tableChart`, `searchYearMonth`를 제거하고 `category=month`, `state=3`을 추가했다.
`Content-Type: application/x-www-form-urlencoded`와 화면 URL의 Referer를 보냈다.
조건·관측시간·응답 크기·해시는 `acquisition-v2-log.json`, `download-log.json`에 있다.

처음에는 July 파일의 최대 표기 110을 화면에도 전달했으나 화면의 선택지는 100이 최대였다.
`sltArgTypeB=110`인 네 요청은 HTTP 200이어도 표가 없는 불완전 HTML을 반환했다.
실패 응답 `age-july-incheon-*.html`과 `acquisition-log.json`을 보존했다.
다음 정상 요청은 실제 선택지 100을 사용했다. 빈 표를 인구 0이나 해당 월 자료 부재로 세지 않았다.
웹 검색 도구의 복잡한 query URL 읽기 실패도 공식 POST 응답의 부재와 혼동하지 않았다.

| 파일 — 위 디렉터리 기준 | SHA256 |
| --- | --- |
| `population-july-original.csv` | `6920fdafd269554d259e8498301f004c9799f499c38832de9352a0e7def916c5` |
| `mois-july-all-eupmyeondong.csv` | `3f1c7c0000104dc1feb3cf643187c1427d87736c67a05df0cb0b7a2e9e78315c` |
| `v2-age-july-incheon-all.html` | `95401a2ab193bdba898ac814c461b44eabe4eed440c4d6ac9ba69f5d9451851c` |
| `v2-age-july-incheon-resident.html` | `93b7768452f3a9a026fd05fb7b4d1c858964fc08505626928db2e8903edb90cb` |
| `v2-age-july-incheon-unknown.html` | `d622b189358f0145d80ad77f30fec17b15f18b8b3eae6f1faac41fc14be34b78` |
| `v2-age-july-incheon-overseas.html` | `e2debb4bc1f4e34d6b9857a9e4820e7ede769eabed19d5b976b481a461be0ac7` |
| `inspect-july.json` | `9bf5ad3b4c2b8246898dc160ab226e644036c1702bd93a12e622167622f8a9eb` |
| `inspect-series.json` | `360ddeeb2fb5987892d9b7853fbd5992288af099f449829fbb0d795776bdbe1d` |
| `kosis-age-qna-22124.html` | `3b1578c4c5820291b0930f9ec7d1f1a7e636d5232619d129fe6acc0a5cdb49b0` |
| `kosis-age-qna-13637.html` | `425f49afdf4d8c450f6300b4da524968370f22bdd46db7b57e4a57d22c1d9d32` |
| `mois-home.html` | `14a6dd571e6881a96d711c908308b73747cefb99ddb9f0db9908c2d9da3328c6` |
| `acquire.mjs` — 공식 화면 네 조건 취득 | `17171258700ad81ccca7a8ede7ffc61f46ecb075261fd5c4fe283a1dd9eb4c55` |
| `download.mjs` — 공식 전체읍면동 CSV 취득 | `b462c11ddd53d88acb6620b672a853310a732cf5ab26ff425f167c03d9960163` |
| `compare.mjs` — 코드·연령·총계·구군별 대조 | `5dfd1f1c98172f36b4cd28f54d42587efab6748284f5902be11d5d41ea568134` |
| `verify.mjs` — 독립 헤더·이름 차이·구분 합 확인 | `190053ce303c292f932816fdfa8dcd1ebdaf5e372a1e8664da581732021d7dee` |
| `comparison-summary.json` | `49b3e1b081390fcbd3aa3471a8ffb51e321b892a84f7651772ea49964c788f90` |
| `row-comparison.json` — 양쪽 CSV의 행 위치 포함 | `6bb3d94a7eafdb40d291a9714703edbd3f81d77d0e8f8742bdff762638233f02` |
| `verification-summary.json` | `2fa2a5076d90f2e863c12db1620ac0ee6d5c6c4e7c343c94163c68d63dfcc246` |
| `acquisition-v2-log.json` | `28d265f582965d3fa1cbb65f5e43fa06742cc78c4f0c2d40659265fcf65fc1c4` |
| `download-log.json` | `9b1e6d30665da30e7abc3443889f331c5686a8c34dfb847839002c0d1cc7316d` |

Node `v22.23.1`에서 `compare.mjs`, `verify.mjs`를 실행했다. 원본은 EUC-KR, 공식 HTML은 UTF-8이다.
스크립트의 `dir`은 위 경로로 고정되어 있다. 새 취득은 새 `mktemp -d` 경로를 만들고 스크립트의
`dir`만 그 경로로 바꿔 실행한다. 취득 스크립트는 기존 출력 파일을 덮어쓰지 않는다. 오프라인
대조도 보존물을 새 디렉터리에 복사한 뒤 같은 방식으로 실행하면 이전 요약을 보존할 수 있다.
요약의 실행시각 및 동적인 HTML 세션 문자열 때문에 재실행 산출물의 해시는 달라질 수 있다.

## 확인된 범위와 남은 구현 격차

이번 조사로 **이 July revision의 등록구분 전체, 11개 인천 구·군의 원문 행 범위, 읍면 출장소 포함,
상위 집계행 제외, 같은 월 공식 합과의 일치**를 source hash·행 위치·요청 조건에 묶었다.
만 나이 적용은 공식 계열 설명과 이 revision의 전체 관측 대조가 함께 지지하며, 파일별 직접
선언은 미확인으로 유지한다. 행정기관코드의 영구 재사용 여부나 다른 월의 관할영역 동일성은
이번 대조로 승인하지 않는다.

현재 제품 `inspect 3033304`는 `capability=inspectable`과 공식 외부 URL만 반환하며, 해당 외부 파일의
typed adapter가 없다는 경고를 낸다. 이번 HTML/CSV 취득과 대조는 별도 공식 원천 조사용 Node
스크립트로 수행했다. **제품의 자동 취득·검토 입력 연결·자율 발견 완료 증거가 아니다.**
원문 정의와 선택 조건, 파일별 대조 및 제외 행 근거가 실제 검토 입력으로 전달되는 경로는 남아 있다.
이 문서의 분석 요약을 제공기관 원문인 것처럼 주입하지 않는다.

연구 스킬의 1차 원천 추적과 그래프 엔지니어링 스킬의 원천/record/claim·기준일 구분을 적용했다.
법령이나 일반 정의만으로 파일별 적용을 승인하지 않고, 실제 기록의 결정론적 대조와 남은 미확인을
따로 남겼다. 로그인·신청·계정 변경·유료 모델 호출·추가 에이전트·외부 메시지 전송은 수행하지 않았다.

## 추가 조사: 공식 6–17세 범위 CSV 계약

2026-09-08 13:34–13:39 UTC에 [같은 행안부 월간 화면](https://jumin.mois.go.kr/ageStatMonth.do)의
기본 연령 범위 선택과 CSV 동작을 확인했다. **공식 폼의 1세 단위·6세부터 17세 조건은 모든 요청
연령을 보존한 43열 CSV를 반환했다.** 실제 내려받기는 1,050,109 bytes, 전국 3,919행으로,
기존 310열 내려받기와 코드 집합이 같았다. 인천을 선택한 폼이라는 이유로 응답을 인천 전용으로
취급할 수는 없다. `전체읍면동현황`의 실제 전달 범위는 전국이며, 인천 범위는 코드로 구분했다.

### 공식 폼·선택 반영·다운로드 연결

아래 위치는 새로 보존한 `age-july-incheon-all-6to17.html`의 물리 행 번호다. HTML과 그 안의
스크립트를 읽고 별도 POST로 응답을 확인했으며, 브라우저에서 JavaScript를 실행한 검증은 아니다.

| 확인 대상 | 공식 응답의 위치와 계약 |
| --- | --- |
| 검색 폼 | 1062행 `form[name=search][action="ageStatMonth.do"][method=post]` |
| 1세 단위 | 1419행 `select#sltArgTypes[name=sltArgTypes]`의 `option[value=1]`이 선택됨. 나머지 선택지는 10·5세 |
| 범위 선택 | `select#sltArgTypeA`, `select#sltArgTypeB`는 원시 HTML에서 비어 있고 `load2()`의 `ageGroup1[2]`, `ageGroup2[2]`가 각각 0–100 정수 옵션 101개를 생성함 |
| 요청 범위 반영 | 979–982행 `init2()`의 `selInitial` 값은 단위 1, 시작 6, 끝 17. 1444행 `#ageChange`는 `세` |
| 100세 경계 | 182–189행은 끝 값 100일 때 `#ageChange`를 `세(이상)`으로 표시. 앞서 보존한 전체 연령 CSV의 실제 마지막 연령 헤더도 `100세 이상` |
| 등록구분·월 | `select#register[name=sltUndefType]`의 빈 값=전체가 선택됨. `#searchYearStart/End`는 2026, `#searchMonthStart/End`는 07이 선택됨 |
| 다운로드 폼 | 1460행 `form#formXlsDown[name=formXlsDown][method=post]`. 1462–1477행 hidden 필드가 인천 코드·전체·2026-07·단위 1·범위 6–17을 반영함 |
| 출력 종류 | 1483행 `input#down3[name=state][value=3]`의 라벨이 전체읍면동현황. 기본 체크값 1에서 3을 선택하는 동작에 해당 |
| CSV 목적지 | 48–60행 `#csvDown` click handler가 선택된 `state=3`을 읽어 `#formXlsDown`의 action을 `downloadCsvAge.do?searchYearMonth=month&xlsStats=3`으로 바꿔 submit함 |
| 계·남녀 선택 | 541–548행 `chkExcel()`이 hidden `sum`, `gender`의 초기 빈 값을 검색 체크박스 상태에 따라 `sum`, `gender`로 채움 |
| 출력 표 | `table#contextTable`의 1785행 월 표시는 `2026년 07월`; 계·남·여 각각 총인구수·연령구간인구수·6–17세 12개 열을 갖고, 데이터 행은 코드·명칭을 포함해 44칸 |

연령 시작·끝의 101개 옵션을 각각 검사했다. 100은 100세 이상 구간이며 110은 화면에 없는 값이다.
이번에는 110 또는 65세 이상을 별도로 요청하지 않았고, 요청한 6–17세 범위를 다른 연령 집계로
대체하지 않았다. 한 달·1세·전체읍면동 조건에 대한 `chkExcel()` 검사도 같은 범위를 허용한다.

지역 선택은 주의가 필요하다. `#sltOrgLvl1`, `#sltOrgLvl2`는 원시 HTML에서 비어 있다.
198–262행의 함수는 공식 AJAX 응답으로 옵션을 채우며, `loadSidonm()`은 `levels=1` 중 요청 코드와
같은 `hangkikcd`를 선택한다. 이번 조사에서는 이 추가 AJAX를 호출하지 않았다. 따라서 hidden이나
JS에 요청 코드가 다시 나타난다는 사실만으로 그 코드의 유효성을 검증했다고 하지 않는다.
실제 표의 첫 행 `2800000000`·인천광역시와 CSV의 같은 코드 및 원본 수치가 일치하는 것이 이번
요청의 관측 근거다. 검색 폼의 hidden `sltOrgType`은 초기값 1이지만 `goSearch()`가 시도 선택에
따라 2로 바꾸며, 다운로드 폼에는 요청한 2가 반영되어 있다.

연도 선택지는 2008–2026이고 월 선택지는 01–12이므로, 옵션에 있다는 것만으로 해당 월의 공표를
보장하지도 않는다. 이 요청의 월은 출력 표와 **CSV의 모든 42개 수치 헤더**가 `2026년07월_`로
시작하는지 별도로 확인했다. 코드·월·연령의 응답 검증을 요청값 반영 검사와 함께 적용해야 한다.

### 실제 응답과 독립 수치 대조

새 요청은 [공식 CSV action](https://jumin.mois.go.kr/downloadCsvAge.do?searchYearMonth=month&xlsStats=3)에
다음 폼을 URL-encoded POST한 것이다. `Content-Type`은 `application/x-www-form-urlencoded`,
`Referer`는 `https://jumin.mois.go.kr/ageStatMonth.do`로 고정했다. 검색 HTML 요청에는 같은 조건에서
`category`, `state`를 빼고 `tableChart=T`, `searchYearMonth=month`를 넣었다.

```json
{
  "sltOrgType": "2", "sltOrgLvl1": "2800000000", "sltOrgLvl2": "A", "sltUndefType": "",
  "searchYearStart": "2026", "searchMonthStart": "07",
  "searchYearEnd": "2026", "searchMonthEnd": "07",
  "sum": "sum", "gender": "gender", "sltOrderType": "1", "sltOrderValue": "ASC",
  "sltArgTypes": "1", "sltArgTypeA": "6", "sltArgTypeB": "17", "category": "month", "state": "3"
}
```

검색 HTML은 13:34:55.093–13:34:55.706 UTC에 HTTP 200, `text/html;charset=UTF-8`,
104,915 bytes로 응답했다. CSV는 13:35:31.726–13:35:37.662 UTC에 HTTP 200으로 응답했다.
CSV의 실제 응답 헤더는 `Content-Type: application/octet-stream;charset=utf-8`이고
`Content-Disposition`은 다음과 같다.

```text
attachment; filename="202607_202607_%EC%97%B0%EB%A0%B9%EB%B3%84%EC%9D%B8%EA%B5%AC%ED%98%84%ED%99%A9_%EC%9B%94%EA%B0%84.csv";
```

**헤더의 charset 표시와 달리 CSV bytes는 strict UTF-8 해독이 실패하고 strict EUC-KR 해독이
성공했다.** 별도의 따옴표 상태 기반 CSV 파서로 escaped quote·필드 안의 쉼표·행 경계를 처리하고,
헤더 중복·행 너비·정수 표기·10자리 코드·코드 중복을 검사했다. 인코딩 오류를 치환 문자로 숨기지
않았다. 첫 필드는 `행정구역`이며 표시명의 마지막 괄호에 있는 10자리 코드를 매칭 키로 읽었다.
이름 유사도는 대조 기준이 아니다. 43열의 순서는 행정구역 1열 다음 계·남·여 각각
`총인구수`, `연령구간인구수`, `6세`부터 `17세`까지 14열이다.

| 대조 범위 | 대조 위치 수 | 결과 |
| --- | ---: | --- |
| 기존 310열 공식 CSV의 전국 3,919코드 → 새 CSV | 3,919 × 42 = 164,598 | 코드 집합 동일, 누락·수치 불일치 0 |
| July 원본 230열의 3,619코드 → 새 CSV | 3,619 × 42 = 151,998 | 누락·수치 불일치 0 |
| 선택 HTML의 인천·11개 구군 → 새 CSV | 12 × 42 = 504 | 누락·수치 불일치 0 |
| 원본 인천 162행의 11개 코드별 구군 합과 인천 합 → 새 CSV의 집계행 | 12 × 42 = 504 | 수치 불일치 0 |

각 행의 42개 수치 중 24개는 원본의 남·여별 6–17세를 직접 대조하고, 계의 12개 연령은 원본
남녀를 더했다. 3개 전체 연령 총인구수를 대조하고 나머지 3개 연령구간인구수는 해당 성별의
6–17세를 독립 합산했다. **기존 0–100세 이상 구간합을 새 6–17세 구간합과 직접 비교하지 않았다.**
인천의 전체 연령 총인구수는 3,063,730명으로 유지되지만 새 연령구간인구수는 304,280명이며,
남 155,855명·여 148,425명이다. 모든 새 CSV 행에서도 남녀 합과 선택 구간 합을 확인했다.
이 대조 수는 중복 합계를 포함한 검사 위치이며, 앞 절의 전체 연령 1,118,271개 대조나 모델 평가
분모에 더하지 않는다.

실제 응답의 전국 3,919행에는 `0000000000` 전국 총계 행이 없고, 16개 시도 집계행이 있다.
원본 일치 3,619행과 추가 300행(상위 코드 패턴 296행·비상위 4행)은 기존 전체 연령 export와
같다. 인천 코드는 177행으로, **원본과 일치하는 162행 안에 읍면 출장소 4행이 이미 포함**되어
있다. 추가 15행은 인천/11개 구군 집계 12행과 기존 구의 인구 0명 출장소 3행이다.

| 원본에 포함된 읍면 출장소 코드 | 원본 행 / 새 CSV 행 | 6–17세 |
| --- | --- | ---: |
| `2871042500` 서도면볼음출장소 | 1357 / 1453 | 5 |
| `2872031500` 북도면장봉출장소 | 1359 / 1456 | 9 |
| `2872034500` 대청면소청출장소 | 1362 / 1459 | 2 |
| `2872037500` 자월면이작출장소 | 1366 / 1463 | 23 |

행 번호는 각 CSV의 헤더를 1행으로 센 값이다. 이 네 행의 39명은 원본과 새 export 모두에서
같으며, 기존 **G4의 원본 162행 → 11개 구군 → 304,280명**을 바꾸지 않는다. 구군 그룹은 이번
snapshot에서 원본 코드 앞 5자리와 공식 집계코드를 대조해 확인했으며, 다른 시점까지 일반화한
행정구역 규칙이나 출장소 일괄 제외 규칙을 추가하지 않았다.

### 새 보존물·재현·구현상 의미

새 보존 경로는 `/tmp/odeduck-monthly-export.fqn2uP/`다. 이전 조사 디렉터리의 원본과 결과를
덮어쓰지 않았다. 두 POST는 각각 55초·16 MiB 스트림 제한을 적용했고 redirect는 따르지 않았다.
Cookie·Authorization을 보내지 않았으며 응답의 익명 세션 쿠키도 후속 요청에 재사용하지 않았다.
HTTP 상태·응답 종류·HTML 완결성 또는 CSV 시작 헤더·attachment를 검사하고, 정확한 요청 body,
시간, 전체 응답 헤더, raw bytes 및 해시를 아래 파일에 보존했다.

| 파일 — 새 디렉터리 기준 | SHA256 |
| --- | --- |
| `mois-july-all-eupmyeondong-6to17.csv` | `911940f3c38ffb7ed487a820607560616dedd5d226dcec1c0b9d7ffb65c4e47f` |
| `age-july-incheon-all-6to17.html` | `1a1423e466dbc572ebee91b7cfeabf5ee7ac39085dc44c2b91ce3b689bfef982` |
| `acquire-range.mjs` | `a584812f515de8a9524dbb699c432d0bf2e0117fd0085826369cd44f8778f713` |
| `verify-range.mjs` | `74472073f9c86ee3471574f738a55a324047914d311b2d77119d931d32591e22` |
| `page-request.json` | `ba93b366f2d9d95fa510b71dd47c7c4e50e46ec72418bb33a8fcc9bfda717296` |
| `page-response.json` | `441cdf92f70a2d600fd01b75617680e9921263ac2ca011f0ca5cbbd9f683d806` |
| `export-request.json` | `5c4b480a3e29052c3c4bf41c40d7c9bfbe7e453f12f0d88894072b976f96a135` |
| `export-response.json` | `6b28af1860cdeac0b099229856f37e3f8ecac4cb505d9fb97b6823693ee82cf8` |
| `comparison-range-summary.json` | `937fdf823a0a1e1c96bd3463f0f29c7d3828eff61c1010e34d3553eaeb14eb03` |
| `contract-range-summary.json` | `7b2f4e1b9f4bc3b7bd62e24432c46a6b64221ba6ba90ad44f866090ea80505fe` |
| `verification-attempts.json` | `c5583763cfad25917dc0737d3b3f6176b4c212c3b65dbca72981b21db685e0c6` |

`verify-range.mjs`는 이전 원본 SHA256 `6920fdafd269554d259e8498301f004c9799f499c38832de9352a0e7def916c5`,
이전 전체 연령 export SHA256 `3f1c7c0000104dc1feb3cf643187c1427d87736c67a05df0cb0b7a2e9e78315c`,
새 CSV·HTML의 해시를 먼저 고정 검사한다. Node `v22.23.1`에서 대조를 완료한 시각은
13:39:42.437 UTC다. 첫 오프라인 실행에서는 JS의 `new Option("9", "9")` 안 공백을 옵션 추출
정규식이 허용하지 않아 실패했다. 공백 처리를 고친 다음 전체 검사가 통과했으며, 이를 원천의
9세 누락으로 분류하지 않았다. 두 POST 요청의 실패나 추가 재요청은 없었다.

재현 명령은 `node acquire-range.mjs page`, `node acquire-range.mjs export`,
`node verify-range.mjs`다. 각 스크립트의 보존 경로는 고정되어 있고 산출물은 `wx`로 신규 생성하므로,
다시 실행할 때는 새 임시 디렉터리를 만들고 스크립트의 출력 경로를 그곳으로 바꾼다. 해시가 고정된
이전 원본은 그대로 읽는다. 이 임시 보존물이 영구 Git archive라는 주장은 하지 않는다.

구현에는 공식 연령 선택을 그대로 보존하는 고정 계약을 사용할 근거가 생겼다. 이 요청은 310열
전체 자료를 내려받아 임의로 자르는 단계 없이 43열의 공식 응답을 제공한다. 다만 응답의 전국 범위,
실제 EUC-KR bytes, 코드·월·요청 연령·합계의 검증과 집계행 구분을 함께 전달해야 한다.
**이번 절은 공식 계약과 수치 보존 조사이며, 오데덕의 자동 발견·제품 취득·검토 입력 전달·모델 호출
또는 G4 완료를 입증하는 실행 결과가 아니다.** 제품 코드·oracle·기존 전체 연령 대조 판정은 이
조사에서 수정하지 않았다.
