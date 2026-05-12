package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/hosting"
	"github.com/crevissepartners/wt/internal/worktree"
)

func newListCmd() *cobra.Command {
	var jsonOut bool
	var porcelain bool
	var verify bool
	var verifyHosting bool
	var baseRef string
	var staleFilter bool
	var safeToRemoveFilter bool
	var recommendedFilter string

	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List worktrees in current repo",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOut && porcelain {
				return usageError(fmt.Errorf("wt list: only one of --json or --porcelain can be specified"))
			}
			if porcelain && verifyHosting {
				return usageError(fmt.Errorf("wt list: --porcelain cannot be combined with --verify-hosting"))
			}

			filters, err := parseListFilters(staleFilter, safeToRemoveFilter, recommendedFilter)
			if err != nil {
				return usageError(err)
			}
			if porcelain && filters.any() {
				return usageError(fmt.Errorf("wt list: --porcelain cannot be combined with --stale, --safe-to-remove, or --recommended"))
			}

			d, err := getDeps(cmd)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			repoRoot, err := git.RepoRoot(ctx, d.Runner, d.Cwd)
			if err != nil {
				return err
			}

			if porcelain {
				out, err := git.WorktreeListPorcelain(ctx, d.Runner, repoRoot)
				if err != nil {
					return err
				}
				_, _ = cmd.OutOrStdout().Write(out)
				return nil
			}

			wts, err := git.WorktreeList(ctx, d.Runner, repoRoot)
			if err != nil {
				return err
			}
			paths := resolveListPaths(ctx, d, repoRoot)

			var verifyCtx *listVerifyContext
			if verify || verifyHosting {
				if baseRef == "" {
					baseRef = git.DefaultBaseRef(ctx, d.Runner, repoRoot)
				}
				exists, err := git.RefExists(ctx, d.Runner, repoRoot, baseRef)
				if err != nil {
					return err
				}
				if !exists {
					return usageError(fmt.Errorf("wt list: base ref does not exist: %s", baseRef))
				}
				verifyCtx = &listVerifyContext{RepoRoot: repoRoot, BaseRef: baseRef, VerifyLocal: verify}
				if verifyHosting {
					remoteURL, err := git.RemoteURL(ctx, d.Runner, repoRoot, "origin")
					if err != nil {
						return err
					}
					verifyCtx.RemoteURL = remoteURL
					verifyCtx.VerifyHosting = true
					verifyCtx.HostingProvider = hosting.DetectProvider(remoteURL)
					verifyCtx.HostingCache = newHostingVerifyCache()
				}
			}

			verifyInfos, err := verifyWorktrees(cmd, d, verifyCtx, wts)
			if err != nil {
				return err
			}

			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(toJSONWorktrees(wts, verifyCtx, verifyInfos, paths, filters))
			}

			hostingNote := formatHostingVerifyNote(verifyCtx, verifyInfos)
			if hostingNote != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), hostingNote)
			}

			for i, wt := range wts {
				info := verifyInfoAt(verifyInfos, i)
				signals := deriveListSignals(wt, info, paths)
				if !signalsMatchListFilters(signals, filters) {
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), formatWorktreeLine(wt, info, signals))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "git porcelain output (for parsing)")
	cmd.Flags().BoolVar(&verify, "verify", false, "verify worktree entries (checks path and merged-to-base)")
	cmd.Flags().BoolVar(&verifyHosting, "verify-hosting", false, "opt-in hosting merge verification (GitHub via gh, GitLab via glab)")
	cmd.Flags().StringVar(&baseRef, "base", "", "base ref for --verify (default: origin/HEAD or main)")
	cmd.Flags().BoolVar(&staleFilter, "stale", false, "show only stale worktrees")
	cmd.Flags().BoolVar(&safeToRemoveFilter, "safe-to-remove", false, "show only safe-to-remove worktrees")
	cmd.Flags().StringVar(&recommendedFilter, "recommended", "", "show only entries with recommended action (none|prune|remove)")
	return cmd
}

type jsonWorktree struct {
	Path   string `json:"path"`
	HEAD   string `json:"head"`
	Branch string `json:"branch"`

	Detached bool `json:"detached"`
	Locked   bool `json:"locked"`
	Prunable bool `json:"prunable"`
	Current  bool `json:"current"`
	Primary  bool `json:"primary"`
	Stale    bool `json:"stale"`

	RecommendedAction string `json:"recommendedAction"`
	SafeToRemove      bool   `json:"safeToRemove"`
	LockReason        string `json:"lockReason,omitempty"`
	PruneReason       string `json:"pruneReason,omitempty"`

	Verify *jsonVerifyFields `json:"-"`
}

type jsonVerifyFields struct {
	PathExists       bool
	DotGitExists     bool
	Valid            bool
	MergedIntoBase   *bool
	BaseRef          string
	HostingProvider  string
	HostingKind      string
	MergedViaHosting *bool
	HostingReason    string
	HostingNumber    *int
	HostingTitle     string
	HostingURL       string
}

type listVerifyContext struct {
	RepoRoot        string
	BaseRef         string
	RemoteURL       string
	VerifyLocal     bool
	VerifyHosting   bool
	HostingProvider hosting.Provider
	HostingCache    *hostingVerifyCache
}

type hostingVerifyCache struct {
	auth   map[hosting.Provider]hosting.VerifyResult
	merged map[hostingMergeCacheKey]hosting.VerifyResult
}

type hostingMergeCacheKey struct {
	Provider hosting.Provider
	Branch   string
	BaseRef  string
}

func newHostingVerifyCache() *hostingVerifyCache {
	return &hostingVerifyCache{
		auth:   map[hosting.Provider]hosting.VerifyResult{},
		merged: map[hostingMergeCacheKey]hosting.VerifyResult{},
	}
}

func (c *hostingVerifyCache) verifyMerged(ctx context.Context, d *deps, verifyCtx *listVerifyContext, branch string) (hosting.VerifyResult, error) {
	if c == nil {
		return hosting.VerifyMerged(ctx, d.Runner, verifyCtx.RepoRoot, verifyCtx.HostingProvider, branch, verifyCtx.BaseRef)
	}

	provider := verifyCtx.HostingProvider
	auth, ok := c.auth[provider]
	if !ok {
		var err error
		auth, err = hosting.VerifyAuth(ctx, d.Runner, verifyCtx.RepoRoot, provider)
		if err != nil {
			return hosting.VerifyResult{}, err
		}
		c.auth[provider] = auth
	}
	if auth.Reason != "" {
		return auth, nil
	}

	key := hostingMergeCacheKey{Provider: provider, Branch: branch, BaseRef: verifyCtx.BaseRef}
	if result, ok := c.merged[key]; ok {
		return result, nil
	}

	result, err := hosting.QueryMerged(ctx, d.Runner, verifyCtx.RepoRoot, provider, branch, verifyCtx.BaseRef)
	if err != nil {
		return hosting.VerifyResult{}, err
	}
	c.merged[key] = result
	return result, nil
}

type verifyInfo struct {
	LocalVerified    bool
	PathExists       bool
	DotGitExists     bool
	Valid            bool
	MergedIntoBase   *bool
	BaseRef          string
	HostingProvider  string
	HostingKind      string
	MergedViaHosting *bool
	HostingReason    string
	HostingNumber    *int
	HostingTitle     string
	HostingURL       string
}

type listSignals struct {
	Current           bool
	Primary           bool
	Stale             bool
	RecommendedAction string
	SafeToRemove      bool
}

type listPaths struct {
	CurrentWorktree string
	PrimaryWorktree string
}

type listFilters struct {
	StaleOnly        bool
	SafeToRemoveOnly bool
	Recommended      string
}

func (f listFilters) any() bool {
	return f.StaleOnly || f.SafeToRemoveOnly || f.Recommended != ""
}

func (jwt jsonWorktree) MarshalJSON() ([]byte, error) {
	type baseJSONWorktree struct {
		Path   string `json:"path"`
		HEAD   string `json:"head"`
		Branch string `json:"branch"`

		Detached bool `json:"detached"`
		Locked   bool `json:"locked"`
		Prunable bool `json:"prunable"`
		Current  bool `json:"current"`
		Primary  bool `json:"primary"`
		Stale    bool `json:"stale"`

		RecommendedAction string `json:"recommendedAction"`
		SafeToRemove      bool   `json:"safeToRemove"`
		LockReason        string `json:"lockReason,omitempty"`
		PruneReason       string `json:"pruneReason,omitempty"`

		PathExists             *bool  `json:"pathExists,omitempty"`
		DotGitExists           *bool  `json:"dotGitExists,omitempty"`
		Valid                  *bool  `json:"valid,omitempty"`
		MergedIntoBase         *bool  `json:"mergedIntoBase,omitempty"`
		BaseRef                string `json:"baseRef,omitempty"`
		HostingProvider        string `json:"hostingProvider,omitempty"`
		HostingKind            string `json:"hostingKind,omitempty"`
		MergedViaHosting       *bool  `json:"mergedViaHosting,omitempty"`
		HostingReason          string `json:"hostingReason,omitempty"`
		HostingChangeNumber    *int   `json:"hostingChangeNumber,omitempty"`
		HostingChangeTitle     string `json:"hostingChangeTitle,omitempty"`
		HostingChangeURL       string `json:"hostingChangeURL,omitempty"`
		HostingChangeURLLegacy string `json:"hostingChangeUrl,omitempty"`
	}

	out := baseJSONWorktree{
		Path:              jwt.Path,
		HEAD:              jwt.HEAD,
		Branch:            jwt.Branch,
		Detached:          jwt.Detached,
		Locked:            jwt.Locked,
		Prunable:          jwt.Prunable,
		Current:           jwt.Current,
		Primary:           jwt.Primary,
		Stale:             jwt.Stale,
		RecommendedAction: jwt.RecommendedAction,
		SafeToRemove:      jwt.SafeToRemove,
		LockReason:        jwt.LockReason,
		PruneReason:       jwt.PruneReason,
	}
	if jwt.Verify != nil {
		if jwt.Verify.MergedIntoBase != nil || jwt.Verify.BaseRef != "" {
			out.PathExists = &jwt.Verify.PathExists
			out.DotGitExists = &jwt.Verify.DotGitExists
			out.Valid = &jwt.Verify.Valid
			out.MergedIntoBase = jwt.Verify.MergedIntoBase
			out.BaseRef = jwt.Verify.BaseRef
		}
		out.HostingProvider = jwt.Verify.HostingProvider
		out.HostingKind = jwt.Verify.HostingKind
		out.MergedViaHosting = jwt.Verify.MergedViaHosting
		out.HostingReason = jwt.Verify.HostingReason
		out.HostingChangeNumber = jwt.Verify.HostingNumber
		out.HostingChangeTitle = jwt.Verify.HostingTitle
		out.HostingChangeURL = jwt.Verify.HostingURL
		out.HostingChangeURLLegacy = jwt.Verify.HostingURL
	}

	if jwt.Verify != nil && (jwt.Verify.MergedIntoBase == nil || jwt.Verify.HostingProvider != "" || jwt.Verify.BaseRef == "") {
		outMap := map[string]any{
			"path":              jwt.Path,
			"head":              jwt.HEAD,
			"branch":            jwt.Branch,
			"detached":          jwt.Detached,
			"locked":            jwt.Locked,
			"prunable":          jwt.Prunable,
			"current":           jwt.Current,
			"primary":           jwt.Primary,
			"stale":             jwt.Stale,
			"recommendedAction": jwt.RecommendedAction,
			"safeToRemove":      jwt.SafeToRemove,
		}
		if jwt.LockReason != "" {
			outMap["lockReason"] = jwt.LockReason
		}
		if jwt.PruneReason != "" {
			outMap["pruneReason"] = jwt.PruneReason
		}
		if jwt.Verify.MergedIntoBase != nil || jwt.Verify.BaseRef != "" {
			outMap["pathExists"] = jwt.Verify.PathExists
			outMap["dotGitExists"] = jwt.Verify.DotGitExists
			outMap["valid"] = jwt.Verify.Valid
			outMap["mergedIntoBase"] = jwt.Verify.MergedIntoBase
			outMap["baseRef"] = jwt.Verify.BaseRef
		}
		if jwt.Verify.HostingProvider != "" {
			outMap["hostingProvider"] = jwt.Verify.HostingProvider
			outMap["hostingKind"] = jwt.Verify.HostingKind
			outMap["mergedViaHosting"] = jwt.Verify.MergedViaHosting
			if jwt.Verify.HostingReason != "" {
				outMap["hostingReason"] = jwt.Verify.HostingReason
			}
			if jwt.Verify.HostingNumber != nil {
				outMap["hostingChangeNumber"] = jwt.Verify.HostingNumber
			}
			if jwt.Verify.HostingTitle != "" {
				outMap["hostingChangeTitle"] = jwt.Verify.HostingTitle
			}
			if jwt.Verify.HostingURL != "" {
				outMap["hostingChangeURL"] = jwt.Verify.HostingURL
				outMap["hostingChangeUrl"] = jwt.Verify.HostingURL
			}
		}
		return json.Marshal(outMap)
	}

	return json.Marshal(out)
}

func toJSONWorktrees(wts []worktree.Worktree, verifyCtx *listVerifyContext, verifyInfos []*verifyInfo, paths listPaths, filters listFilters) []jsonWorktree {
	out := make([]jsonWorktree, 0, len(wts))
	for i, wt := range wts {
		info := verifyInfoAt(verifyInfos, i)
		signals := deriveListSignals(wt, info, paths)
		if !signalsMatchListFilters(signals, filters) {
			continue
		}
		jwt := jsonWorktree{
			Path:              wt.Path,
			HEAD:              wt.HEAD,
			Branch:            wt.Branch,
			Detached:          wt.Detached,
			Locked:            wt.Locked,
			Prunable:          wt.Prunable,
			Current:           signals.Current,
			Primary:           signals.Primary,
			Stale:             signals.Stale,
			RecommendedAction: signals.RecommendedAction,
			SafeToRemove:      signals.SafeToRemove,
			LockReason:        wt.LockReason,
			PruneReason:       wt.PruneReason,
		}
		if verifyCtx != nil {
			if info != nil {
				jwt.Verify = &jsonVerifyFields{
					PathExists:       info.PathExists,
					DotGitExists:     info.DotGitExists,
					Valid:            info.Valid,
					MergedIntoBase:   info.MergedIntoBase,
					BaseRef:          info.BaseRef,
					HostingProvider:  info.HostingProvider,
					HostingKind:      info.HostingKind,
					MergedViaHosting: info.MergedViaHosting,
					HostingReason:    info.HostingReason,
					HostingNumber:    info.HostingNumber,
					HostingTitle:     info.HostingTitle,
					HostingURL:       info.HostingURL,
				}
			}
		}
		out = append(out, jwt)
	}
	return out
}

func verifyWorktrees(cmd *cobra.Command, d *deps, verifyCtx *listVerifyContext, wts []worktree.Worktree) ([]*verifyInfo, error) {
	if verifyCtx == nil {
		return nil, nil
	}

	out := make([]*verifyInfo, len(wts))
	for i, wt := range wts {
		info, err := verifyWorktree(cmd, d, verifyCtx, wt)
		if err != nil {
			return nil, err
		}
		out[i] = info
	}
	return out, nil
}

func verifyInfoAt(infos []*verifyInfo, i int) *verifyInfo {
	if i < 0 || i >= len(infos) {
		return nil
	}
	return infos[i]
}

func verifyWorktree(cmd *cobra.Command, d *deps, verifyCtx *listVerifyContext, wt worktree.Worktree) (*verifyInfo, error) {
	var ctx context.Context
	if cmd != nil {
		ctx = cmd.Context()
	}
	return verifyWorktreeWithContext(ctx, d, verifyCtx, wt)
}

func verifyWorktreeWithContext(ctx context.Context, d *deps, verifyCtx *listVerifyContext, wt worktree.Worktree) (*verifyInfo, error) {
	if verifyCtx == nil {
		return nil, nil
	}

	pathExists, dotGitExists := worktreePathStatus(wt.Path)

	valid := pathExists && dotGitExists && !wt.Prunable

	var merged *bool
	baseRef := ""
	if verifyCtx.VerifyLocal && d != nil && wt.Branch != "" && !wt.Detached {
		isMerged, err := git.IsAncestor(ctx, d.Runner, verifyCtx.RepoRoot, wt.Branch, verifyCtx.BaseRef)
		if err != nil {
			return nil, err
		}
		merged = &isMerged
		baseRef = verifyCtx.BaseRef
	} else if verifyCtx.VerifyLocal {
		baseRef = verifyCtx.BaseRef
	}

	var hostingMerged *bool
	hostingProvider := ""
	hostingKind := ""
	hostingReason := ""
	var hostingNumber *int
	hostingTitle := ""
	hostingURL := ""
	if verifyCtx.VerifyHosting {
		hostingProvider = string(verifyCtx.HostingProvider)
		if verifyCtx.HostingProvider == hosting.ProviderGitHub {
			hostingKind = "pr"
		} else if verifyCtx.HostingProvider == hosting.ProviderGitLab {
			hostingKind = "mr"
		}
		if wt.Branch == "" || wt.Detached {
			hostingReason = "no-branch"
		} else {
			result, err := verifyCtx.HostingCache.verifyMerged(ctx, d, verifyCtx, strings.TrimPrefix(wt.Branch, "refs/heads/"))
			if err != nil {
				return nil, err
			}
			hostingProvider = string(result.Provider)
			hostingKind = result.Kind
			hostingMerged = result.Merged
			hostingReason = result.Reason
			hostingNumber = result.Number
			hostingTitle = result.Title
			hostingURL = result.URL
		}
	}

	return &verifyInfo{
		LocalVerified:    verifyCtx.VerifyLocal,
		PathExists:       pathExists,
		DotGitExists:     dotGitExists,
		Valid:            verifyCtx.VerifyLocal && valid,
		MergedIntoBase:   merged,
		BaseRef:          baseRef,
		HostingProvider:  hostingProvider,
		HostingKind:      hostingKind,
		MergedViaHosting: hostingMerged,
		HostingReason:    hostingReason,
		HostingNumber:    hostingNumber,
		HostingTitle:     hostingTitle,
		HostingURL:       hostingURL,
	}, nil
}

func formatWorktreeLine(wt worktree.Worktree, info *verifyInfo, signals listSignals) string {
	head := wt.HEAD
	if len(head) > 7 {
		head = head[:7]
	}

	branch := displayBranch(wt)
	base := filepath.Base(wt.Path)

	flags := make([]string, 0, 2)
	if wt.Locked {
		flags = append(flags, "locked")
	}
	if wt.Prunable {
		flags = append(flags, "prunable")
	}
	if signals.Current {
		flags = append(flags, "current")
	}
	if signals.Primary {
		flags = append(flags, "primary")
	}

	if info != nil {
		if !info.PathExists {
			flags = append(flags, "missing-path")
		}
		if !info.DotGitExists {
			flags = append(flags, "missing-git")
		}
		if info.LocalVerified && info.MergedIntoBase != nil && *info.MergedIntoBase {
			flags = append(flags, "merged")
		}
		if info.MergedViaHosting != nil && *info.MergedViaHosting {
			flags = append(flags, fmt.Sprintf("merged-hosting:%s", info.HostingProvider))
		}
	} else {
		pathExists, dotGitExists := worktreePathStatus(wt.Path)
		if !pathExists {
			flags = append(flags, "missing-path")
		}
		if !dotGitExists {
			flags = append(flags, "missing-git")
		}
	}
	if signals.Stale {
		flags = append(flags, "stale")
	}
	if signals.SafeToRemove {
		flags = append(flags, "safe-remove")
	}
	if signals.RecommendedAction != "none" {
		flags = append(flags, "recommend:"+signals.RecommendedAction)
	}

	if len(flags) == 0 {
		return fmt.Sprintf("%s  %s  %s  %s", base, branch, head, wt.Path)
	}
	return fmt.Sprintf("%s  %s  %s  %s  [%s]", base, branch, head, wt.Path, strings.Join(flags, ","))
}

func formatHostingVerifyNote(verifyCtx *listVerifyContext, infos []*verifyInfo) string {
	if verifyCtx == nil || !verifyCtx.VerifyHosting {
		return ""
	}

	for _, info := range infos {
		if info == nil {
			continue
		}
		switch info.HostingReason {
		case "gh-auth-unavailable":
			return "note: hosting verify skipped (gh not found on PATH / WT_GH_BIN, or not authenticated)"
		case "glab-auth-unavailable":
			return "note: hosting verify skipped (glab not found on PATH / WT_GLAB_BIN, or not authenticated)"
		case "glab-mr-query-failed":
			return "note: hosting verify skipped (glab MR query failed)"
		case "gh-pr-query-failed":
			return "note: hosting verify skipped (gh PR query failed)"
		case "unsupported-provider":
			if info.HostingProvider != "" && info.HostingProvider != string(hosting.ProviderUnknown) {
				return fmt.Sprintf("note: hosting verify skipped (provider not implemented: %s)", info.HostingProvider)
			}
		}
	}

	return ""
}

func displayBranch(wt worktree.Worktree) string {
	if wt.Branch != "" {
		return strings.TrimPrefix(wt.Branch, "refs/heads/")
	}
	if wt.Detached {
		return "(detached)"
	}
	return "(unknown)"
}

func resolveListPaths(ctx context.Context, d *deps, repoRoot string) listPaths {
	if d == nil {
		return listPaths{}
	}

	paths := listPaths{CurrentWorktree: filepath.Clean(repoRoot)}
	if strings.TrimSpace(paths.CurrentWorktree) == "" {
		paths.CurrentWorktree = filepath.Clean(d.Cwd)
	}
	if primaryRoot, err := git.PrimaryWorktreeRoot(ctx, d.Runner, repoRoot); err == nil {
		paths.PrimaryWorktree = filepath.Clean(primaryRoot)
	}
	return paths
}

func deriveListSignals(wt worktree.Worktree, info *verifyInfo, paths listPaths) listSignals {
	pathExists, dotGitExists := worktreePathStatus(wt.Path)
	if info != nil {
		pathExists = info.PathExists
		dotGitExists = info.DotGitExists
	}

	current := samePath(wt.Path, paths.CurrentWorktree)
	primary := samePath(wt.Path, paths.PrimaryWorktree)
	stale := wt.Prunable || !pathExists || !dotGitExists

	mergedIntoBase := info != nil && info.MergedIntoBase != nil && *info.MergedIntoBase
	mergedViaHosting := info != nil && info.MergedViaHosting != nil && *info.MergedViaHosting
	safeToRemove := !wt.Prunable &&
		!wt.Detached &&
		!wt.Locked &&
		!current &&
		!primary &&
		pathExists &&
		dotGitExists &&
		(mergedIntoBase || mergedViaHosting)

	recommendedAction := "none"
	switch {
	case wt.Prunable:
		recommendedAction = "prune"
	case safeToRemove:
		recommendedAction = "remove"
	}

	return listSignals{
		Current:           current,
		Primary:           primary,
		Stale:             stale,
		RecommendedAction: recommendedAction,
		SafeToRemove:      safeToRemove,
	}
}

func worktreePathStatus(path string) (bool, bool) {
	_, err := os.Stat(path)
	pathExists := err == nil

	_, err = os.Stat(filepath.Join(path, ".git"))
	dotGitExists := err == nil

	return pathExists, dotGitExists
}

func samePath(a string, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func parseListFilters(staleOnly bool, safeToRemoveOnly bool, recommended string) (listFilters, error) {
	out := listFilters{
		StaleOnly:        staleOnly,
		SafeToRemoveOnly: safeToRemoveOnly,
	}
	if recommended == "" {
		return out, nil
	}
	switch recommended {
	case "none", "prune", "remove":
		out.Recommended = recommended
		return out, nil
	default:
		return listFilters{}, fmt.Errorf("wt list: invalid --recommended value %q (expected: none, prune, remove)", recommended)
	}
}

func signalsMatchListFilters(signals listSignals, filters listFilters) bool {
	if filters.StaleOnly && !signals.Stale {
		return false
	}
	if filters.SafeToRemoveOnly && !signals.SafeToRemove {
		return false
	}
	if filters.Recommended != "" && signals.RecommendedAction != filters.Recommended {
		return false
	}
	return true
}
