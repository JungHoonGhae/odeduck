# 코드 리뷰에서 남은 지적 (feat/apply-probe)

Type: task
Status: open

## Question

`/code-review` 2축 리뷰(Standards 7건 · Spec 7건)에서 **지금 고치지 않은** 것들.
최악 2건(doctor masquerade, 실패할 수 없는 테스트)은 이미 수정했다.

## 남은 것 — Standards 축

- **Flag Argument** — `applyFormJS(purpose, category, fill)` 에서 `fill=false` 일 때 `purpose`
  가 무의미하다. 시그니처가 두 모드를 숨긴다. (공유 스크립트 자체는 유지 — 그게 설계의 핵심)
- **Data Clumps** — `fillAndSubmit(tctx, pk, purpose, category, confirm, dialog)` 의
  `pk/purpose/category` 는 이미 `ApplySummary` 로 태어나 있는 묶음. 요약을 먼저 만들어 넘기면 짧다.
- **Mysterious Name** — `onApplyForm` 은 이벤트 핸들러로 읽힌다. Go 관용은 `withApplyForm`.
- **배치 불일치** — `ErrFormUnreachable` 이 함수 사이에 선언(기존 sentinel 은 파일 상단).
  세션 필요 체크가 `sessionCheck`(cmd) / `ApplyCheck`(internal/doctor) 로 갈렸다.
- **Duplicated Code(경미)** — CDP URL 조립이 `wsAlive`/`browserUsable` 에 각각,
  evaluate→TrimSpace→Unmarshal 패턴이 `apply.go` 와 `probe_apply.go` 에 동형으로.

## 남은 것 — Spec 축

- **CI 에서 apply 체크가 돌지 않는다.** 기본값이 브라우저를 띄우므로 CI 는 `--skip-apply` 를
  쓸 수밖에 없다. 즉 유일한 apply 커버리지가 사람이 손으로 돌릴 때만 존재한다.
  → 헤드리스로 도는 지금 구조상 CI 에서도 가능할 수 있다. 확인 필요.
- **`killTree` 가 요구 밖이라는 지적.** 리뷰는 "죽이는 대신 다른 포트를 쓰거나 알리는 편이
  덜 파괴적"이라고 봤다. 반론: 죽이는 대상은 **우리가 띄우고 pid 를 기록한 우리 브라우저**이고
  `closeBrowser` 가 이미 같은 일을 한다. 새 포트를 쓰면 좀비가 누적된다. 유지하되 판단을 남긴다.
- **`--apply-pk` 로 명시 지정했을 때만 skipped** — 기본 카나리 3개가 전부 막히면 drift.
  이 비대칭이 옳은지 실사용에서 확인.

## 사실 확인된 오탐 (수정 안 함)

- "프로브가 사용자 탭을 신청 폼으로 옮기고 복원하지 않는다" → **틀렸다.**
  `chromedp.NewContext(allocCtx)` 는 RemoteAllocator 에서 **새 탭**을 만들고 `cancelTab` 으로
  닫는다. 기존 탭을 건드리지 않는다.

## 왜 지금 안 고치나

전부 판단 사안이고, 라이브 검증(`03`)을 막지 않는다. 최악 2건은 검증의 **성공 기준 자체**를
무너뜨렸으므로 먼저 고쳤다. 나머지는 제품 형태가 정해진 뒤에 손대는 편이 낫다 — 일반화가
성립하면 이 코드의 절반은 다시 쓰인다.
