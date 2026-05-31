package project

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrNotGitRepository    = errors.New("not inside a git repository")
	ErrNotUtreeProject     = errors.New("not inside a utree project")
	ErrNotDirectChild      = errors.New("git root is not a direct child of project root")
	ErrTargetExists        = errors.New("target worktree path already exists")
	ErrInvalidWorktreeName = errors.New("invalid worktree name")
)

type GitRootFinder func(startDir string) (string, error)

type Project struct {
	Root         string
	GitRoot      string
	WorktreeName string
}

func Detect(startDir string, findGitRoot GitRootFinder) (Project, error) {
	gitRoot, err := findGitRoot(startDir)
	if err != nil {
		if !errors.Is(err, ErrNotGitRepository) {
			return Project{}, err
		}
		proj, projectErr := detectFromProjectRoot(startDir, findGitRoot)
		if errors.Is(projectErr, ErrNotUtreeProject) {
			return Project{}, err
		}
		return proj, projectErr
	}

	gitRoot, err = cleanAbs(gitRoot)
	if err != nil {
		return Project{}, err
	}
	projectRoot := filepath.Dir(gitRoot)

	return detectWithProjectRoot(gitRoot, projectRoot)
}

func detectFromProjectRoot(startDir string, findGitRoot GitRootFinder) (Project, error) {
	projectRoot, err := cleanAbs(startDir)
	if err != nil {
		return Project{}, err
	}
	if err := requireMarker(projectRoot); err != nil {
		return Project{}, err
	}

	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		return Project{}, fmt.Errorf("read project root %s: %w", projectRoot, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".utree" {
			continue
		}
		candidate := filepath.Join(projectRoot, entry.Name())
		gitRoot, err := findGitRoot(candidate)
		if err != nil {
			if errors.Is(err, ErrNotGitRepository) {
				continue
			}
			return Project{}, err
		}
		gitRoot, err = cleanAbs(gitRoot)
		if err != nil {
			return Project{}, err
		}
		if gitRoot != candidate {
			continue
		}
		return Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: ""}, nil
	}

	return Project{}, fmt.Errorf("%w: no direct child git worktrees under %s", ErrNotGitRepository, projectRoot)
}

func GitRevParseRoot(startDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = startDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: git rev-parse --show-toplevel failed", ErrNotGitRepository)
	}

	return strings.TrimSpace(string(output)), nil
}

func (p Project) PlanSiblingWorktreePath(worktreeName string) (string, error) {
	return PlanSiblingWorktreePath(p.Root, worktreeName)
}

func PlanSiblingWorktreePath(projectRoot string, worktreeName string) (string, error) {
	if err := validateWorktreeName(worktreeName); err != nil {
		return "", err
	}

	projectRoot, err := cleanAbs(projectRoot)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(projectRoot, worktreeName)

	if filepath.Dir(targetPath) != projectRoot {
		return "", fmt.Errorf("%w: %q", ErrInvalidWorktreeName, worktreeName)
	}

	if _, err := os.Lstat(targetPath); err == nil {
		return "", fmt.Errorf("%w: %s", ErrTargetExists, targetPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check target worktree path %s: %w", targetPath, err)
	}

	return targetPath, nil
}

func detectWithProjectRoot(gitRoot string, projectRoot string) (Project, error) {
	gitRoot, err := cleanAbs(gitRoot)
	if err != nil {
		return Project{}, err
	}
	projectRoot, err = cleanAbs(projectRoot)
	if err != nil {
		return Project{}, err
	}

	if err := requireMarker(projectRoot); err != nil {
		return Project{}, err
	}

	if filepath.Dir(gitRoot) != projectRoot {
		return Project{}, fmt.Errorf("%w: git root %s project root %s", ErrNotDirectChild, gitRoot, projectRoot)
	}

	worktreeName := filepath.Base(gitRoot)
	if worktreeName == "." || worktreeName == string(filepath.Separator) || worktreeName == "" {
		return Project{}, fmt.Errorf("%w: %s", ErrNotDirectChild, gitRoot)
	}

	return Project{
		Root:         projectRoot,
		GitRoot:      gitRoot,
		WorktreeName: worktreeName,
	}, nil
}

func requireMarker(projectRoot string) error {
	markerPath := filepath.Join(projectRoot, ".utree")
	info, err := os.Stat(markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: missing %s", ErrNotUtreeProject, markerPath)
		}
		return fmt.Errorf("stat utree marker %s: %w", markerPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: marker is not a directory: %s", ErrNotUtreeProject, markerPath)
	}
	return nil
}

func validateWorktreeName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidWorktreeName, name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("%w: %q", ErrInvalidWorktreeName, name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("%w: %q", ErrInvalidWorktreeName, name)
	}
	if strings.ContainsRune(name, filepath.Separator) || strings.Contains(name, "/") {
		return fmt.Errorf("%w: %q", ErrInvalidWorktreeName, name)
	}

	return nil
}

func cleanAbs(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make path absolute %s: %w", path, err)
	}
	return filepath.Clean(path), nil
}
