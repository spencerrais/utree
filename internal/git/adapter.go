package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Adapter struct {
	Dir string
	Run CommandRunner
}

// Worktree is a parsed entry from `git worktree list --porcelain`.
type Worktree struct {
	Path   string
	Branch string
	Name   string
}

func (a Adapter) WorktreeList() ([]Worktree, error) {
	output, err := a.run(a.Dir, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	worktrees, err := ParseWorktreeListPorcelain(string(output))
	if err != nil {
		return nil, err
	}
	return worktrees, nil
}

func (a Adapter) WorktreeAdd(path string, branch string) error {
	if _, err := a.run(a.Dir, "git", "worktree", "add", path, branch); err != nil {
		return fmt.Errorf("git worktree add %s %s: %w", path, branch, err)
	}
	return nil
}

func (a Adapter) WorktreeAddNewBranch(path string, branch string, startPoint string) error {
	if _, err := a.run(a.Dir, "git", "worktree", "add", "-b", branch, path, startPoint); err != nil {
		return fmt.Errorf("git worktree add -b %s %s %s: %w", branch, path, startPoint, err)
	}
	return nil
}

func (a Adapter) WorktreeRemove(path string) error {
	if _, err := a.run(a.Dir, "git", "worktree", "remove", path); err != nil {
		return fmt.Errorf("git worktree remove %s: %w", path, err)
	}
	return nil
}

func (a Adapter) StatusPorcelain(dir string) (string, error) {
	output, err := a.run(dir, "git", "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("git status --porcelain: %w", err)
	}
	return string(output), nil
}

func (a Adapter) LocalBranchExists(name string) (bool, error) {
	_, err := a.run(a.Dir, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true, nil
	}
	return false, nil
}

func (a Adapter) DeleteLocalBranch(name string) error {
	if _, err := a.run(a.Dir, "git", "branch", "-d", name); err != nil {
		return fmt.Errorf("git branch -d %s: %w", name, err)
	}
	return nil
}

func (a Adapter) ForceDeleteLocalBranch(name string) error {
	if _, err := a.run(a.Dir, "git", "branch", "-D", name); err != nil {
		return fmt.Errorf("git branch -D %s: %w", name, err)
	}
	return nil
}

func (a Adapter) BranchMerged(defaultBranch string, branch string) (bool, error) {
	output, err := a.run(a.Dir, "git", "branch", "--merged", defaultBranch, "--list", branch)
	if err != nil {
		return false, fmt.Errorf("git branch --merged %s --list %s: %w", defaultBranch, branch, err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		if line == branch {
			return true, nil
		}
	}
	return false, nil
}

// ParseWorktreeListPorcelain parses output from `git worktree list --porcelain`.
func ParseWorktreeListPorcelain(output string) ([]Worktree, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	entries := strings.Split(output, "\n\n")
	worktrees := make([]Worktree, 0, len(entries))
	for _, entry := range entries {
		worktree, err := parseWorktreeListEntry(entry)
		if err != nil {
			return nil, err
		}
		worktrees = append(worktrees, worktree)
	}

	return worktrees, nil
}

func parseWorktreeListEntry(entry string) (Worktree, error) {
	worktree := Worktree{}

	for _, line := range strings.Split(entry, "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			worktree.Path = path
			worktree.Name = filepath.Base(path)
			continue
		}
		if branch, ok := strings.CutPrefix(line, "branch "); ok {
			worktree.Branch = strings.TrimPrefix(branch, "refs/heads/")
		}
	}

	if worktree.Path == "" {
		return Worktree{}, fmt.Errorf("parse git worktree list entry: missing worktree path")
	}

	return worktree, nil
}

func (a Adapter) run(dir string, name string, args ...string) ([]byte, error) {
	if a.Run != nil {
		return a.Run(dir, name, args...)
	}

	cmd := Command(dir, args...)
	return cmd.Output()
}
