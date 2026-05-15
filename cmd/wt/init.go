package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "init <shell>",
		Short:         "Print shell integration snippet",
		Args:          cobra.ExactArgs(1),
		ValidArgs:     []string{"zsh", "bash", "fish", "powershell"},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := strings.ToLower(strings.TrimSpace(args[0]))
			switch shell {
			case "zsh":
				// Output-only: user must opt-in by evaluating or pasting this snippet.
				// Includes an optional completion bridge so `wtg <TAB>` and `wcd <TAB>` complete like `wt path <TAB>`.
				fmt.Fprintln(cmd.OutOrStdout(), "# wt shell integration for zsh (output-only; no rc changes are made)")
				fmt.Fprintln(cmd.OutOrStdout(), `# Apply now: eval "$(wt init zsh)"`)
				fmt.Fprintln(cmd.OutOrStdout(), `# Persist helpers: wt init zsh >> ~/.zshrc`)
				fmt.Fprintln(cmd.OutOrStdout(), "# Optional completion setup: see docs/ux/shell.md")
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), `wtr() { cd "$(wt root)" || return; }`)
				fmt.Fprintln(cmd.OutOrStdout(), `wtg() { cd "$(wt path "$@")" || return; }`)
				fmt.Fprintln(cmd.OutOrStdout(), `wcd() { cd "$(wt path "$@")" || return; }`)
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "# Optional: completion bridge (requires `wt` zsh completion to be installed)")
				fmt.Fprintln(cmd.OutOrStdout(), `if whence -w compdef >/dev/null 2>&1; then`)
				fmt.Fprintln(cmd.OutOrStdout(), `  if ! (( $+functions[_wt] )); then`)
				fmt.Fprintln(cmd.OutOrStdout(), `    whence -w _wt >/dev/null 2>&1 && autoload -Uz _wt`)
				fmt.Fprintln(cmd.OutOrStdout(), `  fi`)
				fmt.Fprintln(cmd.OutOrStdout(), `  if (( $+functions[_wt] )); then`)
				fmt.Fprintln(cmd.OutOrStdout(), `    _wtg() {`)
				fmt.Fprintln(cmd.OutOrStdout(), `      local -a wtg_words`)
				fmt.Fprintln(cmd.OutOrStdout(), `      wtg_words=("${words[@]}")`)
				fmt.Fprintln(cmd.OutOrStdout(), `      words=(wt path "${wtg_words[@]:1}")`)
				fmt.Fprintln(cmd.OutOrStdout(), `      (( CURRENT++ ))`)
				fmt.Fprintln(cmd.OutOrStdout(), `      _wt`)
				fmt.Fprintln(cmd.OutOrStdout(), `    }`)
				fmt.Fprintln(cmd.OutOrStdout(), `    compdef _wtg wtg wcd`)
				fmt.Fprintln(cmd.OutOrStdout(), `  fi`)
				fmt.Fprintln(cmd.OutOrStdout(), `fi`)
				return nil
			case "bash":
				fmt.Fprintln(cmd.OutOrStdout(), "# wt shell integration for bash (output-only; no rc changes are made)")
				fmt.Fprintln(cmd.OutOrStdout(), `# Apply now: eval "$(wt init bash)"`)
				fmt.Fprintln(cmd.OutOrStdout(), `# Persist helpers: wt init bash >> ~/.bashrc`)
				fmt.Fprintln(cmd.OutOrStdout(), "# Optional completion setup: see docs/ux/shell.md")
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), `wtr() { cd "$(wt root)" || return; }`)
				fmt.Fprintln(cmd.OutOrStdout(), `wtg() { cd "$(wt path "$@")" || return; }`)
				fmt.Fprintln(cmd.OutOrStdout(), `wcd() { cd "$(wt path "$@")" || return; }`)
				return nil
			case "powershell":
				exePath := ""
				if p, err := os.Executable(); err == nil {
					exePath = p
				}
				psExePath := strings.ReplaceAll(exePath, "'", "''")
				fmt.Fprintln(cmd.OutOrStdout(), "# wt shell integration for PowerShell (output-only; no profile changes are made)")
				fmt.Fprintln(cmd.OutOrStdout(), "# Note: Windows Terminal's 'wt' App Execution Alias may shadow this binary.")
				fmt.Fprintln(cmd.OutOrStdout(), "#       Bootstrap (first time) via absolute path:")
				fmt.Fprintln(cmd.OutOrStdout(), `#         & "$env:USERPROFILE\go\bin\wt.exe" init powershell | Out-String | Invoke-Expression`)
				fmt.Fprintln(cmd.OutOrStdout(), `#         & "$env:USERPROFILE\go\bin\wt.exe" init powershell >> $PROFILE`)
				fmt.Fprintln(cmd.OutOrStdout(), "#       After the snippet below has loaded, 'wtp' is safe to use for re-invocation:")
				fmt.Fprintln(cmd.OutOrStdout(), "#         wtp init powershell | Out-String | Invoke-Expression")
				fmt.Fprintln(cmd.OutOrStdout(), "#       The helpers resolve the real wt.exe via 'where.exe' (skipping")
				fmt.Fprintln(cmd.OutOrStdout(), "#       WindowsApps); if none is found there, they fall back to the path")
				fmt.Fprintln(cmd.OutOrStdout(), "#       that generated this snippet ($wtBinFallback below). 'wtp' is")
				fmt.Fprintln(cmd.OutOrStdout(), "#       exposed as the safe CLI alias. Use 'wtp list', 'wtp create', etc.")
				fmt.Fprintln(cmd.OutOrStdout(), "# Optional completion setup: see docs/ux/shell.md")
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintf(cmd.OutOrStdout(), "$wtBinFallback = '%s'\n", psExePath)
				fmt.Fprint(cmd.OutOrStdout(), `$Global:WtBin = $null
foreach ($candidate in (& where.exe wt 2>$null)) {
    if ($candidate -notlike '*\WindowsApps\*') { $Global:WtBin = $candidate; break }
}
if (-not $Global:WtBin -and $wtBinFallback -and (Test-Path -LiteralPath $wtBinFallback)) {
    $Global:WtBin = $wtBinFallback
}

if ($Global:WtBin) {
    Set-Alias -Name wtp -Value $Global:WtBin -Scope Global
} else {
    Write-Warning "wt.exe not found on PATH (Windows Terminal alias only?); install wt or adjust PATH."
}

function global:wtr {
    if (-not $Global:WtBin) { Write-Error "wt.exe not found on PATH"; return }
    $root = & $Global:WtBin root 2>$null
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($root)) {
        Write-Error "wt root failed"
        return
    }
    Set-Location -LiteralPath $root.Trim()
}

function global:wtg {
    [CmdletBinding()]
    param([Parameter(ValueFromRemainingArguments = $true)] [string[]] $Arguments)
    if (-not $Global:WtBin) { Write-Error "wt.exe not found on PATH"; return }
    $target = & $Global:WtBin path @Arguments 2>$null
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($target)) {
        Write-Error "wt path failed (no match for: $($Arguments -join ' '))"
        return
    }
    Set-Location -LiteralPath $target.Trim()
}

Set-Alias wcd wtg -Scope Global
`)
				return nil
			case "fish":
				fmt.Fprintln(cmd.OutOrStdout(), "# wt shell integration for fish (output-only; no config changes are made)")
				fmt.Fprintln(cmd.OutOrStdout(), "# Apply now: wt init fish | source")
				fmt.Fprintln(cmd.OutOrStdout(), "# Persist helpers: wt init fish >> ~/.config/fish/config.fish")
				fmt.Fprintln(cmd.OutOrStdout(), "# Optional completion setup: see docs/ux/shell.md")
				fmt.Fprintln(cmd.OutOrStdout(), "")
				fmt.Fprintln(cmd.OutOrStdout(), "function wtr")
				fmt.Fprintln(cmd.OutOrStdout(), "  cd (wt root); or return")
				fmt.Fprintln(cmd.OutOrStdout(), "end")
				fmt.Fprintln(cmd.OutOrStdout(), "function wtg")
				fmt.Fprintln(cmd.OutOrStdout(), "  cd (wt path $argv); or return")
				fmt.Fprintln(cmd.OutOrStdout(), "end")
				fmt.Fprintln(cmd.OutOrStdout(), "function wcd")
				fmt.Fprintln(cmd.OutOrStdout(), "  cd (wt path $argv); or return")
				fmt.Fprintln(cmd.OutOrStdout(), "end")
				return nil
			default:
				return usageError(fmt.Errorf("wt init: unsupported shell: %s", shell))
			}
		},
	}
	return cmd
}
