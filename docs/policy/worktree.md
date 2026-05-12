# Worktree policy

이 문서는 구현의 근거가 되는 내부 정책을 정리한다. 사용자 명령 규칙은 `docs/spec/cli.md`가 우선한다.

## Document Boundary

- `README`: 빠른 설치와 사용 흐름
- `docs/spec/cli.md`: 실제 명령/옵션/출력 규칙
- `docs/policy/worktree.md`: 경로, safety, non-TTY 같은 내부 원칙
- `CHANGELOG.md`: release-please가 관리하는 변경 이력
- `docs/release/notes.md`: 과거 수동 릴리즈 노트 archive

## Git Context And Registered Entries

- `wt`는 filesystem scan이 아니라 `git worktree list`의 registered entry를 기준으로 동작한다.
- `wt path`는 path-only 철학을 유지하기 위해 기본적으로 path 존재 여부나 `.git` 상태를 추가 검사하지 않는다.
- filesystem 이상 징후와 정리 신호는 `wt list --verify`, `wt prune`, `wt cleanup`이 담당한다.

## Primary Root Policy

- primary root는 `git rev-parse --path-format=absolute --git-common-dir` 결과를 기준으로 계산한다.
- linked worktree 안에서 실행해도 create/root 기본 경로는 현재 linked worktree 아래가 아니라 primary root를 기준으로 잡는다.
- 이 정책은 nested worktree 경로 생성을 막기 위해 유지한다.

## Default Create Path And Overrides

- 기본 생성 경로는 `<primary-root>/.wt/<branch>`다.
- `.wt/`는 로컬 산출물 경로로 간주한다.
- override 우선순위는 `--path` > `--root` > `WT_ROOT` > repo-local git config `wt.root` > default root 다.
- `--root`, `WT_ROOT`, `wt.root`가 상대 경로이면 모두 `<primary-root>` 기준으로 해석한다.
- `wt.root`는 repo-local config만 읽는다.

## Create Safety

- `wt create`와 `wt path --create`는 생성 전에 registered entry를 먼저 본다.
- live registered worktree가 이미 있으면 새로 만들지 않고 그 path를 반환한다.
- 로컬 브랜치가 이미 있으면 새 브랜치를 만들지 않고 attach 한다.
- 동일 브랜치 또는 query에 대응되는 registered `prunable` entry가 있으면 자동 복구하지 않고 실패한다.
- stale registered entry 정리는 사용자가 `wt prune --apply`로 명시적으로 수행해야 한다.
- 최종 생성 경로 preflight를 수행한다.
- 기존 파일은 usage error(exit code 2)로 실패한다.
- 기존 디렉터리는 비어있을 때만 허용하고, 비어있지 않으면 usage error(exit code 2)로 실패한다.
- symbolic link를 포함한 기타 파일 타입은 보수적으로 usage error(exit code 2)로 실패한다.
- `--dry-run`도 동일 preflight를 먼저 수행한다.

## Branch To Path Normalization

- 기본 경로 계산은 브랜치의 `/`를 하위 디렉터리로 유지한다.
- 절대 경로나 상위 디렉터리 탈출(`..`)이 되는 브랜치명은 기본 경로 계산에 사용하지 않는다.

## Remove Safety

- `wt remove`는 정상 worktree 제거 전용 명령이다.
- 기본값은 파괴적이지 않아야 한다.
- `--dry-run`은 preview-only 다.
- `--force`는 명시적 opt-in 이다.
- interactive confirm은 TTY 환경에서만 허용한다.
- non-interactive 환경에서는 `--dry-run` 또는 `--force`를 강제한다.
- current worktree와 primary worktree는 제거할 수 없다.
- `prunable` entry는 remove가 아니라 prune으로 정리한다.
- `--tui`는 선택 UX만 바꾸며 safety를 완화하지 않는다.

## Prune Safety

- `wt prune`는 stale/prunable registered entry 정리 전용이다.
- 기본값은 preview-only 다.
- 실제 변경은 `--apply`일 때만 수행한다.
- 실제 prune은 `git worktree prune --expire now` 한 번으로 제한한다.
- 정상 worktree 디렉토리를 직접 지우지 않는다.
- `--tui`는 preview 계층일 뿐 prune 범위를 바꾸지 않는다.

## Cleanup Safety

- `wt cleanup`는 `wt list`의 추천 신호를 실행하는 companion 명령이다.
- 기본값은 preview-only 다.
- `recommendedAction=prune`는 prune 정책으로만 처리한다.
- `recommendedAction=remove`는 `safeToRemove=true`인 항목에만 적용한다.
- current, primary, detached, locked, missing-path, missing-git, prunable 예외는 cleanup에서도 그대로 유지한다.

## Structured Output Consistency Policy (권장)

- 목적:
  - 자동화 소비자가 명령별 분기 없이 `action`/`reason`/`removed`/exit code를 해석할 수 있게 한다.
  - 텍스트 출력과 JSON structured output의 의미 드리프트를 줄인다.
- 적용 범위:
  - `wt list`, `wt run`, `wt remove`, `wt prune`, `wt cleanup`
  - 특히 상태 전이(미리보기/적용)와 실패 표현이 있는 명령(`run/remove/prune/cleanup`)
  - `wt doctor`는 별도 `action` 필드를 쓰지 않지만, text 상태 토큰과 JSON `status`의 의미를 동일하게 유지한다.
- 권장 통일 규칙:
  - `action`은 텍스트 출력 첫 토큰과 JSON 값을 동일 어휘로 유지한다.
  - preview/apply 상태는 `applied`로 표현하고, 실제 엔트리 제거 여부는 `removed`로 분리한다.
  - `reason`은 “판단 근거가 계산된 경우에만” 채운다. 근거가 없으면 필드 생략 또는 빈 값으로 유지한다.
  - `wt run --json`의 `exitCode`는 항상 하위 프로세스 의미를 따른다.
  - usage error는 exit code `2`, 사용자 취소는 `130`, 실행 대상 없음/모호성 같은 선택 실패는 `1`을 기본값으로 유지한다.
- 변경/이탈 절차:
  - 통일 규칙을 의도적으로 벗어나는 변경은 허용하되, 같은 PR에서 `docs/spec/cli.md`와 본 정책 문서를 함께 갱신한다.
  - 이탈 사유(호환성, 안전성, 구현 제약)와 마이그레이션 가이드를 PR 본문 `Behavior`에 명시한다.
  - 사용자-facing 변화라면 관련 스펙/정책/UX 문서를 갱신하고 PR title을 Conventional Commit 형식으로 작성한다.

## Root Policy

- `wt root`는 현재 작업 디렉터리가 linked worktree여도 primary repo root를 돌려준다.
- 셸 helper의 `wtr`는 이 정책을 그대로 노출하는 얇은 래퍼다.

## Hosting Verify Policy

- hosting merge 검증은 로컬 Git merge 검증과 분리된 opt-in 기능이다.
- 현재 지원 provider는 GitHub와 GitLab이다.
- provider는 `origin` remote URL로 감지한다.
- `gh`는 `WT_GH_BIN` 후 `PATH`, `glab`는 `WT_GLAB_BIN` 후 `PATH` 순서로 찾는다.
- 자동 로그인, 자동 브라우저 인증, 자동 fetch는 범위 밖이다.
- hosting 조회 실패는 hard error 대신 `null + reason`으로 degrade 한다.
- 로컬 merge와 hosting merge의 의미는 텍스트 마커와 JSON 필드에서 계속 분리한다.

## TUI And Non-TTY Policy

- TUI는 `stdin`과 `stderr`가 모두 TTY일 때만 허용한다.
- 화면 렌더링은 `stderr`, 최종 결과는 `stdout`에 남겨 파이프와 command substitution 안전성을 유지한다.
- 취소는 `Esc` 또는 `Ctrl+C`이며 exit code `130`을 사용한다.
- non-TTY 환경에서 `--tui`를 요청하면 usage error로 즉시 거부한다.

## Shell Helper Policy

- `wt` 바이너리 자체는 `cd`하지 않는다.
- `wtg`, `wcd`, `wtr`는 모두 셸이 `wt path` 또는 `wt root`의 path-only 출력을 받아 이동하도록 유지한다.
- `wt init`은 출력-only 이며 사용자의 rc 파일을 자동 수정하지 않는다.
