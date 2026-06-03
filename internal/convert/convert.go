package convert

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
)

var (
	ErrNotRepositoryRoot          = errors.New("current directory is not the git repository root")
	ErrCurrentBranchNotDefault    = errors.New("current branch is not the detected default branch")
	ErrPrimaryTargetExists        = errors.New("target primary worktree directory already exists")
	ErrUtreeAlreadyExists         = errors.New(".utree directory already exists")
	ErrLinkedWorktrees            = errors.New("repository has existing linked worktrees")
	ErrSubmodules                 = errors.New("repository uses git submodules")
	ErrDirtyRepository            = errors.New("repository has local changes")
	ErrInvalidPrimaryWorktreeName = errors.New("invalid primary worktree name")
	ErrInvalidAdoptLayout         = errors.New("current repository is not in project/worktree layout")
	ErrAlreadyUtreeProject        = errors.New("already inside a utree project")
)

type Options struct {
	DefaultBranchOverride *string
}

type Dependencies struct {
	GitRoot             func(startDir string) (string, error)
	DefaultBranch       func(repoRoot string, override *string) (string, error)
	ConfigDefaultBranch func(repoRoot string) (string, error)
	BranchSource        func(repoRoot string) git.BranchSource
	CurrentBranch       func(repoRoot string) (string, error)
	Worktrees           func(repoRoot string) ([]git.Worktree, error)
	StatusPorcelain     func(repoRoot string) (string, error)
	HasSubmodules       func(repoRoot string) (bool, error)
}

type Plan struct {
	From          string
	PrimaryTarget string
	MarkerPath    string
	DefaultBranch string
}

type AdoptPlan struct {
	ProjectRoot     string
	WorktreeRoot    string
	WorktreeName    string
	MarkerPath      string
	LinkedWorktrees []string
}

func PlanConversion(startDir string, options Options, deps Dependencies) (Plan, error) {
	deps = withDefaultDependencies(deps)

	startDir, err := cleanAbs(startDir)
	if err != nil {
		return Plan{}, err
	}
	repoRoot, err := deps.GitRoot(startDir)
	if err != nil {
		return Plan{}, err
	}
	repoRoot, err = cleanAbs(repoRoot)
	if err != nil {
		return Plan{}, err
	}
	if err := rejectExistingUtreeProject(repoRoot); err != nil {
		return Plan{}, err
	}
	if startDir != repoRoot {
		return Plan{}, fmt.Errorf("%w: current %s git root %s", ErrNotRepositoryRoot, startDir, repoRoot)
	}

	defaultBranch, err := deps.DefaultBranch(repoRoot, options.DefaultBranchOverride)
	if err != nil {
		return Plan{}, err
	}
	if strings.TrimSpace(defaultBranch) == "" {
		return Plan{}, git.ErrDefaultBranchNotDetected
	}

	currentBranch, err := deps.CurrentBranch(repoRoot)
	if err != nil {
		return Plan{}, err
	}
	if currentBranch != defaultBranch {
		return Plan{}, fmt.Errorf("%w: detected default branch %s current branch %s", ErrCurrentBranchNotDefault, defaultBranch, currentBranch)
	}

	primaryTarget, err := project.PlanSiblingWorktreePath(repoRoot, defaultBranch)
	if err != nil {
		if errors.Is(err, project.ErrTargetExists) {
			return Plan{}, fmt.Errorf("%w: %s", ErrPrimaryTargetExists, filepath.Join(repoRoot, defaultBranch))
		}
		return Plan{}, fmt.Errorf("%w: %q", ErrInvalidPrimaryWorktreeName, defaultBranch)
	}
	if _, err := os.Lstat(primaryTarget); err == nil {
		return Plan{}, fmt.Errorf("%w: %s", ErrPrimaryTargetExists, primaryTarget)
	} else if !os.IsNotExist(err) {
		return Plan{}, fmt.Errorf("check primary target %s: %w", primaryTarget, err)
	}

	utreeDir := filepath.Join(repoRoot, ".utree")
	if _, err := os.Lstat(utreeDir); err == nil {
		return Plan{}, fmt.Errorf("%w: %s", ErrUtreeAlreadyExists, utreeDir)
	} else if !os.IsNotExist(err) {
		return Plan{}, fmt.Errorf("check .utree %s: %w", utreeDir, err)
	}

	worktrees, err := deps.Worktrees(repoRoot)
	if err != nil {
		return Plan{}, err
	}
	if len(worktrees) != 1 || cleanPath(worktrees[0].Path) != repoRoot {
		return Plan{}, ErrLinkedWorktrees
	}

	hasSubmodules, err := deps.HasSubmodules(repoRoot)
	if err != nil {
		return Plan{}, err
	}
	if hasSubmodules {
		return Plan{}, ErrSubmodules
	}

	status, err := deps.StatusPorcelain(repoRoot)
	if err != nil {
		return Plan{}, err
	}
	if dirtyStatus(status) {
		return Plan{}, ErrDirtyRepository
	}

	return Plan{
		From:          repoRoot,
		PrimaryTarget: primaryTarget,
		MarkerPath:    utreeDir,
		DefaultBranch: defaultBranch,
	}, nil
}

func PlanAdoption(startDir string, deps Dependencies) (AdoptPlan, error) {
	deps = withDefaultDependencies(deps)

	gitRoot, err := deps.GitRoot(startDir)
	if err != nil {
		return AdoptPlan{}, err
	}
	gitRoot, err = cleanAbs(gitRoot)
	if err != nil {
		return AdoptPlan{}, err
	}
	if err := rejectExistingUtreeProject(gitRoot); err != nil {
		return AdoptPlan{}, err
	}
	projectRoot := filepath.Dir(gitRoot)
	if projectRoot == gitRoot || filepath.Base(gitRoot) == "." || filepath.Base(gitRoot) == string(filepath.Separator) {
		return AdoptPlan{}, ErrInvalidAdoptLayout
	}
	worktreeName := filepath.Base(gitRoot)
	if strings.TrimSpace(worktreeName) == "" {
		return AdoptPlan{}, ErrInvalidAdoptLayout
	}
	currentBranch, err := deps.CurrentBranch(gitRoot)
	if err != nil {
		return AdoptPlan{}, err
	}
	if strings.TrimSpace(currentBranch) == "" || currentBranch != worktreeName {
		return AdoptPlan{}, fmt.Errorf("%w: worktree directory %q does not match current branch %q", ErrInvalidAdoptLayout, worktreeName, currentBranch)
	}

	utreeDir := filepath.Join(projectRoot, ".utree")
	if _, err := os.Lstat(utreeDir); err == nil {
		return AdoptPlan{}, fmt.Errorf("%w: %s", ErrUtreeAlreadyExists, utreeDir)
	} else if !os.IsNotExist(err) {
		return AdoptPlan{}, fmt.Errorf("check .utree %s: %w", utreeDir, err)
	}

	worktrees, err := deps.Worktrees(gitRoot)
	if err != nil {
		return AdoptPlan{}, err
	}
	if len(worktrees) == 0 {
		return AdoptPlan{}, ErrInvalidAdoptLayout
	}

	linkedWorktrees := make([]string, 0, len(worktrees))
	foundCurrent := false
	for _, worktree := range worktrees {
		path := cleanPath(worktree.Path)
		if path == gitRoot {
			foundCurrent = true
		}
		if filepath.Dir(path) != projectRoot {
			return AdoptPlan{}, fmt.Errorf("%w: linked worktree outside project root: %s", ErrInvalidAdoptLayout, path)
		}
		name := filepath.Base(path)
		if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
			return AdoptPlan{}, fmt.Errorf("%w: invalid worktree path: %s", ErrInvalidAdoptLayout, path)
		}
		linkedWorktrees = append(linkedWorktrees, path)
	}
	if !foundCurrent {
		return AdoptPlan{}, fmt.Errorf("%w: current git root not listed as a worktree", ErrInvalidAdoptLayout)
	}

	return AdoptPlan{
		ProjectRoot:     projectRoot,
		WorktreeRoot:    gitRoot,
		WorktreeName:    worktreeName,
		MarkerPath:      utreeDir,
		LinkedWorktrees: linkedWorktrees,
	}, nil
}

func rejectExistingUtreeProject(gitRoot string) error {
	markerPath := filepath.Join(filepath.Dir(gitRoot), ".utree")
	info, err := os.Stat(markerPath)
	if err == nil {
		if info.IsDir() {
			return fmt.Errorf("%w: %s", ErrAlreadyUtreeProject, markerPath)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("stat utree marker %s: %w", markerPath, err)
}

func Execute(plan Plan, confirmation io.Reader) (bool, error) {
	if !confirmed(confirmation) {
		return false, nil
	}

	if err := os.Mkdir(plan.MarkerPath, 0o755); err != nil {
		return false, fmt.Errorf("create .utree: %w", err)
	}
	if err := os.Mkdir(plan.PrimaryTarget, 0o755); err != nil {
		_ = os.Remove(plan.MarkerPath)
		return false, fmt.Errorf("create primary worktree dir: %w", err)
	}

	entries, err := os.ReadDir(plan.From)
	if err != nil {
		return false, fmt.Errorf("read repository root: %w", err)
	}
	primaryName := filepath.Base(plan.PrimaryTarget)
	for _, entry := range entries {
		name := entry.Name()
		if name == ".utree" || name == primaryName {
			continue
		}
		from := filepath.Join(plan.From, name)
		to := filepath.Join(plan.PrimaryTarget, name)
		if err := os.Rename(from, to); err != nil {
			return false, fmt.Errorf("move %s to %s: %w", from, to, err)
		}
	}

	return true, nil
}

func ExecuteAdoption(plan AdoptPlan, confirmation io.Reader) (bool, error) {
	if !confirmed(confirmation) {
		return false, nil
	}

	if err := os.Mkdir(plan.MarkerPath, 0o755); err != nil {
		return false, fmt.Errorf("create .utree: %w", err)
	}

	return true, nil
}

func confirmed(reader io.Reader) bool {
	if reader == nil {
		return false
	}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

func withDefaultDependencies(deps Dependencies) Dependencies {
	if deps.GitRoot == nil {
		deps.GitRoot = defaultGitRoot
	}
	if deps.ConfigDefaultBranch == nil {
		deps.ConfigDefaultBranch = defaultConfigDefaultBranch
	}
	if deps.BranchSource == nil {
		deps.BranchSource = defaultBranchSource
	}
	if deps.DefaultBranch == nil {
		deps.DefaultBranch = func(repoRoot string, override *string) (string, error) {
			configDefaultBranch, err := deps.ConfigDefaultBranch(repoRoot)
			if err != nil {
				return "", err
			}
			return git.ResolveDefaultBranch(git.FallbackMode, git.DefaultBranchOptions{CLIOverride: override, ConfigDefaultBranch: configDefaultBranch}, deps.BranchSource(repoRoot))
		}
	}
	if deps.CurrentBranch == nil {
		deps.CurrentBranch = defaultCurrentBranch
	}
	if deps.Worktrees == nil {
		deps.Worktrees = defaultWorktrees
	}
	if deps.StatusPorcelain == nil {
		deps.StatusPorcelain = defaultStatusPorcelain
	}
	if deps.HasSubmodules == nil {
		deps.HasSubmodules = defaultHasSubmodules
	}
	return deps
}

func defaultGitRoot(startDir string) (string, error) {
	return project.GitRevParseRoot(startDir)
}

func defaultConfigDefaultBranch(repoRoot string) (string, error) {
	cfg, err := config.Load(repoRoot, config.Overrides{})
	if err != nil {
		return "", err
	}
	return cfg.Git.DefaultBranch, nil
}

func defaultBranchSource(repoRoot string) git.BranchSource {
	return git.CommandBranchSource{Dir: repoRoot}
}

func defaultCurrentBranch(repoRoot string) (string, error) {
	cmd := git.Command(repoRoot, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git branch --show-current: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func defaultWorktrees(repoRoot string) ([]git.Worktree, error) {
	return git.Adapter{Dir: repoRoot}.WorktreeList()
}

func defaultStatusPorcelain(repoRoot string) (string, error) {
	cmd := git.Command(repoRoot, "status", "--porcelain", "--untracked-files=all")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git status --porcelain: %w", err)
	}
	return string(output), nil
}

func defaultHasSubmodules(repoRoot string) (bool, error) {
	_, err := os.Stat(filepath.Join(repoRoot, ".gitmodules"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func dirtyStatus(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "!!") {
			return true
		}
	}
	return false
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func cleanPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
