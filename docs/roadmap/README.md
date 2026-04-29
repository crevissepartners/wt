# Roadmap

이 문서는 현재 구현 상태를 기준으로, 이미 완료된 범위와 다음 현실적 작업 순서를 구분해 기록한다.

## Already Shipped

- Core listing: `wt list`, `--json`, `--porcelain`, `--verify`, `--verify-hosting`
- Derived-signal filters: `wt list --stale`, `--safe-to-remove`, `--recommended <none|prune|remove>`
- Path selection: `wt path`, `--json`, `--create`, `--tui`, `--no-tui`
- Root and execution: `wt root`, `wt run`
- Worktree lifecycle: `wt create`, `wt remove`, `wt prune`, `wt cleanup`
- Environment diagnostics: `wt doctor`, text/JSON checks for Git context, root config, hosting CLI, shell setup
- Shell integration: `wt init <shell>`, `wtg`, `wcd`, `wtr`, `wt completion <shell>`, init/completion install guidance
- TUI flows: reusable picker core, `wt path --tui`, `wt remove --tui`, `wt prune --tui`, `wt cleanup --tui`
- Hosting integration: GitHub PR / GitLab MR merged verification
- Install/update flow: `go install`, `wt upgrade`, latest release resolution
- Agent workflow docs: reusable prompt template, skill registration guide, roadmap/backlog docs

## Next Likely Work

현재 머지 상태(기존 1/2/3 축 반영 완료)를 기준으로, 다음 순서는 `cleanup/list` polish를 작은 묶음으로 순차 마감하는 흐름이 가장 자연스럽다.

1. Package A: help/용어 정합 정리
`wt list`/`wt cleanup` 옵션 설명, review/apply/preview 용어를 문서와 help에서 동일하게 맞춘다. 이후 작업(B/C/D)의 기준선이다.

2. Package B: `wt cleanup --tui` review/apply 문구 고정
review help, continue row, confirm/abort/cancel 메시지를 계약으로 고정한다. safety와 exit code(`130`, aborted) 관련 문구는 유지한다.

3. Package C: `wt list` 필터 discoverability 강화
`--stale`/`--safe-to-remove`/`--recommended` 조합 규칙과 verify 의존성을 help/spec에서 바로 파악 가능하게 정리한다.

4. Package D: `list --json`/`cleanup --json` verify 필드 범위 마감
verify 필드 포함 조건과 deprecated alias 정리 시점을 테스트/스펙 기준으로 확정해 스크립트 소비자 관점의 마감을 완료한다.

구체 후보와 우선순위 메모는 [docs/roadmap/backlog.md](backlog.md)에 분리해 둔다.

## Not Current Scope

- Git 자체를 대체하는 대규모 UI
- 자동 로그인, 자동 fetch, 자동 브라우저 인증
- 사용자의 rc 파일이나 로컬 환경을 기본값으로 자동 수정하는 설치 방식
- 기본값이 파괴적인 remove/prune/reset 류 동작
