# Agent integration guide

이 문서는 `wt`를 Claude/Codex 에이전트 워크플로에 넣을 때 필요한 최소 흐름만 정리한다.
핵심은 helper 설치 자동화가 아니라 worktree 운영 규칙을 재사용하는 것이다.

## Baseline flow

모든 에이전트 작업에서 먼저 확인:

```sh
wt --version
wt list
```

권장 순서:

```sh
wt path --create <branch>
wt run <branch> -- <cmd...>
wt prune
wt cleanup
```

`wt`가 대신하지 않는 것:

- 에이전트 설치/인증
- 셸 rc 자동 수정
- completion 자동 설치

## Shared skill

Claude Code와 Codex는 둘 다 `SKILL.md` 기반 스킬을 쓸 수 있다.
그래서 `wt` 운영 규칙은 공통 skill 하나로 두고, 도구별 차이는 등록 경로와 호출 정책만 분리하는 편이 가장 단순하다.

샘플 파일: `docs/examples/skills/wt-worktree/SKILL.md`

스킬 본문에는 최소한 repo context 확인, `wt --version`, `wt list`, `wt path --create <branch>`, `wt run <branch> -- <cmd...>`, cleanup preview-first 정책, 그리고 에이전트용 `git -C <path>` 규칙을 포함한다.

현재 PC 기준으로는 사용자 작성 스킬이 dotfiles 아래에 있고, 도구별 loader 위치에는 복사하거나 symlink해서 등록한다.

```text
<dotfiles>/.claude/skills/wt-worktree/SKILL.md
<dotfiles>/.codex/skills/wt-worktree/SKILL.md
```

## Register the skill

### Codex

사용자 작성 스킬의 기준 위치:

```text
<dotfiles>/.codex/skills/wt-worktree/SKILL.md
```

Codex가 읽는 전역 위치:

```text
$CODEX_HOME/skills/wt-worktree/SKILL.md
```

`$CODEX_HOME/skills/.system/*`는 Codex가 제공하는 시스템 스킬 영역이다. 사용자 작성 `wt-worktree` 스킬은 `.system` 아래에 두지 않는다.

### Claude Code

사용자 작성 스킬의 기준 위치:

```text
<dotfiles>/.claude/skills/wt-worktree/SKILL.md
```

Claude Code가 읽는 위치:

```text
.claude/skills/wt-worktree/SKILL.md
~/.claude/skills/wt-worktree/SKILL.md
```

팀 또는 repo 고유 정책이 있으면 `.claude/skills/...`에 repo-local override를 두고, 개인 공통 정책은 `~/.claude/skills/...` 또는 dotfiles 관리본을 사용한다.

## Use it in prompts

예시:

```text
wt-worktree 스킬을 사용해서 <task>
```

프롬프트에는 아래 세 줄이 있으면 충분하다.

- `wt --version`, `wt list`
- `wt path --create <branch>`
- `wt run <branch> -- <cmd...>` 또는 `git -C <path> ...`로 그 worktree에서만 작업

## Install and upgrade

설치 기준:

```sh
go install github.com/crevissepartners/wt/cmd/wt@latest
```

이미 설치돼 있으면:

```sh
wt upgrade
```

helper나 skill 등록은 여전히 사용자가 직접 opt-in 해야 한다.

## Team policy

- 새 자동화/에이전트 PR에는 사용한 `wt` 명령과 exit code를 남긴다.
- 사용자-facing 변경이면 관련 스펙/UX 문서를 같이 갱신하고, PR title을 Conventional Commit 형식으로 작성한다.
- 기능/수정 PR에서는 `CHANGELOG.md`와 release-please version 파일을 직접 bump 하지 않는다.

## References

- Claude Code Agent Skills: `https://docs.claude.com/en/docs/claude-code/skills`
- Claude Code Subagents: `https://docs.anthropic.com/en/docs/claude-code/sub-agents`
