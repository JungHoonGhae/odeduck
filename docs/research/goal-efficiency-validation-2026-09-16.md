# 목표 실행의 반복 입력·추가 모델 호출 — 2026-09-16

## 변경 범위

사용자가 기본 검색 대비 목표 실행의 토큰 비용을 줄이고, 별도 Codex·Claude 실행이 필수인지
재검토하도록 요청했다. MCP는 원래 host가 계획을 맡는다. 추가 agent CLI는 별도 검토를 선택한
경우에만 필요하다는 경계를 기본 안내·CLI·사용자 Skill에 일치시켰다.

CLI `goal`의 필수 `--review-with`를 제거했다. 단독 CLI는 계획용 모델이 필요하지만 독립 검토는
선택 사항이다. 검토가 설정되지 않은 실행은 계산한 artifact와 review_required를 반환하고 추가
계획 호출을 멈춘다. 이 상태를 output_ready로 올리거나 exit 0인 완주로 바꾸지 않았다. MCP host는
실제 결과·출처·한계로 답할 수 있으나 독립 검토가 수행됐다고 주장할 수 없다.

## 입력을 줄인 방법

- 공통 지침은 기본 안내와 열 개 상세 topic으로 나눴다. read_guide는 필요한 topic만 읽는다.
  전체 상세 계약은 같은 원본에 보존하며 기본 정책·근거 공개·예산·검토 기준은 축소하지 않았다.
- MCP는 최초 snapshot 뒤 이전 revision에 대한 RFC 6902 변경분을 보낸다. 누적 후보·관측·결과를
  매번 복제하지 않는다. fullState:true는 맥락 복구·기존 클라이언트를 위한 전체 snapshot이다.
- modelUsage는 실제 자식 모델 호출의 prompt/response 바이트·시간과 provider가 보고한 토큰을
  기록한다. Codex·Claude의 보고 형식을 인식한다. 미보고 호출을 0토큰으로 추정하지 않는다.
  캐시 토큰은 전체 입력 토큰의 부분이며, cache-write/read 보고 여부도 별도로 표시한다.

짧은 진입 안내에서 필요한 상세만 읽게 하는 구조는
[OpenAI의 지침 재검토 가이드](https://developers.openai.com/blog/rethinking-skills-and-prompts-for-gpt-6-astra)의
progressive disclosure 원칙을 참고했다. 행동별 결정적 검증은 지침 길이와 독립적으로 유지한다.

## 재현 가능한 관측

| 측정 대상 | 변경 전/전체 전달 | 변경 후 | 의미 |
| --- | ---: | ---: | --- |
| 기본 계획 지침 | 53,731 bytes | 5,900 bytes | 89.0% 감소. 요청한 상세 topic은 추가되므로 모든 호출의 감소율은 아님 |
| 동일한 6행동 MCP fixture의 누적 응답 | snapshot 17,134 bytes | delta 9,384 bytes | 45.2% 감소. 같은 최신 구현의 응답 형식 비교이며 과금 토큰 비교는 아님 |
| 기본 MCP fixture의 추가 모델 호출 | — | 0 | host 추론·토큰은 제외. 전체 AI 비용 0이라는 의미가 아님 |

fixture는 실제 Engine의 검색·검사·취득 seam에서 두 원천 값을 받아 정확한 십진 합계를 계산한다.
`9007199254740993 + 1 = 9007199254740994`를 보존하며, 매 단계 변경분을 적용한 상태와 별도
전체 snapshot이 동일함을 대조한다. null/누락/false·배열 삭제·escaped JSON Pointer도 검사한다.
검토가 없으면 결과는 review_required이고 independent review는 비어 있다. 위 바이트 수는 이
fixture의 한 실행 값이며, 원천·결과 크기와 타임스탬프 길이에 따라 달라진다.

재현:

```sh
go test ./internal/mcpserver -run 'TestGoalPatch|TestDefaultMCPDelta' -v -count=1
go test ./internal/agentplan -run 'TestGoalInitialPrompt|TestProviderUsage' -v -count=1
go test ./cmd/odeduck -run TestGoalReturnsArtifactWithout -v -count=1
```

### 실제 모델 계측 확인

설치된 Codex로 `goal --agent codex --max-rounds 1`을 실행했다. 질문은
“공공데이터에서 일반인이 예상하기 어려운 사실을 찾아 출처와 한계를 보여주세요.”였다.
의미 검색 preflight 뒤 첫 계획 호출의 사용량이 다음과 같이 기록됐다.

- 계획 호출 1회, 검토 호출 0회, 보고 누락 0회
- prompt 7,987 bytes, provider response 1,773 bytes, 13,344 ms
- provider 보고 input 20,525 tokens, 그중 cached input 16,640 tokens, output 251 tokens

바이트와 토큰은 다르다. provider CLI 자체의 입력도 토큰 계측에 들어가므로 prompt 파일 크기를
토큰 수나 과금액으로 치환하지 않는다. 이 실행은 의도적으로 한 단계에서 budget_exhausted로
끝냈다. 실제 데이터 취득·자율 완주·전체 비용 절감률을 입증한 실험이 아니다. 이 계측 뒤 캐시 필드별
보고 횟수 표시를 보완했으며 추가 실모델 호출 없이 같은 parser fixture로 검증했다.

## 보존하는 한계

MCP host의 모델 토큰·비용, 로컬 임베딩은 오데덕이 관측할 수 없어 이 집계에 포함하지 않는다.
미지원 provider 형식은 unreportedCalls로 남긴다. CLI의 행동별 새 계획 호출과 누적 상태 전달은
여전히 남아 있다. 이번에는 공통 지침과 MCP 응답의 반복을 줄였으며, 모든 모델 호출을 묶거나
전체 목표의 비용 상한을 새로 도입한 것은 아니다.

같은 목표·원천·모델·권한에서 기본 검색과 목표 실행의 완주율·총 토큰을 비교하는 후속 평가가
필요하다. 기존 [무힌트 실행 실패와 제한된 후속 완료](goal-entrypoint-validation-2026-09-16.md),
INTENT.md의 I1–I10 상태와 독립 검토 기준은 바꾸지 않았다.
