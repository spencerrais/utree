package workspace

import (
	"fmt"
	"strings"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
)

type SafetyKind string

const (
	SafetyDirty         SafetyKind = "dirty"
	SafetyCleanMerged   SafetyKind = "clean_merged"
	SafetyCleanUnmerged SafetyKind = "clean_unmerged"
	SafetyNoLocalBranch SafetyKind = "no_local_branch"
)

type WorktreeStatus struct {
	HasUnstagedChanges bool
	HasStagedChanges   bool
	HasUntrackedFiles  bool
}

func (s WorktreeStatus) Dirty() bool {
	return s.HasUnstagedChanges || s.HasStagedChanges || s.HasUntrackedFiles
}

type RemovalSafety struct {
	Worktree      git.Worktree
	Branch        string
	DefaultBranch string
	Status        WorktreeStatus
	Kind          SafetyKind
	BranchMerged  bool
}

type SafetyOptions struct {
	DefaultBranchOverride *string
}

type SafetyDependencies interface {
	StatusPorcelain(path string) (string, error)
	DefaultBranch(proj project.Project, configDefaultBranch string, override *string) (string, error)
	BranchMerged(defaultBranch string, branch string) (bool, error)
}

func ParseWorktreeStatusPorcelain(output string) WorktreeStatus {
	status := WorktreeStatus{}
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "??") {
			status.HasUntrackedFiles = true
			continue
		}
		if len(line) < 2 {
			status.HasStagedChanges = true
			status.HasUnstagedChanges = true
			continue
		}
		if line[0] != ' ' && line[0] != '?' {
			status.HasStagedChanges = true
		}
		if line[1] != ' ' && line[1] != '?' {
			status.HasUnstagedChanges = true
		}
	}
	return status
}

func AssessRemovalSafety(proj project.Project, worktree git.Worktree, cfg config.Config, options SafetyOptions, deps SafetyDependencies) (RemovalSafety, error) {
	deps = withDefaultSafetyDependencies(proj, deps)
	branch := strings.TrimSpace(worktree.Branch)
	if branch == "" {
		return RemovalSafety{Worktree: worktree, Kind: SafetyNoLocalBranch}, nil
	}

	statusOutput, err := deps.StatusPorcelain(worktree.Path)
	if err != nil {
		return RemovalSafety{}, err
	}
	status := ParseWorktreeStatusPorcelain(statusOutput)
	safety := RemovalSafety{Worktree: worktree, Branch: branch, Status: status}
	if status.Dirty() {
		safety.Kind = SafetyDirty
		return safety, nil
	}

	defaultBranch, err := deps.DefaultBranch(proj, cfg.Git.DefaultBranch, options.DefaultBranchOverride)
	if err != nil {
		return RemovalSafety{}, err
	}
	safety.DefaultBranch = defaultBranch
	merged, err := deps.BranchMerged(defaultBranch, branch)
	if err != nil {
		return RemovalSafety{}, err
	}
	safety.BranchMerged = merged
	if merged {
		safety.Kind = SafetyCleanMerged
	} else {
		safety.Kind = SafetyCleanUnmerged
	}
	return safety, nil
}

func withDefaultSafetyDependencies(proj project.Project, deps SafetyDependencies) SafetyDependencies {
	if deps != nil {
		return deps
	}
	return defaultSafetyDependencies{project: proj}
}

type defaultSafetyDependencies struct {
	project project.Project
}

func (d defaultSafetyDependencies) StatusPorcelain(path string) (string, error) {
	return git.Adapter{Dir: d.project.GitRoot}.StatusPorcelain(path)
}

func (d defaultSafetyDependencies) DefaultBranch(proj project.Project, configDefaultBranch string, override *string) (string, error) {
	branch, err := git.ResolveDefaultBranch(git.FallbackMode, git.DefaultBranchOptions{CLIOverride: override, ConfigDefaultBranch: configDefaultBranch}, git.CommandBranchSource{Dir: proj.GitRoot})
	if err != nil {
		return "", fmt.Errorf("detect default branch: %w", err)
	}
	return branch, nil
}

func (d defaultSafetyDependencies) BranchMerged(defaultBranch string, branch string) (bool, error) {
	return git.Adapter{Dir: d.project.GitRoot}.BranchMerged(defaultBranch, branch)
}
