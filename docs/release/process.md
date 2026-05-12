# Release process

`wt` 릴리즈는 release-please가 생성하는 `vX.Y.Z` 태그와 GitHub Release를 기준으로 배포한다.

## Rules

- 기능/수정 PR은 버전 파일이나 `CHANGELOG.md`를 직접 수정하지 않는다.
- main에 squash merge되는 PR title은 release-please가 읽는 Conventional Commit subject다.
- 사용자-facing 기능은 `feat: ...`, 버그 수정은 `fix: ...`를 사용한다.
- breaking change는 Conventional Commit 규칙에 따라 `!` 또는 `BREAKING CHANGE:` footer를 사용한다.
- release-please PR만 `.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`, `CHANGELOG.md`를 갱신한다.
- 릴리즈 태그는 `vX.Y.Z` 형식을 사용하고, `internal/buildinfo.Version`과 일치해야 한다.
- 릴리즈 설치 경로는 `go install github.com/crevissepartners/wt/cmd/wt@latest`를 기준으로 유지한다.

## Release steps

1. 기능/수정 PR을 Conventional Commit title로 main에 squash merge한다.
2. main push마다 `release-please` 워크플로가 변경 subject를 누적한다.
3. release-please가 `chore(main): release X.Y.Z` PR을 생성하거나 갱신한다.
4. release PR에는 `.release-please-manifest.json`, `internal/buildinfo/buildinfo.go`, `CHANGELOG.md` 변경이 포함된다.
5. release PR을 merge하면 release-please가 `vX.Y.Z` 태그와 GitHub Release를 생성한다.
6. tag push 이벤트에서 CI가 tag 형식과 `internal/buildinfo.Version` 일치를 검증한다.

## Manual release exception

수동 태깅은 기본 전략이 아니다. release-please 장애 등 예외 상황에서만 사용하고, PR/이슈에 사유와 실행 로그를 남긴다.

```sh
git switch main
git pull --ff-only
version="$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' internal/buildinfo/buildinfo.go | head -n 1)"
git tag -a "v${version}" -m "release: v${version}"
git push origin "v${version}"
```

## Verification

- `go list -m github.com/crevissepartners/wt@latest`가 방금 배포한 태그를 가리키는지 확인한다.
- `go install github.com/crevissepartners/wt/cmd/wt@latest` 후 `wt --version` 출력이 태그 버전과 일치하는지 확인한다.
- GitHub Actions `ci`는 PR에서 `make premerge`를 실행한다.
- GitHub Actions `ci`는 tag push 시 `v<semver>` 형식과 `internal/buildinfo.Version` 일치 여부를 검증한다.
- GitHub Release 본문과 `CHANGELOG.md`는 release-please가 생성한 내용을 기준으로 한다.

## Release-please failure response

- release PR이 열리지 않으면 PR title이 Conventional Commit 형식인지 먼저 확인한다.
- `RELEASE_PLEASE_TOKEN` 권한 문제면 repository secret과 `contents: write`, `pull-requests: write` 권한을 확인한다.
- release PR에 버전 파일이 누락되면 `release-please-config.json`의 `extra-files`와 `x-release-please-version` marker를 확인한다.
- 수동 tag push로 우회하지 말고, 가능하면 release-please 설정을 고친 PR을 먼저 만든다.
