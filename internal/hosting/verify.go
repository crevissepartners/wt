package hosting

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/crevissepartners/wt/internal/runner"
)

type Provider string

const (
	ProviderUnknown Provider = "unknown"
	ProviderGitHub  Provider = "github"
	ProviderGitLab  Provider = "gitlab"
)

type VerifyResult struct {
	Provider Provider
	Kind     string
	Merged   *bool
	Reason   string
	Number   *int
	Title    string
	URL      string
}

func DetectProvider(remoteURL string) Provider {
	normalized := strings.ToLower(strings.TrimSpace(remoteURL))
	switch {
	case strings.Contains(normalized, "github.com"):
		return ProviderGitHub
	case strings.Contains(normalized, "gitlab.com"), strings.Contains(normalized, "gitlab"):
		return ProviderGitLab
	default:
		return ProviderUnknown
	}
}

func VerifyMerged(ctx context.Context, r runner.Runner, repoRoot string, provider Provider, branch string, baseRef string) (VerifyResult, error) {
	if strings.TrimSpace(branch) == "" {
		return VerifyResult{Provider: provider, Kind: kindForProvider(provider), Reason: "no-branch"}, nil
	}

	auth, err := VerifyAuth(ctx, r, repoRoot, provider)
	if err != nil {
		return VerifyResult{}, err
	}
	if auth.Reason != "" {
		return auth, nil
	}

	return QueryMerged(ctx, r, repoRoot, provider, branch, baseRef)
}

func VerifyAuth(ctx context.Context, r runner.Runner, repoRoot string, provider Provider) (VerifyResult, error) {
	switch provider {
	case ProviderGitHub:
		return verifyGitHubAuth(ctx, r, repoRoot)
	case ProviderGitLab:
		return verifyGitLabAuth(ctx, r, repoRoot)
	default:
		return VerifyResult{Provider: provider, Kind: kindForProvider(provider), Reason: "unsupported-provider"}, nil
	}
}

func QueryMerged(ctx context.Context, r runner.Runner, repoRoot string, provider Provider, branch string, baseRef string) (VerifyResult, error) {
	if strings.TrimSpace(branch) == "" {
		return VerifyResult{Provider: provider, Kind: kindForProvider(provider), Reason: "no-branch"}, nil
	}

	switch provider {
	case ProviderGitHub:
		return queryGitHubMerged(ctx, r, repoRoot, branch, baseRef)
	case ProviderGitLab:
		return queryGitLabMerged(ctx, r, repoRoot, branch, baseRef)
	default:
		return VerifyResult{Provider: provider, Kind: kindForProvider(provider), Reason: "unsupported-provider"}, nil
	}
}

func verifyGitHubAuth(ctx context.Context, r runner.Runner, repoRoot string) (VerifyResult, error) {
	ghBin, ok := findGitHubCLI()
	if !ok {
		return VerifyResult{Provider: ProviderGitHub, Kind: "pr", Reason: "gh-auth-unavailable"}, nil
	}

	if _, err := r.Run(ctx, repoRoot, ghBin, "auth", "status"); err != nil {
		return VerifyResult{Provider: ProviderGitHub, Kind: "pr", Reason: "gh-auth-unavailable"}, nil
	}

	return VerifyResult{Provider: ProviderGitHub, Kind: "pr"}, nil
}

func queryGitHubMerged(ctx context.Context, r runner.Runner, repoRoot string, branch string, baseRef string) (VerifyResult, error) {
	ghBin, ok := findGitHubCLI()
	if !ok {
		return VerifyResult{Provider: ProviderGitHub, Kind: "pr", Reason: "gh-auth-unavailable"}, nil
	}

	args := []string{"pr", "list", "--state", "merged", "--head", branch, "--json", "number,title,url", "--limit", "1"}
	if shortBase := shortRefName(baseRef); shortBase != "" {
		args = append(args, "--base", shortBase)
	}

	res, err := r.Run(ctx, repoRoot, ghBin, args...)
	if err != nil {
		return VerifyResult{Provider: ProviderGitHub, Kind: "pr", Reason: "gh-pr-query-failed"}, nil
	}

	var prs []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(res.Stdout, &prs); err != nil {
		return VerifyResult{Provider: ProviderGitHub, Kind: "pr", Reason: "gh-invalid-json"}, nil
	}

	merged := len(prs) > 0
	result := VerifyResult{Provider: ProviderGitHub, Kind: "pr", Merged: &merged}
	if merged {
		result.Number = &prs[0].Number
		result.Title = strings.TrimSpace(prs[0].Title)
		result.URL = strings.TrimSpace(prs[0].URL)
	}
	return result, nil
}

func verifyGitLabAuth(ctx context.Context, r runner.Runner, repoRoot string) (VerifyResult, error) {
	glabBin, ok := findGitLabCLI()
	if !ok {
		return VerifyResult{Provider: ProviderGitLab, Kind: "mr", Reason: "glab-auth-unavailable"}, nil
	}

	if _, err := r.Run(ctx, repoRoot, glabBin, "auth", "status"); err != nil {
		return VerifyResult{Provider: ProviderGitLab, Kind: "mr", Reason: "glab-auth-unavailable"}, nil
	}

	return VerifyResult{Provider: ProviderGitLab, Kind: "mr"}, nil
}

func queryGitLabMerged(ctx context.Context, r runner.Runner, repoRoot string, branch string, baseRef string) (VerifyResult, error) {
	glabBin, ok := findGitLabCLI()
	if !ok {
		return VerifyResult{Provider: ProviderGitLab, Kind: "mr", Reason: "glab-auth-unavailable"}, nil
	}

	endpoint := "projects/:fullpath/merge_requests?state=merged&source_branch=" + url.QueryEscape(branch) + "&per_page=1&order_by=updated_at&sort=desc"
	if shortBase := shortRefName(baseRef); shortBase != "" {
		endpoint += "&target_branch=" + url.QueryEscape(shortBase)
	}

	res, err := r.Run(ctx, repoRoot, glabBin, "api", endpoint)
	if err != nil {
		return VerifyResult{Provider: ProviderGitLab, Kind: "mr", Reason: "glab-mr-query-failed"}, nil
	}

	var mrs []struct {
		IID      int    `json:"iid"`
		Title    string `json:"title"`
		WebURL   string `json:"web_url"`
		MergedAt string `json:"merged_at"`
	}
	if err := json.Unmarshal(res.Stdout, &mrs); err != nil {
		return VerifyResult{Provider: ProviderGitLab, Kind: "mr", Reason: "glab-invalid-json"}, nil
	}

	merged := len(mrs) > 0 && strings.TrimSpace(mrs[0].MergedAt) != ""
	result := VerifyResult{Provider: ProviderGitLab, Kind: "mr", Merged: &merged}
	if merged {
		result.Number = &mrs[0].IID
		result.Title = strings.TrimSpace(mrs[0].Title)
		result.URL = strings.TrimSpace(mrs[0].WebURL)
	}
	return result, nil
}

func kindForProvider(provider Provider) string {
	switch provider {
	case ProviderGitHub:
		return "pr"
	case ProviderGitLab:
		return "mr"
	default:
		return "unknown"
	}
}

func findGitHubCLI() (string, bool) {
	return findCLI("WT_GH_BIN", "gh")
}

func findGitLabCLI() (string, bool) {
	return findCLI("WT_GLAB_BIN", "glab")
}

func findCLI(explicitEnv string, defaultName string) (string, bool) {
	if explicit := strings.TrimSpace(os.Getenv(explicitEnv)); explicit != "" {
		return explicit, true
	}

	if path, err := exec.LookPath(defaultName); err == nil {
		return path, true
	}

	return "", false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func shortRefName(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	ref = strings.TrimPrefix(ref, "origin/")
	ref = strings.TrimPrefix(ref, "upstream/")
	return ref
}
