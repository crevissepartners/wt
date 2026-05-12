package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	postCreateSymlinkSection       = "hooks.postCreate.symlinks"
	legacyPostCreateSymlinkSection = "postCreate.symlink"

	missingSourceSkip = "skip"
	missingSourceFail = "fail"

	existingTargetFail = "fail"
	existingTargetSkip = "skip"
)

type postCreateConfig struct {
	Symlinks []postCreateSymlink
}

type postCreateSymlink struct {
	Source           string
	Target           string
	OnMissingSource  string
	OnExistingTarget string
	line             int
}

func loadPostCreateConfig(primaryRoot string, readFile func(string) ([]byte, error)) (postCreateConfig, error) {
	path := filepath.Join(primaryRoot, ".wt", "config.toml")
	if readFile == nil {
		readFile = os.ReadFile
	}

	data, err := readFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return postCreateConfig{}, nil
		}
		return postCreateConfig{}, fmt.Errorf("wt config: read %s: %w", path, err)
	}

	cfg, err := parsePostCreateConfig(path, data)
	if err != nil {
		return postCreateConfig{}, err
	}
	return cfg, nil
}

func parsePostCreateConfig(path string, data []byte) (postCreateConfig, error) {
	var cfg postCreateConfig
	current := -1
	section := ""

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripTOMLComment(scanner.Text()))
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			if isPostCreateSymlinkSection(section) {
				cfg.Symlinks = append(cfg.Symlinks, postCreateSymlink{
					OnMissingSource:  missingSourceSkip,
					OnExistingTarget: existingTargetFail,
					line:             lineNo,
				})
				current = len(cfg.Symlinks) - 1
				continue
			}
			current = -1
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			current = -1
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return postCreateConfig{}, fmt.Errorf("wt config: %s:%d: expected key = value", path, lineNo)
		}
		if !isPostCreateSymlinkSection(section) {
			continue
		}
		if current < 0 {
			return postCreateConfig{}, fmt.Errorf("wt config: %s:%d: key outside [[%s]]", path, lineNo, postCreateSymlinkSection)
		}

		key = strings.TrimSpace(key)
		str, err := parseTOMLString(strings.TrimSpace(value))
		if err != nil {
			return postCreateConfig{}, fmt.Errorf("wt config: %s:%d: %w", path, lineNo, err)
		}

		switch key {
		case "source":
			cfg.Symlinks[current].Source = str
		case "target":
			cfg.Symlinks[current].Target = str
		case "onMissingSource":
			cfg.Symlinks[current].OnMissingSource = str
		case "onExistingTarget":
			cfg.Symlinks[current].OnExistingTarget = str
		default:
			return postCreateConfig{}, fmt.Errorf("wt config: %s:%d: unknown %s key %q", path, lineNo, postCreateSymlinkSection, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return postCreateConfig{}, fmt.Errorf("wt config: %s: %w", path, err)
	}

	for i := range cfg.Symlinks {
		if err := normalizePostCreateSymlink(path, &cfg.Symlinks[i]); err != nil {
			return postCreateConfig{}, err
		}
	}
	return cfg, nil
}

func isPostCreateSymlinkSection(section string) bool {
	return section == postCreateSymlinkSection || section == legacyPostCreateSymlinkSection
}

func normalizePostCreateSymlink(path string, link *postCreateSymlink) error {
	source, err := cleanConfigRelativePath(link.Source)
	if err != nil {
		return fmt.Errorf("wt config: %s:%d: invalid source: %w", path, link.line, err)
	}
	link.Source = source

	if strings.TrimSpace(link.Target) == "" {
		link.Target = link.Source
	}
	target, err := cleanConfigRelativePath(link.Target)
	if err != nil {
		return fmt.Errorf("wt config: %s:%d: invalid target: %w", path, link.line, err)
	}
	link.Target = target

	switch link.OnMissingSource {
	case missingSourceSkip, missingSourceFail:
	default:
		return fmt.Errorf("wt config: %s:%d: onMissingSource must be %q or %q", path, link.line, missingSourceSkip, missingSourceFail)
	}
	switch link.OnExistingTarget {
	case existingTargetFail, existingTargetSkip:
	default:
		return fmt.Errorf("wt config: %s:%d: onExistingTarget must be %q or %q", path, link.line, existingTargetFail, existingTargetSkip)
	}
	return nil
}

func cleanConfigRelativePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	path = filepath.Clean(filepath.FromSlash(path))
	if path == "." || path == ".." || strings.HasPrefix(path, ".."+string(os.PathSeparator)) || filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be relative and stay within its root")
	}
	return path, nil
}

func parseTOMLString(value string) (string, error) {
	if strings.HasPrefix(value, `"`) {
		out, err := strconv.Unquote(value)
		if err != nil {
			return "", fmt.Errorf("invalid string: %w", err)
		}
		return out, nil
	}
	if strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`) && len(value) >= 2 {
		return value[1 : len(value)-1], nil
	}
	return "", fmt.Errorf("expected string value")
}

func stripTOMLComment(line string) string {
	inSingle := false
	inDouble := false
	escaped := false

	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case inDouble && r == '\\':
			escaped = true
		case !inSingle && r == '"':
			inDouble = !inDouble
		case !inDouble && r == '\'':
			inSingle = !inSingle
		case !inSingle && !inDouble && r == '#':
			return line[:i]
		}
	}
	return line
}

func applyPostCreateSymlinks(primaryRoot string, worktreePath string, cfg postCreateConfig, dryRun bool) error {
	for _, link := range cfg.Symlinks {
		if err := applyPostCreateSymlink(primaryRoot, worktreePath, link, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func applyPostCreateSymlink(primaryRoot string, worktreePath string, link postCreateSymlink, dryRun bool) error {
	sourcePath := filepath.Join(primaryRoot, link.Source)
	targetPath := filepath.Join(worktreePath, link.Target)

	if _, err := os.Lstat(sourcePath); err != nil {
		if os.IsNotExist(err) && link.OnMissingSource == missingSourceSkip {
			if dryRun {
				fmt.Fprintf(os.Stderr, "dry-run: skip symlink %s -> %s (source missing)\n", targetPath, sourcePath)
			}
			return nil
		}
		return fmt.Errorf("wt post-create symlink: source %s: %w", sourcePath, err)
	}

	if existing, err := os.Lstat(targetPath); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			if sameSymlinkTarget(targetPath, sourcePath) {
				if dryRun {
					fmt.Fprintf(os.Stderr, "dry-run: symlink exists %s -> %s\n", targetPath, sourcePath)
				}
				return nil
			}
		}
		if link.OnExistingTarget == existingTargetSkip {
			if dryRun {
				fmt.Fprintf(os.Stderr, "dry-run: skip symlink %s -> %s (target exists)\n", targetPath, sourcePath)
			}
			return nil
		}
		return fmt.Errorf("wt post-create symlink: target already exists: %s", targetPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("wt post-create symlink: target %s: %w", targetPath, err)
	}

	linkValue, err := filepath.Rel(filepath.Dir(targetPath), sourcePath)
	if err != nil {
		linkValue = sourcePath
	}
	if dryRun {
		fmt.Fprintf(os.Stderr, "dry-run: ln -s %s %s\n", linkValue, targetPath)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("wt post-create symlink: mkdir %s: %w", filepath.Dir(targetPath), err)
	}
	if err := os.Symlink(linkValue, targetPath); err != nil {
		return fmt.Errorf("wt post-create symlink: ln -s %s %s: %w", linkValue, targetPath, err)
	}
	return nil
}

func sameSymlinkTarget(targetPath string, sourcePath string) bool {
	linkValue, err := os.Readlink(targetPath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(linkValue) {
		linkValue = filepath.Join(filepath.Dir(targetPath), linkValue)
	}
	linkAbs, err := filepath.Abs(filepath.Clean(linkValue))
	if err != nil {
		return false
	}
	sourceAbs, err := filepath.Abs(filepath.Clean(sourcePath))
	if err != nil {
		return false
	}
	return linkAbs == sourceAbs
}
