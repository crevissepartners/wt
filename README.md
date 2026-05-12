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

권장 설치(릴리즈 태그 기준):

```sh
go install github.com/crevissepartners/wt/cmd/wt@latest
```

로컬 소스에서 설치:

```sh
go install ./cmd/wt
```

버전 확인:

```sh
wt --version
```

## Quick Start

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
| `wt init <shell>` | 셸 함수 출력 (`wtr`, `wtg`, `wcd`) |

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

글로벌(모든 프로젝트) 등록:

```text
~/.claude/skills/wt-worktree/SKILL.md
~/.codex/skills/wt-worktree/SKILL.md
```

리포 전용 등록:

```text
.claude/skills/wt-worktree/SKILL.md
```

스킬 파일에는 아래 흐름을 포함하는 것을 권장합니다.

- `wt --version`, `wt list` 선확인
- `wt path --create <branch>`로 작업 경로 확보
- `wt run <branch> -- <cmd...>`로 실행
- `wt prune`/`wt cleanup` preview 후 정리

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
