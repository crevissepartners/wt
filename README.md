# wt

`wt`는 `git worktree` 운영을 단순화하는 CLI입니다.
브랜치별 워크트리 생성/탐색/정리 흐름을 표준화해서, 반복적인 Git 명령과 실수 가능성을 줄이는 것이 목적입니다.
특히 여러 에이전트를 병렬로 돌리는 agentic coding 환경에서 worktree 수가 빠르게 늘어나는 상황을 쉽게 관리하는 것도 핵심 목적입니다.

## Purpose

- 워크트리 경로를 빠르게 찾고(`wt path`) 셸에서 바로 이동 가능한 출력 제공
- 브랜치별 워크트리를 안전하게 생성/재사용(`wt create`, `wt path --create`)
- stale/prunable entry와 안전 제거 대상을 분리해 정리(`wt prune`, `wt remove`, `wt cleanup`)
- 스크립트 친화 출력(`--json`, `--porcelain`)과 사람 친화 기본 출력의 균형 유지

## Installation

### macOS / Linux

권장 설치(릴리즈 태그 기준):

```sh
go install github.com/crevissepartners/wt/cmd/wt@latest
```

로컬 소스에서 설치:

```sh
go install ./cmd/wt
```

### Windows (PowerShell)

Windows Terminal이 `wt` 명령을 App Execution Alias로 점유합니다. `wt init powershell`이 출력하는 helper는 `where.exe`로 진짜 `wt.exe`를 찾고 **`wtp`** 별칭으로 노출하므로, alias를 직접 끄지 않아도 동작합니다.

```powershell
# 1) Go가 설치되어 있어야 합니다 (https://go.dev/dl/).
go install github.com/crevissepartners/wt/cmd/wt@latest

# 2) 첫 설정은 절대경로로 한 번만 (이때는 'wt'가 WT alias에 가려져 있으므로):
& "$env:USERPROFILE\go\bin\wt.exe" init powershell >> $PROFILE
. $PROFILE

# 3) 이후엔 wtp / wtg / wtr / wcd 그대로 사용:
wtp --version
wtp list
wtg feature/login   # cd helper (worktree 경로로 이동)
```

별도 옵션으로 Windows Terminal alias를 끄고 싶다면 설정 → 앱 → 고급 앱 설정 → 앱 실행 별칭에서 "Windows Terminal (wt.exe)"을 Off. 끄면 `wt`도 그대로 동작합니다.

### 버전 확인

```sh
wt --version       # macOS/Linux
wtp --version      # Windows (PowerShell)
```

## Quick Start

> Windows (PowerShell)에서는 아래 예시의 `wt`를 `wtp`로 바꿔 읽으세요 (`wtp list`, `wtp path ...`). Windows Terminal alias와 충돌을 피하기 위한 별칭입니다. `wtr`/`wtg`/`wcd` cd helper는 그대로입니다.

```sh
# 1) 현재 repo의 worktree 상태 확인
wt list

# 2) query로 worktree 경로 찾기
wt path feature/login

# 3) 없으면 생성하면서 경로 얻기
wt path --create feature/login

# 4) 안전하게 정리(미리보기)
wt prune
wt remove feature/login --dry-run
```

셸 helper를 쓰면 `cd` 흐름이 더 간단해집니다.

```sh
eval "$(wt init zsh)"
wtg feature/login   # == cd "$(wt path feature/login)"
```

```powershell
# PowerShell (after Installation bootstrap; loads wtp/wtr/wtg/wcd)
. $PROFILE
wtg feature/login   # == Set-Location (wt path feature/login)
```

## Core Commands

| Command | 용도 |
| --- | --- |
| `wt list` | registered worktree 목록 조회 |
| `wt path [query]` | query에 맞는 worktree 경로 출력 (path-only) |
| `wt create <branch>` | 브랜치용 worktree 생성/재사용 |
| `wt root` | primary repo root 출력 |
| `wt run <query> -- <cmd...>` | 선택된 worktree에서 명령 실행 |
| `wt remove [query]` | 안전 규칙 기반 worktree 제거 |
| `wt prune` | prunable entry 미리보기/정리 |
| `wt cleanup` | 추천 액션(prune/remove) 일괄 처리 |
| `wt doctor` | 환경/설치/컨텍스트 진단 |
| `wt upgrade` | 릴리즈 버전으로 자체 업그레이드 |
| `wt init <shell>` | 셸 함수 출력 (`wtr`, `wtg`, `wcd`; `zsh`/`bash`/`fish`/`powershell`) |

상세 동작/옵션은 [docs/spec/cli.md](docs/spec/cli.md)를 참고하세요.

## Common Workflows

생성/이동:

```sh
wt create feature/a
cd "$(wt path feature/a)"
```

없으면 생성 후 이동:

```sh
cd "$(wt path --create feature/a)"
```

특정 워크트리에서 테스트 실행:

```sh
wt run feature/a -- go test ./...
```

정리(권장 순서):

```sh
wt prune              # stale entry preview
wt prune --apply      # stale entry prune
wt cleanup            # remove/prune 추천 액션 preview
wt cleanup --apply    # 안전 기준 만족 항목만 실제 적용
```

## Upgrade & Release

최신 릴리즈로 업그레이드:

```sh
wt upgrade
```

주의:

- `wt upgrade`는 `PATH`로 찾은 설치된 `wt`만 업그레이드한다.
- 레포 안의 `./wt` 같은 로컬 빌드 바이너리에서 실행하면 실패하고 `wt upgrade`로 다시 실행하라고 안내한다.
- Windows에서는 alias 충돌로 `wt upgrade` 대신 `wtp upgrade`를 쓴다 (또는 alias를 끈 상태라면 `wt upgrade`).

특정 버전으로 업그레이드:

```sh
wt upgrade --version v0.10.2
```

실행 명령만 확인:

```sh
wt upgrade --dry-run
```

릴리즈 정책/태그 규칙은 [docs/release/process.md](docs/release/process.md)를 참고하세요.

## Safety Rules

- 파괴적 동작은 기본값으로 실행하지 않습니다.
- remove/prune 계열은 preview/confirm 흐름을 우선합니다.
- 정상 출력은 `stdout`, 에러/안내는 `stderr`를 사용합니다.
- TUI는 `stdin`/`stderr`가 모두 TTY일 때만 동작합니다.

## Agent Skill Registration

`wt-worktree` 스킬을 등록하면 Claude/Codex에서 동일한 worktree 운영 규칙을 재사용할 수 있습니다.

현재 권장 구조는 사용자 작성 스킬을 dotfiles에 두고, 각 도구의 skill loader가 읽는 위치에 복사하거나 symlink하는 방식입니다.

```text
<dotfiles>/.claude/skills/wt-worktree/SKILL.md
<dotfiles>/.codex/skills/wt-worktree/SKILL.md
```

도구별 전역 등록 대상:

```text
Claude Code: ~/.claude/skills/wt-worktree/SKILL.md
Codex:      $CODEX_HOME/skills/wt-worktree/SKILL.md
```

Codex의 `$CODEX_HOME/skills/.system/*`는 번들 시스템 스킬 영역이므로 `wt-worktree` 같은 사용자 스킬 등록 대상으로 사용하지 않습니다. Claude Code는 필요하면 리포 전용 `.claude/skills/wt-worktree/SKILL.md`도 둘 수 있습니다.

스킬 파일에는 아래 흐름을 포함해야 합니다.

- `wt --version`, `wt list` 선확인
- `wt path --create <branch>`로 작업 경로 확보
- `wt run <branch> -- <cmd...>`로 실행
- `wt prune`/`wt cleanup` preview 후 정리
- 에이전트가 worktree path에서 Git 명령을 실행할 때는 `cd <path> && git ...` 대신 `git -C <path> ...` 사용

상세 템플릿과 도구별 차이는 [docs/ux/agents.md](docs/ux/agents.md)를 참고하세요.
레포에 포함된 복붙용 샘플은 [docs/examples/skills/wt-worktree/SKILL.md](docs/examples/skills/wt-worktree/SKILL.md) 입니다.

## Documentation

- CLI 스펙: [docs/spec/cli.md](docs/spec/cli.md)
- 셸 통합: [docs/ux/shell.md](docs/ux/shell.md)
- TUI 가이드: [docs/ux/tui.md](docs/ux/tui.md)
- 에이전트 연동 가이드(Claude/Codex): [docs/ux/agents.md](docs/ux/agents.md)
- 변경 이력: [CHANGELOG.md](CHANGELOG.md)
- 릴리즈 절차: [docs/release/process.md](docs/release/process.md)

## Development

```sh
make build
make test
make premerge
```

## License

MIT. See [LICENSE](LICENSE).
