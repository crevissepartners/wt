# AGENTS.md (repo: wt)

이 파일은 `wt` 레포에서 자동화 코딩 에이전트가 일관되고 안전하게 작업하기 위한 로컬 규칙/런북입니다.

## Scope
- 목적: `git worktree`를 더 쉽게 쓰기 위한 CLI 헬퍼
- Non-goals:
  - Git 자체를 대체하는 복잡한 UI/대규모 설정 시스템
  - 사용자의 로컬 환경을 파괴할 수 있는 기본 동작(삭제/초기화 등)은 반드시 opt-in

## Repo layout
- 현재: `main.go` (임시/스캐폴딩)
- 권장(성장 시):
  - `cmd/wt/` : 바이너리 엔트리포인트(`main`)
  - `internal/` : git/worktree 로직, resolver, config
  - `scripts/` : 개발 보조 스크립트(선택)

## Tooling
- Go 버전은 `go.mod`를 따른다.
- 포맷: `gofmt` (필수)
- 의존성: 꼭 필요할 때만 추가. CLI 프레임워크를 쓰면 `cobra` 또는 `urfave/cli` 중 하나로 통일(혼용 금지).

## Common commands
- 포맷(수정): `make fmt`
- 포맷(검증): `make fmt-check`
- 자동 수정(Go 1.26+): `make fix` (파일을 수정함)
- 자동 수정(diff): `make fix-diff` (파일을 수정하지 않음)
- 필수 체크: `make check` (현재 작업트리 기준)
- 테스트: `make test`
- 빌드: `make build`
- 실행: `make run ARGS="--help"`
- PR 생성(선택): `make pr-create` (필요 시 `gh auth login` 먼저)

## Skill usage policy
- worktree 관련 작업(생성/탐색/정리/병렬 브랜치 운영)은 기본적으로 `wt-worktree` 스킬을 사용한다.
- 스킬이 사용 불가한 환경이면, 동일한 표준 흐름을 수동으로 따른다.
  - `wt --version`, `wt list` 선확인
  - `wt path --create <branch>`로 작업 경로 확보
  - `wt run <branch> -- <cmd...>`로 실행
  - `wt prune`/`wt cleanup`은 preview 후 적용
- 에이전트는 worktree 관련 작업에서 `git worktree` 원시 명령보다 `wt` 명령을 우선 사용한다.

## Docs hygiene
- 사용자에게 보이는 동작(명령/옵션/출력)이 바뀌면:
  - 스펙은 `docs/`에 반영하고
  - 릴리즈 노트는 기능 PR에서 직접 작성하지 않는다. release-please가 Conventional Commit 기반으로 `CHANGELOG.md`를 갱신한다.

## Output/UX rules
- `wt goto ...` 계열은 셸에서 `cd "$(wt goto ...)"`가 가능하도록 **경로만** 출력하는 모드를 기본으로 유지한다.
- 사람이 보는 출력과 스크립트용 출력이 충돌하면:
  - 기본은 사람이 보기 좋게
  - `--json` 또는 `--porcelain`(스크립트용) 옵션을 제공
- 에러 메시지는 stderr, 정상 출력은 stdout.

## Safety rules (중요)
- 사용자가 명시하지 않은 파괴적 동작 금지:
  - `git reset --hard`, `git clean -fdx`, 임의의 브랜치 삭제, 강제 prune 등
- `wt remove`/`--create` 같이 파일/폴더를 건드리는 동작은:
  - 대상(worktree path, branch)을 명확히 출력
  - 기본적으로 확인(confirmation) 또는 드라이런(`--dry-run`) 지원을 고려

## Specs (docs)
- CLI/옵션 스펙(초안): `docs/spec/cli.md`
- 정책(초안): `docs/policy/worktree.md`
- 셸 통합/완성(초안): `docs/ux/shell.md`
- TUI 스펙(초안): `docs/ux/tui.md`
- 로드맵: `docs/roadmap/README.md`
- 변경 이력: `CHANGELOG.md` (release-please 관리)
- 과거 수동 릴리즈 노트: `docs/release/notes.md` (legacy archive)

## Versioning
- 버전은 release-please가 관리한다.
- 현재 버전 source는 `.release-please-manifest.json`과 `internal/buildinfo/buildinfo.go`의 `x-release-please-version` marker다.
- 기능/수정 PR은 release-please version 파일이나 `CHANGELOG.md`를 직접 편집하지 않는다.
- release-please PR만 `.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`, `CHANGELOG.md`를 갱신한다.
- 릴리즈 Git tag는 반드시 `vX.Y.Z` 형식을 사용한다.
- 릴리즈 설치 기준 경로는 `go install github.com/crevissepartners/wt/cmd/wt@latest`로 유지한다.
- main에 squash merge되는 PR title은 release-please가 읽는 Conventional Commit subject다. 사용자-facing 변경은 `feat: ...` 또는 `fix: ...`처럼 엄격히 작성한다.

## Merge gate (to main)
- main에 머지(또는 PR ready) 전에 `make premerge`를 통과시킨다.
- 사용자에게 보이는 변경(명령/옵션/출력/기본값)이 포함되면:
  - 같은 PR에서 관련 스펙/정책/UX 문서를 갱신한다.
  - PR title을 Conventional Commit 형식으로 작성한다.

## Release automation
- `release-please-action`이 main push를 감시하고 Conventional Commit subject를 누적해 release PR을 생성/갱신한다.
- release PR은 `.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`, `CHANGELOG.md`를 함께 갱신한다.
- release PR을 merge하면 release-please가 `vX.Y.Z` 태그와 GitHub Release를 생성한다.
- CI는 tag push에서 `vX.Y.Z` 형식과 `internal/buildinfo.Version` 일치 여부를 검증한다.
- 자동화 에이전트는 릴리즈 정책을 우회하기 위해 수동 tag push를 기본 전략으로 사용하지 않는다(예외 상황은 PR 본문에 사유 명시).
- Non-Conventional commit subject는 release-please가 release note 계산에서 건너뛸 수 있으므로 PR title을 엄격히 유지한다.

## PR guidelines
- PR 작성 가이드: `docs/pr-guidelines.md`

## Orchestrator context rules
- 오케스트레이터(상위 에이전트)가 작업을 시작할 때는 아래 문서를 **항상 `@` context로 추가**한다.
  - `@docs/prompts/feature-pr-template.md`
  - `@docs/pr-guidelines.md`
  - `@AGENTS.md`
- 작업 시작 응답에서, 위 문서 기준으로 이번 작업의 `Goal`, `Constraints`, `Definition of Done`을 3~6줄로 먼저 요약한다.
- 사용자-facing 변경 작업에서는 템플릿(`docs/prompts/feature-pr-template.md`)의 섹션 구조를 유지해 PR 본문을 작성한다.
- `@` context 문서에 없는 내용을 추측으로 규칙화하지 않는다.

## PR / agent writing rules
- 에이전트가 PR 본문/설명/체크리스트를 작성하거나 수정할 때는 **기본적으로 한글**로 작성한다.
- 사용자에게 보이는 변경이 있는 PR이면, PR 본문에 **E2E 재현 가이드**를 포함한다.
- 사용자에게 보이는 변경이 있는 PR이면, PR 본문에 **E2E Done 체크리스트**를 추가한다.
  - 각 항목에 실행/스킵 상태, exit code, stdout/stderr 핵심 요약, 근거(evidence)를 남긴다.
  - 현재 repo에서 바로 실행하는 방법이 가능하면 그 경로를 우선 적는다.
  - 현재 repo 상태에 의존적이면, 임시 repo 기반 재현 방법도 함께 적는다.
- PR/이슈/문서 예시에 **민감정보나 로컬 환경 식별 정보**를 넣지 않는다.
  - 금지 예: 실제 홈 디렉토리, 실제 checkout 경로, 사내/개인 전용 URL, 로컬 토큰/키/계정명
  - 대신 `/path/to/wt`, `<repo>`, `<query>` 같은 플레이스홀더를 사용한다.
- PR 본문에 stdout/stderr, exit code, 안전성(safety) 관련 변화가 있으면 반드시 명시한다.
