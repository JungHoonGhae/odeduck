# Project skill evaluation

평가일: 2026-09-04  
평가 기준: `skill-judge` 120점 체계

## odeduck-business-harness

- **점수:** 107/120, B
- **패턴:** Process
- **지식 비율:** Expert 65% / Activation 32% / Redundant 3%
- **판정:** 프로젝트의 데이터 계약과 결제 gate를 잘 결합하지만, 고객조사의 일부는 범용 원칙이라 실제
  인터뷰·결제 artifact가 쌓일수록 오대덕 고유 기준으로 교체해야 한다.

| 차원 | 점수 | 최대 | 근거 |
| --- | ---: | ---: | --- |
| Knowledge delta | 15 | 20 | 정부 무료 시스템, 식별 edge, 결제 등급을 한 gate로 묶은 부분은 고유하나 고객조사 일부는 범용 |
| Mindset + procedure | 13 | 15 | dataset이 아닌 바뀌는 결정에서 시작하고 inspect까지 연결 |
| Anti-patterns | 14 | 15 | 실제 조사에서 반복된 여섯 함정과 이유가 구체적 |
| Specification | 15 | 15 | WHAT/WHEN과 사업·GTM trigger가 frontmatter에 있음 |
| Progressive disclosure | 14 | 15 | 150줄 미만의 self-contained skill, 관련 프로젝트 문서만 조건부 선택 |
| Freedom calibration | 13 | 15 | 발산은 자유롭고 결제·식별 gate는 엄격함 |
| Pattern | 9 | 10 | 단계별 완료 조건이 있는 Process 패턴 |
| Practical usability | 14 | 15 | 결과 형식과 중단 조건이 명확하나 실제 artifact 예시는 아직 없음 |

### 남은 개선

1. 첫 고객 인터뷰와 발주서가 생기면 익명화된 성공·실패 artifact를 기준으로 추가한다.
2. 같은 후보를 두 번 평가한 뒤에도 결과가 흔들리면 evidence 등급별 최소 표본을 조정한다.
3. 고정된 14~30일 실험이 구매 주기가 긴 공공·대기업 고객에 맞지 않는 사례가 나오면 branch를 분리한다.

## odeduck-graph-engineering

- **점수:** 114/120, A
- **패턴:** Process
- **지식 비율:** Expert 84% / Activation 15% / Redundant 1%
- **판정:** source record·canonical entity·claim과 valid/observed time을 분리해 현재 오대덕의 가장 큰
  오연결 위험을 직접 제어한다. 특정 graph database를 미리 선택하지 않은 점도 현재 단계에 맞다.

| 차원 | 점수 | 최대 | 근거 |
| --- | ---: | ---: | --- |
| Knowledge delta | 18 | 20 | 공공데이터의 식별 범위, 정정, coverage, 행동별 오탐을 graph 계약으로 구체화 |
| Mindset + procedure | 15 | 15 | query-first에서 gold set과 저장 선택까지 순서가 비가역성에 맞음 |
| Anti-patterns | 15 | 15 | metadata edge 물질화, 이름 병합, provenance 삭제 등 실제 실패를 직접 차단 |
| Specification | 15 | 15 | entity resolution, provenance, temporal, graph model trigger가 명확함 |
| Progressive disclosure | 14 | 15 | 150줄 미만 self-contained skill이며 외부 runtime reference가 없음 |
| Freedom calibration | 14 | 15 | 모델링 판단은 열어두고 고비용 자동 행동의 식별 규칙은 엄격함 |
| Pattern | 9 | 10 | 완료 기준과 branch가 있는 Process 패턴 |
| Practical usability | 15 | 15 | 평가셋과 결과 계약에 실제 verified ledger schema와 exact/deterministic fixture가 반영됨 |

### 남은 개선

1. 실제 자동 행동별 허용 오탐이 정해지면 현재의 정성 gate를 수치 계약으로 바꾼다.
2. graph 저장 후보를 비교하는 날에는 동일 gold query를 실행하는 benchmark script를 함께 추가한다.

## 종합

두 스킬 모두 프로젝트 로컬에서 독립 실행 가능하다. 사업 하네스는 실제 고객 artifact가 들어오기 전까지
B로 유지하고, graph 하네스는 현재 설계 검토에 사용할 수 있다. 외부 스킬 업데이트를 자동 동기화하지
않으며, 오대덕에서 관찰한 실패가 있을 때만 이 평가와 하네스를 함께 갱신한다.
