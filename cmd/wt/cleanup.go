package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/hosting"
	tuipicker "github.com/crevissepartners/wt/internal/tui/picker"
	"github.com/crevissepartners/wt/internal/worktree"
)

type cleanupItem struct {
	Path              string `json:"path"`
	Branch            string `json:"branch,omitempty"`
	RecommendedAction string `json:"recommendedAction"`
	Action            string `json:"action"`
	Applied           bool   `json:"applied"`
	Removed           bool   `json:"removed"`
	Reason            string `json:"reason"`
	SafeToRemove      bool   `json:"safeToRemove"`

	PathExists          *bool  `json:"pathExists,omitempty"`
	DotGitExists        *bool  `json:"dotGitExists,omitempty"`
	Valid               *bool  `json:"valid,omitempty"`
	MergedIntoBase      *bool  `json:"mergedIntoBase,omitempty"`
	BaseRef             string `json:"baseRef,omitempty"`
	HostingProvider     string `json:"hostingProvider,omitempty"`
	HostingKind         string `json:"hostingKind,omitempty"`
	MergedViaHosting    *bool  `json:"mergedViaHosting,omitempty"`
	HostingReason       string `json:"hostingReason,omitempty"`
	HostingChangeNumber *int   `json:"hostingChangeNumber,omitempty"`
	HostingChangeTitle  string `json:"hostingChangeTitle,omitempty"`
	HostingChangeURL    string `json:"hostingChangeUrl,omitempty"`
}

type cleanupCandidate struct {
	Worktree worktree.Worktree
	Info     *verifyInfo
	Signals  listSignals
	Reason   string
}

func newCleanupCmd() *cobra.Command {
	var apply bool
	var jsonOut bool
	var tui bool

	cmd := &cobra.Command{
		Use:           "cleanup",
		Short:         "Preview or apply recommended prune/remove actions",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if tui && jsonOut {
				return usageError(fmt.Errorf("wt cleanup: --tui cannot be combined with --json"))
			}

			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			if tui && (d.CanUseTUI == nil || !d.CanUseTUI()) {
				return usageError(fmt.Errorf("wt cleanup: --tui requires a TTY on stdin and stderr"))
			}

			ctx := cmd.Context()
			repoRoot, err := git.RepoRoot(ctx, d.Runner, d.Cwd)
			if err != nil {
				return err
			}

			candidates, hostingNote, err := collectCleanupCandidates(ctx, d, repoRoot)
			if err != nil {
				return err
			}
			if hostingNote != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), hostingNote)
			}

			selectedCandidates := candidates
			if tui {
				selectedCandidates = cleanupReviewCandidates(candidates)
				if len(selectedCandidates) > 0 {
					selectedCandidates, err = reviewCleanupCandidatesWithTUI(cmd, d, selectedCandidates, apply)
					if err != nil {
						return err
					}
				}
			}

			if tui && apply && len(selectedCandidates) > 0 {
				confirm := confirmCleanup
				if d.ConfirmCleanup != nil {
					confirm = d.ConfirmCleanup
				}
				confirmed, err := confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), len(selectedCandidates))
				if err != nil {
					return err
				}
				if !confirmed {
					return &exitError{Code: 1, Err: fmt.Errorf("wt cleanup: aborted")}
				}
			}

			items, err := runCleanup(cmd, d, repoRoot, selectedCandidates, apply)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(items)
			}

			for _, item := range items {
				fmt.Fprintln(cmd.OutOrStdout(), formatCleanupLine(item))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&apply, "apply", false, "actually run recommended prune/remove actions")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&tui, "tui", false, "review and select recommended candidates in TUI before returning or applying")
	return cmd
}

func cleanupReviewCandidates(candidates []cleanupCandidate) []cleanupCandidate {
	out := make([]cleanupCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate.Signals.RecommendedAction {
		case "prune":
			out = append(out, candidate)
		case "remove":
			if candidate.Signals.SafeToRemove {
				out = append(out, candidate)
			}
		}
	}
	return out
}

func collectCleanupCandidates(ctx context.Context, d *deps, repoRoot string) ([]cleanupCandidate, string, error) {
	wts, err := git.WorktreeList(ctx, d.Runner, repoRoot)
	if err != nil {
		return nil, "", err
	}

	baseRef := git.DefaultBaseRef(ctx, d.Runner, repoRoot)
	exists, err := git.RefExists(ctx, d.Runner, repoRoot, baseRef)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		return nil, "", fmt.Errorf("wt cleanup: base ref does not exist: %s", baseRef)
	}

	verifyCtx := &listVerifyContext{
		RepoRoot:    repoRoot,
		BaseRef:     baseRef,
		VerifyLocal: true,
	}
	if remoteURL, err := git.RemoteURL(ctx, d.Runner, repoRoot, "origin"); err != nil {
		return nil, "", err
	} else if strings.TrimSpace(remoteURL) != "" {
		verifyCtx.RemoteURL = remoteURL
		verifyCtx.VerifyHosting = true
		verifyCtx.HostingProvider = hosting.DetectProvider(remoteURL)
		verifyCtx.HostingCache = newHostingVerifyCache()
	}

	paths := resolveListPaths(ctx, d, repoRoot)
	out := make([]cleanupCandidate, 0, len(wts))
	hostingNote := ""
	for _, wt := range wts {
		info, err := verifyWorktreeWithContext(ctx, d, verifyCtx, wt)
		if err != nil {
			return nil, "", err
		}
		signals := deriveListSignals(wt, info, paths)
		if hostingNote == "" && info != nil {
			switch info.HostingReason {
			case "gh-auth-unavailable":
				hostingNote = "note: hosting verify skipped (gh not found on PATH / WT_GH_BIN, or not authenticated)"
			case "unsupported-provider":
				if info.HostingProvider != "" && info.HostingProvider != string(hosting.ProviderUnknown) {
					hostingNote = fmt.Sprintf("note: hosting verify skipped (provider not implemented: %s)", info.HostingProvider)
				}
			}
		}
		out = append(out, cleanupCandidate{
			Worktree: wt,
			Info:     info,
			Signals:  signals,
			Reason:   cleanupReason(wt, info, signals),
		})
	}

	return out, hostingNote, nil
}

func runCleanup(cmd *cobra.Command, d *deps, repoRoot string, candidates []cleanupCandidate, apply bool) ([]cleanupItem, error) {
	items := make([]cleanupItem, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, cleanupItemForCandidate(candidate, false))
	}
	if !apply {
		return items, nil
	}

	hasPrune := false
	for _, candidate := range candidates {
		if candidate.Signals.RecommendedAction == "prune" {
			hasPrune = true
			break
		}
	}

	remainingAfterPrune := map[string]struct{}{}
	if hasPrune {
		if err := git.WorktreePrune(cmd.Context(), d.Runner, repoRoot); err != nil {
			return nil, err
		}
		afterPrune, err := git.WorktreeList(cmd.Context(), d.Runner, repoRoot)
		if err != nil {
			return nil, err
		}
		for _, wt := range afterPrune {
			remainingAfterPrune[wt.Path] = struct{}{}
		}
	}

	for i, candidate := range candidates {
		switch candidate.Signals.RecommendedAction {
		case "prune":
			items[i].Applied = true
			if _, ok := remainingAfterPrune[candidate.Worktree.Path]; ok {
				items[i].Action = actionKept
			} else {
				items[i].Action = actionPruned
				items[i].Removed = true
			}
		case "remove":
			if !candidate.Signals.SafeToRemove {
				continue
			}
			if err := git.WorktreeRemove(cmd.Context(), d.Runner, repoRoot, candidate.Worktree.Path, true); err != nil {
				return nil, err
			}
			items[i].Action = actionRemoved
			items[i].Applied = true
			items[i].Removed = true
		}
	}

	return items, nil
}

func reviewCleanupCandidatesWithTUI(cmd *cobra.Command, d *deps, candidates []cleanupCandidate, apply bool) ([]cleanupCandidate, error) {
	if d == nil || d.CanUseTUI == nil || !d.CanUseTUI() {
		return nil, usageError(fmt.Errorf("wt cleanup: --tui requires a TTY on stdin and stderr"))
	}

	review := reviewCleanupWithTUI
	if d.ReviewCleanup != nil {
		review = d.ReviewCleanup
	}

	selected, err := review(cmd, candidates, apply)
	if err == nil {
		return selected, nil
	}
	if errors.Is(err, tuipicker.ErrCancelled) {
		return nil, &exitError{Code: 130, Err: fmt.Errorf("wt cleanup: review cancelled")}
	}
	if errors.Is(err, tuipicker.ErrNonTTY) {
		return nil, usageError(fmt.Errorf("wt cleanup: --tui requires a TTY on stdin and stderr"))
	}
	return nil, err
}

const cleanupReviewDoneID = "__wt_cleanup_done__"

func reviewCleanupWithTUI(cmd *cobra.Command, candidates []cleanupCandidate, apply bool) ([]cleanupCandidate, error) {
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return nil, tuipicker.ErrNonTTY
	}
	screen, ok := cmd.ErrOrStderr().(*os.File)
	if !ok {
		return nil, tuipicker.ErrNonTTY
	}

	selected := map[string]struct{}{}
	for {
		chosen, err := tuipicker.Run(in, screen, tuipicker.Config{
			Title: "Select cleanup candidates",
			Help:  cleanupReviewHelp(apply),
			Items: buildCleanupReviewPickerItems(candidates, selected, apply),
		})
		if err != nil {
			return nil, err
		}
		if chosen.ID == cleanupReviewDoneID {
			break
		}
		if _, ok := selected[chosen.ID]; ok {
			delete(selected, chosen.ID)
		} else {
			selected[chosen.ID] = struct{}{}
		}
	}

	out := make([]cleanupCandidate, 0, len(selected))
	for _, candidate := range candidates {
		if _, ok := selected[candidate.Worktree.Path]; !ok {
			continue
		}
		out = append(out, candidate)
	}
	return out, nil
}

func cleanupReviewHelp(apply bool) string {
	if apply {
		return "Enter toggle candidate, select continue row to confirm apply, Esc cancel"
	}
	return "Enter toggle candidate, select continue row to print preview, Esc cancel"
}

func buildCleanupReviewPickerItems(candidates []cleanupCandidate, selected map[string]struct{}, apply bool) []tuipicker.Item {
	items := make([]tuipicker.Item, 0, len(candidates)+1)

	continueLabel := "Continue: preview selected candidates"
	if apply {
		continueLabel = "Continue: apply selected candidates"
	}
	items = append(items, tuipicker.Item{
		ID:         cleanupReviewDoneID,
		Label:      continueLabel,
		Meta:       fmt.Sprintf("selected %d/%d", len(selected), len(candidates)),
		Detail:     "Press Enter to continue",
		FilterText: "continue done preview apply",
	})

	for _, candidate := range candidates {
		branch := strings.TrimPrefix(candidate.Worktree.Branch, "refs/heads/")
		if branch == "" {
			branch = filepath.Base(candidate.Worktree.Path)
		}

		action := candidate.Signals.RecommendedAction
		if action == "" || action == "none" {
			action = actionSkip
		}

		prefix := "[ ]"
		if _, ok := selected[candidate.Worktree.Path]; ok {
			prefix = "[x]"
		}

		metaParts := []string{"action:" + action}
		if candidate.Reason != "" {
			metaParts = append(metaParts, candidate.Reason)
		}

		items = append(items, tuipicker.Item{
			ID:         candidate.Worktree.Path,
			Label:      prefix + " " + branch,
			Detail:     candidate.Worktree.Path,
			Meta:       strings.Join(metaParts, " | "),
			FilterText: strings.Join([]string{branch, candidate.Worktree.Path, candidate.Worktree.Branch, candidate.Reason, action}, " "),
		})
	}

	return items
}

func confirmCleanup(in io.Reader, out io.Writer, count int) (bool, error) {
	prompt := fmt.Sprintf("Apply cleanup to %d selected candidate", count)
	if count != 1 {
		prompt += "s"
	}
	prompt += "? [y/N] "

	if _, err := fmt.Fprint(out, prompt); err != nil {
		return false, err
	}

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("wt cleanup: failed to read confirmation: %w", err)
	}

	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func cleanupItemForCandidate(candidate cleanupCandidate, applied bool) cleanupItem {
	item := cleanupItem{
		Path:              candidate.Worktree.Path,
		Branch:            strings.TrimPrefix(candidate.Worktree.Branch, "refs/heads/"),
		RecommendedAction: candidate.Signals.RecommendedAction,
		Action:            cleanupPreviewAction(candidate.Signals.RecommendedAction),
		Applied:           applied,
		Removed:           false,
		Reason:            candidate.Reason,
		SafeToRemove:      candidate.Signals.SafeToRemove,
	}
	if candidate.Info != nil {
		item.PathExists = &candidate.Info.PathExists
		item.DotGitExists = &candidate.Info.DotGitExists
		item.Valid = &candidate.Info.Valid
		item.MergedIntoBase = candidate.Info.MergedIntoBase
		item.BaseRef = candidate.Info.BaseRef
		item.HostingProvider = candidate.Info.HostingProvider
		item.HostingKind = candidate.Info.HostingKind
		item.MergedViaHosting = candidate.Info.MergedViaHosting
		item.HostingReason = candidate.Info.HostingReason
		item.HostingChangeNumber = candidate.Info.HostingNumber
		item.HostingChangeTitle = candidate.Info.HostingTitle
		item.HostingChangeURL = candidate.Info.HostingURL
	}
	return item
}

func cleanupPreviewAction(recommendedAction string) string {
	switch recommendedAction {
	case "prune":
		return actionWouldPrune
	case "remove":
		return actionWouldRemove
	default:
		return actionSkip
	}
}

func formatCleanupLine(item cleanupItem) string {
	line := fmt.Sprintf("%s  %s", item.Action, item.Path)
	if item.Branch != "" {
		line += fmt.Sprintf("  (%s)", item.Branch)
	}
	if item.Reason != "" {
		line += fmt.Sprintf("  [%s]", item.Reason)
	}
	return line
}

func cleanupReason(wt worktree.Worktree, info *verifyInfo, signals listSignals) string {
	switch signals.RecommendedAction {
	case "prune":
		if wt.PruneReason != "" {
			return wt.PruneReason
		}
		return "prunable"
	case "remove":
		reasons := make([]string, 0, 2)
		if info != nil && info.MergedIntoBase != nil && *info.MergedIntoBase {
			reasons = append(reasons, "merged:"+cleanupShortRef(info.BaseRef))
		}
		if info != nil && info.MergedViaHosting != nil && *info.MergedViaHosting {
			reason := "merged-hosting"
			if info.HostingProvider != "" {
				reason += ":" + info.HostingProvider
			}
			if info.HostingNumber != nil {
				reason += fmt.Sprintf("#%d", *info.HostingNumber)
			}
			reasons = append(reasons, reason)
		}
		if len(reasons) == 0 {
			return "safe-remove"
		}
		return strings.Join(reasons, ", ")
	default:
		return cleanupSkipReason(wt, info, signals)
	}
}

func cleanupSkipReason(wt worktree.Worktree, info *verifyInfo, signals listSignals) string {
	switch {
	case signals.Current:
		return "current"
	case signals.Primary:
		return "primary"
	case wt.Detached:
		return "detached"
	case wt.Locked:
		return "locked"
	}

	pathExists, dotGitExists := worktreePathStatus(wt.Path)
	if info != nil {
		pathExists = info.PathExists
		dotGitExists = info.DotGitExists
	}

	switch {
	case !pathExists:
		return "missing-path"
	case !dotGitExists:
		return "missing-git"
	case wt.Prunable:
		return "prunable"
	default:
		return "not-recommended"
	}
}

func cleanupShortRef(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	ref = strings.TrimPrefix(ref, "origin/")
	ref = strings.TrimPrefix(ref, "upstream/")
	return ref
}
