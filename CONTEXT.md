# OpenDataCTL Discovery

OpenDataCTL이 고립된 공공데이터를 발견하고, 서로 결합할 가치와 가능성을 검증할 때 사용하는 공통 언어다.

## Language

**Data Node**:
검색·상세 확인·호출의 대상이 되는 하나의 공공데이터 API 또는 데이터셋.
_Avoid_: 결과, 아이템

**Anchor Node**:
사용자의 질문과 직접 관련되어 연결 탐색의 출발점이 되는 Data Node.
_Avoid_: 정답 API, 메인 데이터

**Bridge Node**:
Anchor Node와 역할은 다르지만 함께 사용했을 때 새로운 판단이나 측정을 가능하게 할 수 있는 Data Node.
_Avoid_: 연관 데이터, 랜덤 추천

**Connection Hypothesis**:
둘 이상의 Data Node를 결합하면 각 노드만으로는 할 수 없던 사업적 판단이나 연구 측정이 가능해진다는 검증 전 주장.
_Avoid_: 인사이트, 사업 기회

**Connection Edge**:
두 Data Node가 공간·시간·엔티티 키 또는 명시된 proxy 변환으로 결합될 수 있다는 관계.
_Avoid_: 유사성, 관련성

**Incremental Value**:
Data Node를 결합했을 때만 새로 가능해지는 의사결정, 예측, 우선순위화 또는 연구 측정.
_Avoid_: 시너지, 새로움

**Verified Connection**:
모든 필수 Connection Edge가 공식 명세와 실제 표본의 교집합·매칭 품질로 검증된 Connection Hypothesis.
_Avoid_: 가능성 있는 연결, 흥미로운 조합

**Abstention**:
주어진 증거와 예산 안에서 유효한 Connection Hypothesis를 만들 수 없다고 명시적으로 반환하는 정상 결과.
_Avoid_: 검색 실패, 결과 없음 오류
