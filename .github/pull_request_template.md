## Summary
- 무엇을/왜 변경했는지 3~5줄

## User impact
- 사용자에게 보이는 변화가 있으면 bullet로 명시 (없으면 `None`)
- stdout/stderr 규칙, exit code 변화가 있으면 반드시 명시

## Behavior (examples)
- 실행 예 1~2개 (특히 새 플래그/출력)

## Safety
- 파괴적 동작 여부, 기본값의 안전성, opt-in/confirm/--dry-run 정책

## Tests
- `make premerge`

## E2E guide
아래 중 하나를 **반드시** 포함합니다.
- 재현 커맨드 (권장)
- 또는 `N/A` + 이유 (예: 문서 변경만 포함)

예시:
```sh
make run ARGS="--help"
make run ARGS="list --verify --base origin/main"
```

## Docs
- 스펙/정책/UX/릴리즈 절차 등 변경한 문서 링크

## Release
- PR title은 Conventional Commit 형식인가? (`feat: ...`, `fix: ...`, `chore: ...`)
- 기능/수정 PR에서 `CHANGELOG.md`와 release-please version 파일을 직접 수정하지 않았는가?
