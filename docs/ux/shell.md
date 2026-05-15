# Shell integration and completion

이 문서는 `wt init`과 `wt completion`을 실제 사용 순서대로 정리한다.
원칙은 그대로다. helper는 output-only이고, completion 설치도 사용자가 직접 opt-in 한다.

## Recommended flow

1. `wt`를 설치한다.
2. `wt init <shell>`로 helper를 지금 적용하거나 rc에 직접 추가한다.
3. `wtg`/`wcd` 탭 완성이 필요할 때만 `wt completion <shell>`을 설치한다.

빠른 시작:

```sh
eval "$(wt init zsh)"
```

PowerShell (Windows Terminal alias 충돌을 피하기 위해 첫 호출은 절대경로):

```powershell
& "$env:USERPROFILE\go\bin\wt.exe" init powershell | Out-String | Invoke-Expression
```

## `wt init <shell>`

`wt init`은 rc 파일을 직접 수정하지 않는다.
출력은 아래 두 부분만 가진다.

- 상단 주석: 즉시 적용, 영구 적용, 자세한 completion 문서 위치
- helper 본문: 셸 함수 정의

helper 범위:

- `wtr()`: `cd "$(wt root)" || return`
- `wtg()`: `cd "$(wt path "$@")" || return`
- `wcd()`: `cd "$(wt path "$@")" || return`

- `wtr`: 현재 Git 컨텍스트의 primary repo root로 이동
- `wtg`: query로 worktree를 찾아 이동
- `wcd`: `wtg`와 동일 동작의 별칭 helper

PowerShell의 경우 Windows Terminal의 `wt` App Execution Alias와 충돌하므로, init 출력은 `where.exe`로 진짜 `wt.exe`를 찾아 `Set-Alias wtp`로 노출한다. 즉 Windows 사용자는 `wt list` 대신 `wtp list`를 쓴다. `wtr`/`wtg`/`wcd`는 동일하다.

자동으로 하지 않는 것:

- rc 파일 수정
- completion 파일 설치/로드
- 인증이나 upgrade 실행

## Apply helpers

zsh:

```sh
eval "$(wt init zsh)"
```

bash:

```sh
eval "$(wt init bash)"
```

fish:

```sh
wt init fish | source
```

PowerShell (Windows Terminal alias 충돌을 피하려고 첫 호출은 절대경로):

```powershell
& "$env:USERPROFILE\go\bin\wt.exe" init powershell | Out-String | Invoke-Expression
```

영구 적용은 각 셸 rc에 같은 출력을 append 하면 된다. PowerShell은 `$PROFILE`에 append한다(첫 회는 절대경로, 이후엔 `wtp`):

```powershell
& "$env:USERPROFILE\go\bin\wt.exe" init powershell >> $PROFILE   # 첫 회
wtp init powershell >> $PROFILE                                   # 이후 갱신
```

스니펫은 `$wtBinFallback`에 init을 실행한 `wt.exe` 절대경로를 embed 한다. `$env:USERPROFILE\go\bin`이 PATH에 없어 `where.exe`가 진짜 `wt.exe`를 못 찾는 경우 이 fallback이 사용된다.

## Install completion

`wt`는 Cobra 기본 `wt completion <shell>` 명령을 제공한다.
`wt init <shell>`은 completion 상세 단계를 반복하지 않고 이 문서로만 연결한다.

### zsh

```sh
mkdir -p ~/.zsh/completions
wt completion zsh > ~/.zsh/completions/_wt
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit && compinit
```

`wt init zsh`에는 completion bridge가 포함된다. `_wt`가 로드되어 있으면 `wtg <TAB>`와 `wcd <TAB>`가 `wt path <TAB>`처럼 동작한다.

### bash

```sh
mkdir -p ~/.bash_completion.d
wt completion bash > ~/.bash_completion.d/wt
source ~/.bash_completion.d/wt
```

### fish

```sh
mkdir -p ~/.config/fish/completions
wt completion fish > ~/.config/fish/completions/wt.fish
```

### PowerShell

```powershell
wt completion powershell | Out-String | Invoke-Expression
```

영구 적용은 `$PROFILE`에 append한다(예: `wt completion powershell >> $PROFILE`). Cobra 기본 completion은 `wtg`/`wcd` 별칭에는 자동 연결되지 않으므로, 탭 완성은 `wt path <TAB>`을 직접 사용한다.

## Completion scope

기본 후보는 현재 등록된 worktree다.

- linked worktree가 있는 브랜치명은 completion 후보에 포함된다.
- detached entry는 basename 기반으로 후보가 잡힐 수 있다.

원격 브랜치 후보를 추가하고 싶으면:

```sh
export WT_PATH_COMPLETE_REMOTE=1
```

이 경우에도 자동 `git fetch`는 하지 않는다. 로컬에 이미 있는 `refs/remotes/origin/*`만 사용한다.

## Troubleshooting

`wtg`나 `wcd`에서 탭이 파일명 완성으로만 동작하면 `_wt`가 로드되지 않은 상태일 가능성이 높다.

진단:

```sh
whence -v _wt || true
```

`_wt not found`면 completion을 먼저 설치해야 한다.

추가로 `wt doctor`를 실행하면 shell 감지, rc marker, completion 파일 존재 여부를 한 번에 점검할 수 있다.
completion은 셸별 예상 경로를 순서대로 검사한다(예: zsh는 `~/.zsh/completions/_wt`, `~/.zfunc/_wt`; bash는 `~/.bash_completion.d/wt`, `~/.local/share/bash-completion/completions/wt`).
