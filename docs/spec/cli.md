# CLI spec

이 문서는 `main`에 실제 구현된 `wt` CLI의 사용자-facing 규칙만 기록한다.

## Global Rules

- Git 컨텍스트는 현재 디렉토리에서 결정한다.
- 정상 출력은 `stdout`, 에러와 note는 `stderr`를 사용한다.
- 사람이 보는 출력과 스크립트 출력이 충돌하면 `--json` 또는 `--porcelain`을 제공한다.
- 경로를 돌려주는 명령(`wt path`, `wt root`, `wt create`)의 기본 출력은 path-only 다.
- `--tui` 화면에서 긴 줄(branch/path/help/filter)은 현재 터미널 가로폭에 맞춰 말줄임(`...`) 처리한다.

## Local `.wt/config.toml`

`wt`는 primary repo의 `<primary-root>/.wt/config.toml`을 로컬 설정 파일로 읽는다. 이 파일은 primary repo 기준의 worktree 보조 설정이며, 현재 구현된 항목은 post-create managed symlink뿐이다.

```toml
[[hooks.postCreate.symlinks]]
source = ".env.local"
target = ".env.local"
onMissingSource = "skip"
onExistingTarget = "fail"
```

규칙:

- `source`는 primary repo root 기준 상대 경로다.
- `target`은 새 worktree root 기준 상대 경로다.
- `target`을 생략하면 `source`와 같은 상대 경로를 사용한다.
- `source`와 `target`은 절대 경로와 상위 디렉터리 탈출(`..`)을 허용하지 않는다.
- `onMissingSource`는 `skip` 또는 `fail`이며 기본값은 `skip`이다.
- `onExistingTarget`은 `fail` 또는 `skip`이며 기본값은 `fail`이다.
- target이 이미 같은 source를 가리키는 symlink이면 성공으로 처리한다.
- target이 다른 파일, 디렉터리, 또는 다른 symlink이고 `onExistingTarget = "fail"`이면 실패한다.
- symlink는 상대 symlink로 생성한다.

### Structured output 규칙 (remove/prune/cleanup)

- `action`은 텍스트 출력의 첫 토큰과 같은 어휘를 JSON에도 그대로 사용한다.
  - preview: `would-prune`, `would-remove`, `skip`
  - apply 결과: `pruned`, `removed`, `kept`
- `applied`는 실제 apply 경로를 실행했는지 여부다.
  - preview-only 결과는 항상 `false`
  - apply 실행 후 확정된 결과는 `true`
- `removed`는 해당 엔트리가 실제로 제거되었는지 여부다.
  - preview-only 결과는 항상 `false`
  - apply 실행 시 `action=pruned|removed`면 `true`, `action=kept|skip`면 `false`
- `reason`은 명령이 판단 근거를 계산하는 경우에만 포함한다(`cleanup`, `prune`).
- `wt run --json`의 `exitCode`는 `wt` 자체가 아니라 하위 프로세스 exit code를 의미한다.

## `wt list`

현재 repo의 registered worktree를 나열한다.

기본 출력:

- 한 줄에 `basename  branch  short-head  path`를 출력한다.
- 추가 신호가 있으면 `[...]` 마커를 붙인다.
- 마커는 `locked`, `prunable`, `current`, `primary`, `missing-path`, `missing-git`, `merged`, `merged-hosting:<provider>`, `stale`, `safe-remove`, `recommend:prune|remove`를 사용한다.

옵션:

- `--json`: 구조화 JSON 출력
- `--porcelain`: `git worktree list --porcelain` 원문 출력
- `--verify`: path/.git 상태와 base ref merge 여부를 함께 검증
- `--verify-hosting`: hosting(PR/MR) 기준 merge 여부를 함께 검증
- `--base <ref>`: `--verify` 또는 `--verify-hosting`의 기준 ref. 기본값은 `origin/HEAD` 또는 `main`
- `--stale`: `stale=true` 항목만 출력
- `--safe-to-remove`: `safeToRemove=true` 항목만 출력
- `--recommended <none|prune|remove>`: `recommendedAction` 값으로 필터

조합 규칙:

- `--json`과 `--porcelain`은 함께 쓸 수 없다.
- `--porcelain`과 `--verify-hosting`은 함께 쓸 수 없다.
- `--porcelain`과 `--stale`/`--safe-to-remove`/`--recommended`는 함께 쓸 수 없다.

검증 규칙:

- `--json --verify`는 각 항목에 `pathExists`, `dotGitExists`, `valid`, `mergedIntoBase`, `baseRef`를 포함한다.
- detached 또는 branch 없는 항목은 `mergedIntoBase: null`을 사용한다.
- `--verify-hosting`은 GitHub(`gh`)와 GitLab(`glab`)만 지원한다.
- hosting 검증 실패는 명령 전체 실패로 승격하지 않는다.
- hosting 검증이 불가능하면 텍스트 출력에는 note, JSON에는 `mergedViaHosting: null`과 `hostingReason`을 남긴다.
- hosting change metadata key는 lower camelCase를 기본으로 하되, `URL` 같은 널리 쓰이는 initialism은 보존한다. canonical key는 `hostingChangeNumber`, `hostingChangeTitle`, `hostingChangeURL`이다.
- compatibility policy: `hostingChangeUrl`은 기존 스크립트 호환성을 위한 deprecated alias로 당분간 함께 유지한다. 새 소비자는 `hostingChangeURL`만 사용해야 한다.
- GitHub 바이너리 탐색 순서는 `WT_GH_BIN` 후 `PATH`다.
- GitLab 바이너리 탐색 순서는 `WT_GLAB_BIN` 후 `PATH`다.

파생 신호:

- `stale=true`: `prunable=true` 이거나 path/.git 누락
- `recommendedAction=prune`: `prunable=true`
- `recommendedAction=remove`: `prunable=false`, `current=false`, `primary=false`, `detached=false`, `locked=false`, `missing-path=false`, `missing-git=false`, 그리고 로컬 merge 또는 hosting merge가 확인된 경우
- `safeToRemove=true`: `recommendedAction=remove`와 같은 안전 기준을 만족한 경우

필터 규칙:

- `--stale`, `--safe-to-remove`, `--recommended`는 모두 AND로 결합된다.
- 텍스트 기본 출력과 `--json` 출력에 동일한 필터 semantics를 적용한다.
- `--safe-to-remove`와 `--recommended remove`는 merge 검증 결과(`--verify`/`--verify-hosting`)가 없으면 대부분 매칭되지 않는다.

## `wt path [query]`

query와 매칭되는 registered worktree path를 출력한다.

규칙:

- 선택 기준은 filesystem scan이 아니라 `git worktree list`의 registered entry다.
- 기본 출력은 path-only 다.
- `query` 없이 실행하려면 `--tui`가 필요하다.
- 후보가 0개면 exit code `1`과 함께 `no matches` 에러를 반환한다.
- 후보가 1개면 자동 선택한다.
- 후보가 2개 이상이면 기본 동작은 에러 + 후보 목록이다.
- ambiguous 상황에서 `--tui`를 주면 후보만 TUI로 다시 고른다.
- `wt path <query> --tui`는 `query`를 초기 filter 값으로 사용한다.
- 취소(`Esc`, `Ctrl+C`)는 exit code `130`이다.

옵션:

- `--json`: `{path, branch}`
- `--create`: 없으면 worktree 생성
- `--path <dir>`: `--create` 시 최종 경로 지정
- `--root <dir>`: `--create` 시 기본 root 지정
- `--from <ref>`: `--create` 시 start point 지정
- `--dry-run`: `--create` 실행 계획만 출력(실제 생성 없음)
- `--tui`: 전체 목록 또는 다중 후보에서 TUI 선택
- `--no-tui`: 다중 후보 시에도 TUI 없이 실패

조합 규칙:

- `--tui`와 `--no-tui`는 함께 쓸 수 없다.
- `--tui`와 `--create`는 함께 쓸 수 없다.
- `--path`, `--root`, `--from`, `--dry-run`은 `--create`가 있을 때만 허용된다.
- `query` 없이 `--no-tui`는 허용되지 않는다.

`--create` 규칙:

- 동일 브랜치의 live registered worktree가 있으면 그 path를 그대로 반환한다.
- 로컬 브랜치가 있고 live registered worktree만 없으면 `git worktree add <path> <branch>`로 attach 한다.
- 로컬 브랜치가 없고 `origin/<branch>`가 있으면 `git worktree add -b <branch> <path> origin/<branch>`를 사용한다.
- 둘 다 없으면 `git worktree add -b <branch> <path> <from>`을 사용한다.
- 동일 브랜치 또는 query에 대응되는 registered `prunable` entry가 있으면 자동 복구하지 않고 실패하며 `wt prune --apply`를 안내한다.
- 새 worktree 생성 성공 직후 `<primary-root>/.wt/config.toml`의 `[[hooks.postCreate.symlinks]]` 항목을 적용한다.
- 이미 존재하는 live registered worktree path를 반환하는 경우에는 post-create symlink를 다시 적용하지 않는다.
- post-create symlink 실패 시 명령은 non-zero로 실패하지만, 이미 생성된 worktree를 자동 삭제하지 않는다.
- 최종 생성 경로 preflight를 먼저 수행한다:
  - 경로가 없으면 통과
  - 기존 파일이면 usage error(exit code 2)
  - 기존 디렉터리가 비어있으면 통과
  - 기존 디렉터리가 비어있지 않으면 usage error(exit code 2)
  - symbolic link를 포함한 기타 타입은 usage error(exit code 2)
- `--dry-run`도 동일 preflight를 수행하고, 통과 시에만 `stderr`에 preview command를 출력한다.
- `--dry-run`은 post-create symlink를 실제 생성하지 않고 `stderr`에 preview만 출력한다.

## `wt root`

현재 Git 컨텍스트의 primary repository root를 출력한다.

규칙:

- 기본 출력은 path-only 다.
- linked worktree 안에서 실행해도 primary repo root를 출력한다.

옵션:

- `--json`: `{root}`

## `wt run <query> -- <cmd...>`

`wt path`와 같은 매칭 규칙으로 worktree를 고른 뒤 그 디렉토리에서 명령을 실행한다.

규칙:

- query 매칭과 ambiguous 처리, exit code는 `wt path`와 같다.
- 기본 모드는 하위 프로세스의 `stdout`/`stderr`와 exit code를 그대로 전달한다.
- `--json` 사용 시 하위 프로세스 출력은 중계하지 않고 `{path, command, exitCode}`만 `stdout`에 쓴다.
- 하위 프로세스가 non-zero면 `wt`도 같은 exit code로 종료하고, JSON `exitCode`에도 같은 값을 쓴다.

옵션:

- `--json`

## `wt create <branch>`

branch용 worktree를 만든다.

규칙:

- 기본 출력은 path-only 다.
- 기본 경로는 `<primary-root>/.wt/<branch>`다.
- `--root`, `WT_ROOT`, repo-local git config `wt.root`는 모두 `<primary-root>` 기준으로 해석한다.
- 우선순위는 `--path` > `--root` > `WT_ROOT` > repo-local `wt.root` > default root 다.
- 동일 브랜치의 live registered worktree가 있으면 그 path를 반환한다.
- 동일 브랜치의 registered `prunable` entry가 있으면 실패하고 `wt prune --apply`를 안내한다.
- 새 worktree 생성 성공 직후 `<primary-root>/.wt/config.toml`의 `[[hooks.postCreate.symlinks]]` 항목을 적용한다.
- 이미 존재하는 live registered worktree path를 반환하는 경우에는 post-create symlink를 다시 적용하지 않는다.
- post-create symlink 실패 시 명령은 non-zero로 실패하지만, 이미 생성된 worktree를 자동 삭제하지 않는다.

옵션:

- `--path <dir>`
- `--root <dir>`
- `--from <ref>`
- `--dry-run`

`--dry-run` 규칙:

- 실제 생성 대신 실행될 `git worktree add ...` 명령을 `stderr`에 출력한다.
- 반환값은 실제 생성 시 사용할 path다.
- 실제 실행 전 최종 생성 경로 preflight를 동일하게 수행한다.
- post-create symlink는 실제 생성하지 않고 `stderr`에 preview만 출력한다.

## `wt remove [query]`

선택한 worktree를 제거한다.

규칙:

- `query` 없이 실행하려면 `--tui`가 필요하다.
- `--dry-run`이면 preview-only 다.
- `--force`이면 추가 확인 없이 즉시 제거한다.
- `--dry-run`과 `--force`가 모두 없으면 interactive TTY에서만 confirm prompt를 사용한다.
- non-interactive 환경에서는 `--dry-run` 또는 `--force`가 필요하다.
- 현재 실행 중인 worktree는 제거할 수 없다.
- primary worktree는 제거할 수 없다.
- `prunable` entry는 remove 대상이 아니며 `wt prune --apply`를 사용해야 한다.
- 실제 삭제는 `git worktree remove --force <path>`를 사용한다.
- remove 실행 전 대상 worktree 내부 파일/디렉터리(예: `.cache`)의 readonly 권한을 owner-write 가능 상태로 보정한 뒤 삭제를 시도한다.
- readonly 항목 때문에 실패하면 error에 `target=<path>`와 함께 진단 메시지를 포함한다.

옵션:

- `--dry-run`
- `--force`
- `--json`: `{path, branch, action, applied, removed}`
- `--tui`: query 생략 또는 다중 후보 시 TUI 선택

TUI 규칙:

- `wt remove --tui`는 전체 registered worktree 목록을 대상으로 선택한다.
- `wt remove <query> --tui`는 0개면 실패, 1개면 바로 선택, 2개 이상이면 TUI로 고른다.
- TUI를 써도 current/primary/prunable safety rule은 그대로 유지된다.
- 취소는 exit code `130`이다.

출력 규칙:

- text 출력은 `would-remove` 또는 `removed` 한 줄이다.
- interactive confirm prompt는 `stderr`에 `Remove worktree <path> (<branch>)? [y/N]` 형식으로 출력한다.

## `wt prune`

stale/prunable registered entry를 미리 보거나 정리한다.

규칙:

- 기본 동작은 preview-only 다.
- 대상은 `git worktree list`에서 `prunable`로 표시되는 entry만이다.
- `--apply`가 있을 때만 `git worktree prune --expire now`를 실행한다.
- 정상 worktree 디렉토리를 직접 삭제하지 않는다.

옵션:

- `--apply`
- `--json`: `{path, branch, pruneReason, reason, action, applied, removed}` 배열
- `--tui`: prunable entry preview를 TUI로 표시

TUI 규칙:

- `--tui`와 `--json`은 함께 쓸 수 없다.
- `--tui`는 `stdin`과 `stderr`가 모두 TTY일 때만 허용된다.
- `wt prune --tui`는 preview-only 기본값을 유지한다.
- `wt prune --tui --apply`는 preview 후 confirm prompt를 거친 뒤 prune 한다.
- preview 취소는 exit code `130`, confirm 거부는 `wt prune: aborted`다.

출력 규칙:

- text 출력은 `would-prune`, `pruned`, `kept`를 사용한다.
- `--apply` 후에는 prune 전 후보 목록과 prune 후 목록을 비교해 `removed`를 계산한다.

## `wt cleanup`

`wt list`의 추천 신호를 실제 prune/remove 실행과 연결한다.

규칙:

- 기본 동작은 preview-only 다.
- `recommendedAction=prune`는 `wt prune`과 같은 정책으로 처리한다.
- `recommendedAction=remove`는 `safeToRemove=true`인 항목에만 적용한다.
- 실제 prune은 `git worktree prune --expire now`
- 실제 remove는 `git worktree remove --force <path>`
- remove 실행 경로는 `wt remove`와 동일한 readonly 권한 보정/실패 진단 규칙을 사용한다.

옵션:

- `--apply`
- `--json`: `{path, branch, recommendedAction, action, applied, removed, reason, safeToRemove, ...verifyFields}` 배열
  - local verify 필드는 `pathExists`, `dotGitExists`, `valid`, `mergedIntoBase`, `baseRef`를 포함한다.
  - hosting verify가 수행되면 `hostingProvider`, `hostingKind`, `mergedViaHosting`, `hostingReason`, `hostingChangeNumber`, `hostingChangeTitle`, `hostingChangeUrl`를 포함한다.
- `--tui`: 추천된 prune/remove 후보를 TUI에서 선택해 preview/apply 대상으로 좁힌다.

TUI 규칙:

- `--tui`와 `--json`은 함께 쓸 수 없다.
- `--tui`는 `stdin`과 `stderr`가 모두 TTY일 때만 허용된다.
- `wt cleanup --tui`는 선택된 후보만 `would-prune`/`would-remove` preview를 출력한다.
- `wt cleanup --tui --apply`는 선택된 후보만 대상으로 confirm prompt 이후 실행한다.
- review 취소는 exit code `130`, confirm 거부는 `wt cleanup: aborted`다.

출력 규칙:

- text 출력은 `would-prune`, `would-remove`, `skip`, `pruned`, `removed`, `kept`를 사용한다.
- 각 line은 `action  path  (branch)  [reason]` 형식이다.
- remove 이유는 `merged:<base>` 또는 `merged-hosting:<provider>[#number]`처럼 짧게 출력한다.

## `wt doctor`

현재 실행 환경에서 `wt` 진단 정보를 출력한다.

규칙:

- 진단 실패가 곧 명령 전체 실패가 되지 않도록 설계한다.
- usage error가 필요한 경우에만 exit code `2`를 사용한다.
- 기본 출력은 사람이 읽기 쉬운 text 형식이다.
- 점검 상태는 `ok`, `warn`, `unavailable`로 구분한다.
- text 출력은 각 줄의 상태 토큰(`[ok]`, `[warn]`, `[unavailable]`)이 JSON `status`와 동일 의미를 갖는다.
- 기본 점검 대상은 Git context, primary root 해석, `WT_ROOT`, repo-local `wt.root`, hosting CLI(`gh`/`glab`) 준비 상태, shell/completion 상태다.
- hosting CLI 인증 점검 key는 `hosting.gh.auth`, `hosting.glab.auth`를 사용한다.
- `SHELL`이 비어 있거나 미지원 셸이면 `shell.detect=warn`과 함께 `shell.init`, `shell.completion`을 `unavailable`로 출력해 체크 집합을 유지한다.
- completion 점검은 셸별로 정의된 예상 위치들을 순서대로 검사하고, 하나라도 존재하면 `ok`를 반환한다.

옵션:

- `--json`: 구조화 JSON 출력

## `wt upgrade`

릴리즈된 `wt` 최신 버전(또는 지정 버전)을 설치한다.

규칙:

- 기본 동작은 현재 실행 중인 `wt` 바이너리 디렉터리를 `GOBIN`으로 사용한다.
- 현재 실행 중인 바이너리가 `PATH`의 설치된 `wt`와 다르면(예: `./wt`) usage error로 실패하고 `wt upgrade`로 다시 실행하라고 안내한다.
- `--version latest`(기본값)일 때는 `go list -m -versions github.com/crevissepartners/wt`에서 최신 릴리즈 태그(`vX.Y.Z`)를 해석한 뒤 해당 버전을 설치한다.
- `--version <vX.Y.Z|latest>`로 설치 버전을 고를 수 있다.
- `--version` 값에는 `@`를 포함할 수 없다.
- 설치 명령은 `go install`을 사용한다.

옵션:

- `--version <vX.Y.Z|latest>`
- `--dry-run`: 실행 없이 install command preview만 `stderr`에 출력

출력 규칙:

- 성공 시 `stdout`에 `upgraded: <package@version>`, `install-dir: <path>`를 출력한다.
- 로컬 빌드 바이너리(`./wt`)에서 실행한 경우 usage error(exit code `2`)로 실패하고 현재 실행 파일 경로와 `PATH`의 `wt` 경로를 함께 출력한다.
- 그 외 실패 시 에러는 `stderr`로 전달되고 명령은 실패한다.

## `wt init <shell>`

셸 helper 함수를 `stdout`으로 출력한다.

지원 셸:

- `zsh`
- `bash`
- `fish`
- `powershell`

규칙:

- rc 파일을 자동 수정하지 않는다.
- 출력 상단 주석으로 opt-in 적용 경로를 짧게 안내한다(즉시 적용, rc/`$PROFILE` 영구 적용, shell 문서 위치).
- 현재 출력에는 `wtr`, `wtg`, `wcd`가 포함된다.
- `wtr`는 `wt root`, `wtg`와 `wcd`는 `wt path`를 호출해 셸이 직접 `cd`(PowerShell의 경우 `Set-Location`)하도록 연결한다.
- `wt init zsh`에는 `_wt` completion이 설치된 경우 `wtg`와 `wcd`를 `wt path` completion에 연결하는 bridge도 포함된다.
- `wt init powershell`은 Windows Terminal의 `wt` App Execution Alias 충돌을 우회한다:
  - 출력 스니펫이 `where.exe wt`로 PATH 상의 모든 `wt` 후보 중 `\WindowsApps\` 경로를 제외한 첫 항목을 `$Global:WtBin`에 캐시한다.
  - `Set-Alias -Name wtp -Value $Global:WtBin -Scope Global`로 CLI를 **`wtp`** 별칭으로 노출한다. Windows 사용자는 `wt list` 대신 `wtp list`를 호출한다.
  - `wtr`/`wtg`는 `function global:` 정의로 caller scope에 등록되며 `$Global:WtBin`을 통해 binary를 호출한다. `wcd`는 `wtg`의 alias.
  - 부트스트랩 시점에는 `wt` alias가 Terminal로 가려져 있을 수 있으므로, 첫 호출은 절대경로(예: `& "$env:USERPROFILE\go\bin\wt.exe" init powershell`)로 한다. 스니펫 헤더 주석이 동일하게 안내한다.
- completion 설치는 별도 opt-in이며 `wt completion <shell>` 기반이다. 자동 설치/자동 로드는 수행하지 않는다.
