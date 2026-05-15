package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestInit_Zsh(t *testing.T) {
	t.Parallel()

	cmd := newInitCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"zsh"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "wtr() { cd \"$(wt root)\" || return; }") {
		t.Fatalf("stdout = %q, want wtr function", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Apply now: eval \"$(wt init zsh)\"") {
		t.Fatalf("stdout = %q, want zsh apply-now guide", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Optional completion setup: see docs/ux/shell.md") {
		t.Fatalf("stdout = %q, want zsh completion docs guide", stdout.String())
	}
	if !strings.Contains(stdout.String(), "wtg() { cd \"$(wt path") {
		t.Fatalf("stdout = %q, want wtg function", stdout.String())
	}
	if !strings.Contains(stdout.String(), "wcd() { cd \"$(wt path") {
		t.Fatalf("stdout = %q, want wcd function", stdout.String())
	}
	if !strings.Contains(stdout.String(), "compdef _wtg wtg wcd") {
		t.Fatalf("stdout = %q, want completion bridge for wtg and wcd", stdout.String())
	}
}

func TestInit_Fish(t *testing.T) {
	t.Parallel()

	cmd := newInitCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"fish"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "function wtr") {
		t.Fatalf("stdout = %q, want fish wtr function", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Apply now: wt init fish | source") {
		t.Fatalf("stdout = %q, want fish apply-now guide", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Optional completion setup: see docs/ux/shell.md") {
		t.Fatalf("stdout = %q, want fish completion docs guide", stdout.String())
	}
	if !strings.Contains(stdout.String(), "function wtg") {
		t.Fatalf("stdout = %q, want fish function", stdout.String())
	}
	if !strings.Contains(stdout.String(), "function wcd") {
		t.Fatalf("stdout = %q, want fish wcd function", stdout.String())
	}
}

func TestInit_PowerShell(t *testing.T) {
	t.Parallel()

	cmd := newInitCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"powershell"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()

	wants := []string{
		`Bootstrap (first time) via absolute path:`,
		`& "$env:USERPROFILE\go\bin\wt.exe" init powershell | Out-String | Invoke-Expression`,
		`& "$env:USERPROFILE\go\bin\wt.exe" init powershell >> $PROFILE`,
		"wtp init powershell | Out-String | Invoke-Expression",
		"# Optional completion setup: see docs/ux/shell.md",
		"$wtBinFallback = '",
		`& where.exe wt 2>$null`,
		`-notlike '*\WindowsApps\*'`,
		"$Global:WtBin = $candidate",
		"if (-not $Global:WtBin -and $wtBinFallback -and (Test-Path -LiteralPath $wtBinFallback)) {",
		"$Global:WtBin = $wtBinFallback",
		"Set-Alias -Name wtp -Value $Global:WtBin -Scope Global",
		"function global:wtr {",
		"function global:wtg {",
		"Set-Alias wcd wtg -Scope Global",
		"Set-Location -LiteralPath $root.Trim()",
		"& $Global:WtBin root",
		"& $Global:WtBin path @Arguments",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestInit_Bash(t *testing.T) {
	t.Parallel()

	cmd := newInitCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"bash"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), `wtr() { cd "$(wt root)" || return; }`) {
		t.Fatalf("stdout = %q, want bash wtr function", stdout.String())
	}
	if !strings.Contains(stdout.String(), `# Apply now: eval "$(wt init bash)"`) {
		t.Fatalf("stdout = %q, want bash apply-now guide", stdout.String())
	}
	if !strings.Contains(stdout.String(), "# Optional completion setup: see docs/ux/shell.md") {
		t.Fatalf("stdout = %q, want bash completion docs guide", stdout.String())
	}
	if !strings.Contains(stdout.String(), `wtg() { cd "$(wt path "$@")" || return; }`) {
		t.Fatalf("stdout = %q, want bash wtg function", stdout.String())
	}
	if !strings.Contains(stdout.String(), `wcd() { cd "$(wt path "$@")" || return; }`) {
		t.Fatalf("stdout = %q, want bash wcd function", stdout.String())
	}
}
