# Feature PR Prompt Template

이 문서는 에이전트에게 바로 전달할 수 있는 실전용 작업 프롬프트 템플릿이다.
목표는 짧지만 빠뜨리면 안 되는 항목은 남기는 것이다.

## Use this shape

- 시작 3~6줄 안에 `Goal`, `Constraints`, `Definition of Done`을 먼저 둔다.
- 병렬 작업이면 skill, branch, worktree 전략을 프롬프트에 명시한다.
- 애매한 `TODO:`를 남기지 말고, 범위를 줄이거나 가정을 적는다.
- PR 본문 품질 기준은 프롬프트 안에서 같이 고정한다.

## Agent contract

프롬프트를 받은 에이전트는 아래를 반드시 지켜야 한다.

- 범위를 벗어난 리팩터링을 하지 않는다.
- 구현 + 테스트 + 문서 동기화를 한 PR 안에서 완료한다.
- stdout/stderr/exit code 계약이 바뀌면 PR 본문에 명시한다.
- 머지 게이트(`make premerge`)를 통과시킨다.
- 불확실한 내용은 추측으로 문서화하지 않는다.
- 구현 작업은 기본적으로 `wt` 분리 워크트리에서 진행한다.
- 사용자-facing 변경 PR에서는 E2E 명령을 실제 실행하고 결과를 PR 본문에 남긴다.
- 기능/수정 PR에서는 release-please version 파일(`.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`)과 `CHANGELOG.md`를 직접 bump 하지 않는다.
- release-please가 PR title의 Conventional Commit subject를 읽으므로 PR title은 `feat: ...`, `fix: ...`, `chore: ...` 형식을 지킨다.

## Minimal template

```md
# [TASK TYPE]: <task_title>

@docs/prompts/feature-pr-template.md
@docs/pr-guidelines.md
@AGENTS.md

Goal
- <goal_1>
- <goal_2>

Constraints
- <must_keep_or_must_not_change>
- <scope_boundary>
- <$skill_name> 스킬을 사용한다.
- 시작 전에 `wt --version`, `wt list`를 실행한다.
- `wt path --create <branch_name>`로 분리 worktree를 만든 뒤 그 worktree에서만 작업한다.

Definition of Done
- <functional_done_condition>
- <test_or_doc_done_condition>
- `make premerge`를 통과한다.
- PR 본문은 한글로 작성하고 `Summary`, `User impact`, `Behavior`, `Safety`, `Tests`, `E2E guide`, `E2E Done` 섹션을 포함한다.

Task
- branch: <branch_name>
- 대상 파일/모듈: <files_or_modules>
- 기능 계약: <success/failure/stdout-stderr/exit-code>
- 테스트: <tests_to_add_or_run>
- 문서: <docs_to_update>

## PR body requirements
- Summary, User impact, Behavior, Safety, Tests, E2E guide, E2E Done 섹션을 모두 포함한다.
- 사용자-facing 변경이면 E2E guide를 생략하지 않는다.
- 사용자-facing 변경이면 E2E Done에서 실행 결과를 체크리스트로 남긴다.
- 예시에는 민감정보/로컬 절대경로를 넣지 않는다.
```

## Use the extended version only when needed

아래 항목은 항상 필요한 것은 아니다. 작업이 크거나 위험할 때만 추가한다.

- `Context`: 이전 PR, 현재 한계, 이번 범위
- `Non-goals`: 범위 이탈 방지용
- `Validation`: `make premerge` 외 추가 검증
- `E2E Execution Requirements`: user-facing 변경에서만 상세화
- `Docs Synchronization`: 문서 반영 범위가 넓을 때만 상세화

## Prompt checklist

프롬프트를 넘기기 전에 아래를 확인한다.

- branch/worktree 이름이 명시되어 있는가
- skill 사용 여부가 명시되어 있는가
- 변경해야 할 문서와 코드 범위가 적혀 있는가
- 기능 계약이 `stdout`/`stderr`/exit code까지 필요하면 적혀 있는가
- user-facing 변경이면 E2E 기록 방식이 적혀 있는가
- PR 본문 섹션 요구사항이 포함되어 있는가

## Example

아래는 변수 치환 예시다. 실제 작업마다 값만 바꿔 재사용한다.

```md
# [Feature]: wt list filter options

@docs/prompts/feature-pr-template.md
@docs/pr-guidelines.md
@AGENTS.md

Goal
- `wt list`에서 stale/safe-to-remove/recommended-action 기준 필터링을 지원한다.
- 기존 list 출력과 검증 흐름을 깨지 않고 필요한 후보만 바로 볼 수 있게 한다.

Constraints
- `wt-worktree` 스킬을 사용한다.
- 시작 전에 `wt --version`, `wt list`를 실행한다.
- `wt path --create feat/wt-list-filters`로 분리 worktree를 만든 뒤 그 worktree에서만 작업한다.
- 기존 `--json`, `--porcelain`, `--verify`, `--verify-hosting` 계약을 불필요하게 바꾸지 않는다.

Definition of Done
- 필터 옵션과 조합 규칙이 구현된다.
- 텍스트 출력과 JSON 출력에서 필터 결과가 일관된다.
- 관련 스펙/정책/UX 문서가 동기화된다.
- `make premerge`를 통과한다.

Task
- branch: `feat/wt-list-filters`
- 대상 파일/모듈: `cmd/wt/list.go`, `cmd/wt/list_test.go`
- 기능 계약: `--stale`, `--safe-to-remove`, `--recommended <none|prune|remove>` 필터를 정의하고 조합 규칙을 명확히 한다.
- 테스트: 옵션 조합, JSON/text 일관성, 기존 verify/porcelain 회귀를 확인한다.
- 문서: `docs/spec/cli.md`, 필요 시 `docs/ux/*`, `docs/policy/*`, roadmap/backlog를 갱신한다.

## PR body requirements
- Summary, User impact, Behavior, Safety, Tests, E2E guide, E2E Done 섹션을 모두 포함한다.
- 사용자-facing 변경이면 E2E guide를 생략하지 않는다.
- 사용자-facing 변경이면 E2E Done에서 실행 결과를 체크리스트로 남긴다.
- 예시에는 민감정보/로컬 절대경로를 넣지 않는다.
```
