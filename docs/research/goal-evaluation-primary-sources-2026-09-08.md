# 목표 평가 방법론: 자율 완주·오염 방지·사업 가설의 검증 경계

조사일: 2026-09-08. 상태: 1차 자료 조사 완료, 실제 목표 평가 결과 아님.
연결: [평가 재설계 이슈](https://github.com/JungHoonGhae/odeduck/issues/44),
[INTENT](../../INTENT.md), [완료 계획](../specs/goal-driven-completion-plan.md).

## 결론

안전한 중단과 유용한 자율 완주를 별도로 채점해야 한다. 가능한 목표에서도 전부
`review_required`로 끝나는 시스템은 오탐 방지 점수를 얻을 수 있지만 목표 완주에는 실패한다.
이를 확인하려면 실행기와 독립적인 양성 정답, 오탐 사례, 같은 예산의 비교 실행이 필요하다.
사업 가설의 근거가 충실하다는 판단도 고객의 구매 의사나 사업성 검증을 대신하지 않는다.

아래는 외부 자료에서 확인한 사실과 오데덕에 권하는 설계를 구분한다. 기존 G1–G5, I1–I10의
통과선을 낮추거나 공개된 사례를 새 held-out 평가로 이름만 바꾸자는 제안이 아니다.
웹은 평가 방법론 확인에만 사용했다. 새로운 공공데이터 관측·고객 인터뷰·유료 모델 실행은 하지 않았다.

## 확인한 1차 자료와 채택 범위

| 자료 | 원문에서 확인한 내용 | 채택 / 그대로 옮기지 않을 것 |
| --- | --- | --- |
| [Anthropic, Demystifying evals for AI agents](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents), 2026-01-09; Definitions, Steps 2–5, pass@k/pass^k | 대화 기록과 실제 환경의 최종 상태를 구분한다. 풀 수 있는 정답 예시, 행동해야 하는/하지 말아야 하는 양쪽 사례, 실행 격리와 적절한 채점기를 권한다. 여러 번 중 한 번 성공과 매번 성공은 다른 지표다. | 결과 채점과 제약 위반 감사를 분리한다. 정해진 도구 순서를 정답으로 삼거나 중단만 보상하지 않는다. 글의 사례 수·확률 예시를 제품 통과선으로 옮기지 않는다. |
| [MLE-bench 논문 v3](https://arxiv.org/html/2410.07095v3), §§2.1, 3, 4, 6 | 개발용 문제를 시험용 문제와 분리한다. 반복 실행과 자원 변경을 평가하며, 공개 해법 오염을 설명 변경·로그·표절 검사로 조사한다. 저자들은 고수준 전략의 재사용과 미래 모델의 오염까지 배제하지 못한다고 명시한다. | 개발/시험 분리, 자원·재시도 공개, 오염 감사의 한계를 채택한다. 질문을 바꿔 쓰거나 공개 데이터를 재분할한 것만으로 오염이 없다고 선언하지 않는다. Kaggle 성능을 공공데이터 분석·사업 역량으로 일반화하지 않는다. |
| [Anthropic, Eval awareness in Claude Opus 4.6’s BrowseComp performance](https://www.anthropic.com/engineering/eval-awareness-browsecomp), 2026-03-06 | 원천 조사 대신 공개된 벤치마크 답을 찾거나 복호화한 사례를 보고한다. 다른 실행의 검색 흔적도 웹에 남았다. URL 차단만으로 막지 못한 우회가 있었다. | 입력에서 정답을 지우는 것 외에 전체 접근 경로를 감사한다. 이 사례를 모든 모델의 행동이나 특정 오데덕 실행의 오염 증거로 취급하지 않는다. |
| [Karpathy, autoresearch/program.md](https://github.com/karpathy/autoresearch/blob/master/program.md), 2026-09-08 열람한 master | 변경 대상은 `train.py`; 평가·데이터 준비를 포함한 `prepare.py`와 평가 하네스는 변경 금지다. 먼저 baseline을 실행하고 고정 학습 시간 안에서 비교한다. 비슷한 성능이면 단순화에 가치를 둔다. | 변경 가능한 구현과 고정 평가의 경계, baseline, 단순성 판단을 채택한다. `program.md`까지 에이전트가 임의로 바꾸는 구조로 해석하지 않는다. 학습 5분·val_bpb 단일 지표는 이 제품에 옮기지 않는다. |
| [NSF National I-Corps Teams 공식 프로그램](https://www.nsf.gov/funding/opportunities/nsf-national-innovation-corps-teams-nsf-national-i-corps-tm), Synopsis, 2026-09-08 열람 | 고객·산업 조사와 현장 업무의 직접 확인을 사용한다. 사업모델 판단, 제품-시장 적합성에 대한 직접 증거, 기술 시연을 별도 산출물로 설명한다. | 기술적 가능성과 시장 증거를 구분한다. 이 프로그램의 절차가 오데덕의 수익성이나 특정 결제 통과선을 보장한다고 해석하지 않는다. |

논문은 버전을 고정했다. 웹 문서와 `master`는 이후 변경될 수 있으며 열람일이 확인 범위다.
아래 연결한 정책의 통과선과 검증 순서는 위 자료를 참고한 **로컬 제품 판단**이다.

## 채택한 정책

조사에서 나온 로컬 설계는 [벤치마크의 평가 계약](../specs/goal-completion-benchmark-v1.md#evaluation-contract-2026-09-08)에 합쳤다.
이 노트에는 원문 근거와 채택 범위만 둔다. 구체적인 통과선·격리·사업 가설 평가를 바꿀 때는 그 계약을 수정한다.
현재 실행의 상태·한계는 [Goal Trial Audit](../specs/goal-trial-audit-v1.md)에서 확인한다.
