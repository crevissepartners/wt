# PR guidelines

이 문서는 `wt` 레포의 PR 본문과 검증 흐름을 정리한다.

## Core Rules

- PR 하나는 한 가지 주제만 다룬다.
- 사용자-facing 변경이 있으면 `docs/spec/cli.md`와 관련 UX/정책 문서를 함께 갱신한다.
- 파괴적 동작은 기본값으로 두지 않는다.
- 자동화 에이전트가 작성하는 PR 본문은 기본적으로 한글을 사용한다.
- 예시와 E2E 가이드에는 로컬 절대경로, 계정명, 사내 URL 같은 민감정보를 넣지 않는다.
- 재사용 프롬프트는 `docs/prompts/feature-pr-template.md`를 기본으로 사용한다.
- PR title은 squash merge 후 main에 남는 Conventional Commit subject다. release-please가 이 subject를 읽으므로 `feat: ...`, `fix: ...`, `chore: ...` 형식을 지킨다.

## Merge Gate

- 머지 전 기준은 `make premerge` 통과다.
- 기능/수정 PR은 release-please version 파일(`.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`)과 `CHANGELOG.md`를 직접 bump 하지 않는다.
- 버전과 changelog는 release-please가 생성하는 release PR에서만 갱신한다.
- 문서 정합성 PR이라도, 어떤 실제 구현 상태를 기준으로 문서를 맞췄는지 PR 본문에서 분명히 적는다.

## Release policy

- 릴리즈 태그는 `v<semver>` 형식을 사용한다. 예: `v0.10.2`
- `release-please-action`이 main push의 Conventional Commit subject를 누적해 release PR을 생성/갱신한다.
- release PR은 `.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`, `CHANGELOG.md`를 함께 갱신한다.
- release PR을 merge하면 release-please가 `vX.Y.Z` 태그와 GitHub Release를 생성한다.
- 태그는 항상 `internal/buildinfo.Version` 값과 일치해야 한다. (`tag == v<buildinfo.Version>`)
- 릴리즈/업그레이드 관련 PR은 설치 기준 경로를 `go install github.com/crevissepartners/wt/cmd/wt@latest`로 명시한다.
- 태그 push 시 `ci`의 tag 검증이 통과해야 릴리즈가 완료된 것으로 본다.
- Non-Conventional commit subject는 release-please가 건너뛸 수 있으므로 squash merge PR title을 엄격히 관리한다.

## Recommended PR Body Sections

### Summary

- 무엇을 바꿨는지
- 왜 이 변경이 필요한지
- 문서/코드 기준이 무엇인지

예시:

- `PR #1~#23 확인 기준으로 README/spec/policy 문서를 현재 main 구현과 일치시키는 문서 정리`

### User impact

- 사용자에게 보이는 명령/옵션/출력 변화
- stdout/stderr 또는 exit code 변화
- 문서-only PR이면 `문서 기준 정리만 있고 동작 변화는 없음`처럼 명시

### Behavior

- 바뀐 명령 예시 1~2개
- 문서 PR이면 어떤 명령 흐름을 기준으로 정리했는지 적는다

### Safety

- remove/prune/create 같은 동작의 기본 안전성
- `--dry-run`, `--force`, confirm, non-TTY 정책 변화 여부

### Tests

- `make premerge`
- 필요하면 관련 명령 수동 검증 커맨드

### E2E guide

- 사용자-facing PR이면 포함한다.
- 불필요하면 `N/A`와 이유를 적는다.

### E2E Done

- `E2E guide` 아래에 실행 완료 체크리스트 섹션으로 추가한다.
- 각 실행 항목에 `완료/스킵`, `exit code`, `stdout/stderr 핵심 요약`, `근거(evidence)`를 남긴다.

## Agent E2E Execution Policy

- 사용자-facing PR에서는 E2E 가이드를 작성만 하지 말고, 에이전트가 실제로 실행한다.
- 구현 작업은 기본적으로 `wt` 분리 워크트리에서 진행한다.
  - 예: `wt path --create <branch>`
- PR 본문 `E2E guide`/`E2E Done` 섹션에는 아래를 반드시 포함한다.
  - 실행한 명령 목록
  - 각 명령의 exit code
  - stdout/stderr 핵심 요약
  - 완료/스킵 상태
  - 실행 근거(evidence: 로그 요약, 캡처, 코멘트 링크 등)
  - 실행 환경(현재 repo / 임시 repo)
  - 실패 또는 스킵한 항목과 사유
- `--tui` 검증은 실제 TTY에서만 수행한다.
  - 비-TTY 환경이면 스킵 사유와 대체 검증 명령을 함께 적는다.

## E2E Guide Examples

### Option A: 현재 repo에서 빠르게 확인

```sh
wt path --create <branch>
make run ARGS="list --verify"
make run ARGS="root"
make run ARGS="path main"
make run ARGS="path --json main"
make run ARGS="remove main --dry-run"
make run ARGS="prune"
```

주의:

- 현재 repo 상태에 따라 `path main`이 없거나 `prune` 후보가 없을 수 있다.
- 이 경우 실패/스킵 사유를 PR 본문에 기록한다.

TTY 환경이면 아래를 추가로 실행한다.

```sh
make run ARGS="path --tui"
make run ARGS="remove --tui --dry-run"
make run ARGS="prune --tui"
```

### Option B: 임시 repo로 재현

```sh
tmp="$(mktemp -d)"
wt_repo="/path/to/wt"

cd "$tmp"
git init repo
cd repo
git config user.name test
git config user.email test@example.com
touch a
git add a
git commit -m init
git branch feature-a
git worktree add ../repo-feature-a feature-a

go run "$wt_repo"/cmd/wt list
go run "$wt_repo"/cmd/wt path feature-a
go run "$wt_repo"/cmd/wt root
go run "$wt_repo"/cmd/wt remove feature-a --dry-run
```

stale entry까지 보고 싶으면:

```sh
git worktree remove ../repo-feature-a
git worktree add ../repo-feature-b -b feature-b
rm -rf ../repo-feature-b
go run "$wt_repo"/cmd/wt prune
```

## Docs Section

PR 본문에는 수정한 사용자 문서를 명시한다.

- `README.md`
- `docs/spec/cli.md`
- 필요 시 `docs/ux/*`, `docs/policy/*`, `docs/roadmap/*`
- 릴리즈 정책 변경이면 `docs/release/process.md`, `docs/pr-guidelines.md`

## Prompt Template

- 팀 표준 구현 프롬프트: `docs/prompts/feature-pr-template.md`
- 권장 방식: 템플릿 변수(`<...>`)를 채운 뒤 에이전트에 전달
- 권장 구조: 시작 3~6줄 안에 `Goal`, `Constraints`, `Definition of Done`을 먼저 요약한다.
- 실전 기본형에는 skill, branch/worktree, 대상 파일, 기능 계약, 테스트, 문서 대상만 남긴다.
- 병렬 에이전트 작업이면 각 프롬프트에 branch/worktree 이름을 명시해 작업 경계를 고정한다.
- 리뷰 시 확인: 템플릿의 `Definition of Done` 체크 항목 충족 여부
