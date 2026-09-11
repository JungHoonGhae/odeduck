# 검증 결과와 적용 범위

[README로 돌아가기](../README.md) · [요구사항별 진행 상태](specs/goal-driven-completion-plan.md)

| 확인한 것 | 근거와 적용 범위 |
| --- | --- |
| 통합 카탈로그 | 2026-09-02 릴리스 기준 96,683개 노드. REST·LINK·FILE과 복수 제공형 보존 |
| 실계정 신청·호출 | 온비드 공매·나라장터 입찰·공영도매시장 경매·중소기업 지원사업 데이터 4종의 활용신청→승인→호출 |
| 외부 제공기관·변경 감시 | SafetyKorea·FoodSafetyKorea·VWorld 호출. 서울 열린데이터광장은 HTTPS 부재로 호출 차단. adapter 4개·canary 11개를 주간 CI로 검사 |
| 원천 보고 검토 | 두 개발 사례의 마지막 12회에서 승인 기대 6회·보류 기대 6회가 일치. 사전 선택한 원천·절차의 검증 |
| 전체 목표 완주 | 미완료. 사전 선택한 원천의 검토 결과를 분야 간 자율 완주의 증거로 사용하지 않음 |

원천 보고 검토의 [전체 시도·실패 기록](research/source-report-review-validation-2026-09-08.md)를
함께 공개했다. 사전 선택한 자료로 얻은 검토 결과와 키워드·PK 없이 목표에 도달하는 능력은 구분한다.
분야 간 자율 완주와 공간·인과·사업 가설의 검증은 아직 끝나지 않았다.

<details>
<summary>기존 공개 도구와 비교한 범위</summary>

data.go.kr 관련 공개 도구 10개를 고정한 소스 revision의 도구 등록과 호출 코드로 비교했다.
검색→상세→호출을 지원하는 도구가 있었다. 조사한 구현에서는 활용신청 제출과 승인 확인을 사람이 맡았다.

<p align="center">
  <img src="assets/odeduck-before-after.svg" width="900" alt="조사한 공개 도구의 탐색·호출 범위와 오데덕의 목표 기반 검색·검사·활용신청·승인 확인·호출 흐름 비교">
</p>

오데덕은 목표 기반 카탈로그 탐색, API·FILE·LINK 검사, 활용신청·승인·키 재사용·실호출을 한 흐름에
묶는다. 이 비교는 범용 목표 완주나 분석 정확도의 우위를 입증하지 않는다.
비교 대상·시점·판정 근거는 [경쟁 워크플로 감사](research/competitive-workflow-audit.md)와
[경쟁·수요 교차 검증](research/competitor-and-demand-cross-validation-2026.md)에 있다.

</details>
