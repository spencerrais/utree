package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFromInsideSiblingWorktree(t *testing.T) {
	projectRoot := newUtreeProject(t)
	gitRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	startDir := mkdir(t, filepath.Join(gitRoot, "subdir"))

	got, err := Detect(startDir, fakeGitRootFinder(gitRoot, nil))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	if got.GitRoot != gitRoot {
		t.Fatalf("expected git root %q, got %q", gitRoot, got.GitRoot)
	}
	if got.Root != projectRoot {
		t.Fatalf("expected project root %q, got %q", projectRoot, got.Root)
	}
	if got.WorktreeName != "main" {
		t.Fatalf("expected worktree name main, got %q", got.WorktreeName)
	}
}

func TestDetectFailsWhenUtreeMarkerMissing(t *testing.T) {
	projectRoot := t.TempDir()
	gitRoot := mkdir(t, filepath.Join(projectRoot, "feature-a"))

	_, err := Detect(gitRoot, fakeGitRootFinder(gitRoot, nil))
	if !errors.Is(err, ErrNotUtreeProject) {
		t.Fatalf("expected ErrNotUtreeProject, got %v", err)
	}
}

func TestDetectAcceptsUtreeDirectoryWithoutMetadata(t *testing.T) {
	projectRoot := t.TempDir()
	mkdir(t, filepath.Join(projectRoot, ".utree"))
	gitRoot := mkdir(t, filepath.Join(projectRoot, "main"))

	got, err := Detect(gitRoot, fakeGitRootFinder(gitRoot, nil))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if got.Root != projectRoot {
		t.Fatalf("expected project root %q, got %q", projectRoot, got.Root)
	}
}

func TestDetectFromProjectRootUsesDirectChildGitWorktree(t *testing.T) {
	projectRoot := newUtreeProject(t)
	gitRoot := mkdir(t, filepath.Join(projectRoot, "main"))

	got, err := Detect(projectRoot, fakeGitRootFinderMap(map[string]string{gitRoot: gitRoot}))
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}

	if got.Root != projectRoot {
		t.Fatalf("expected project root %q, got %q", projectRoot, got.Root)
	}
	if got.GitRoot != gitRoot {
		t.Fatalf("expected git root %q, got %q", gitRoot, got.GitRoot)
	}
	if got.WorktreeName != "" {
		t.Fatalf("expected empty worktree name at project root, got %q", got.WorktreeName)
	}
}

func TestDetectFromProjectRootFailsWithoutChildGitWorktree(t *testing.T) {
	projectRoot := newUtreeProject(t)
	mkdir(t, filepath.Join(projectRoot, "not-git"))

	_, err := Detect(projectRoot, fakeGitRootFinderMap(nil))
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("expected ErrNotGitRepository, got %v", err)
	}
}

func TestDetectFailsWhenNotInsideGitRepo(t *testing.T) {
	_, err := Detect(t.TempDir(), fakeGitRootFinder("", ErrNotGitRepository))
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("expected ErrNotGitRepository, got %v", err)
	}
}

func TestDetectFailsWhenGitRootIsProjectRoot(t *testing.T) {
	projectRoot := newUtreeProject(t)

	_, err := Detect(projectRoot, fakeGitRootFinder(projectRoot, nil))
	if !errors.Is(err, ErrNotUtreeProject) {
		t.Fatalf("expected ErrNotUtreeProject, got %v", err)
	}
}

func TestDetectFailsWhenGitRootIsNotDirectChild(t *testing.T) {
	projectRoot := newUtreeProject(t)
	nestedGitRoot := mkdir(t, filepath.Join(projectRoot, "nested", "feature-a"))

	_, err := detectWithProjectRoot(nestedGitRoot, projectRoot)
	if !errors.Is(err, ErrNotDirectChild) {
		t.Fatalf("expected ErrNotDirectChild, got %v", err)
	}
}

func TestSiblingPathPlansDirectChildUnderProjectRoot(t *testing.T) {
	projectRoot := newUtreeProject(t)
	project := Project{Root: projectRoot}

	got, err := project.PlanSiblingWorktreePath("feature-a")
	if err != nil {
		t.Fatalf("PlanSiblingWorktreePath returned error: %v", err)
	}

	want := filepath.Join(projectRoot, "feature-a")
	if got != want {
		t.Fatalf("expected target %q, got %q", want, got)
	}
}

func TestSiblingPathRejectsExistingPath(t *testing.T) {
	projectRoot := newUtreeProject(t)
	existingPath := mkdir(t, filepath.Join(projectRoot, "feature-a"))
	project := Project{Root: projectRoot}

	_, err := project.PlanSiblingWorktreePath("feature-a")
	if !errors.Is(err, ErrTargetExists) {
		t.Fatalf("expected ErrTargetExists, got %v", err)
	}
	if _, err := os.Stat(existingPath); err != nil {
		t.Fatalf("expected existing path to remain, got stat error: %v", err)
	}
}

func TestSiblingPathRejectsUnsafeWorktreeNames(t *testing.T) {
	project := Project{Root: t.TempDir()}
	testCases := []string{
		"",
		"../escape",
		"nested/name",
		".",
		"..",
		filepath.Join("nested", "name"),
		filepath.FromSlash("nested/name"),
	}

	for _, testCase := range testCases {
		t.Run(testCase, func(t *testing.T) {
			_, err := project.PlanSiblingWorktreePath(testCase)
			if !errors.Is(err, ErrInvalidWorktreeName) {
				t.Fatalf("expected ErrInvalidWorktreeName, got %v", err)
			}
		})
	}
}

func fakeGitRootFinder(gitRoot string, err error) GitRootFinder {
	return func(string) (string, error) {
		return gitRoot, err
	}
}

func fakeGitRootFinderMap(gitRoots map[string]string) GitRootFinder {
	return func(startDir string) (string, error) {
		startDir, err := filepath.Abs(startDir)
		if err != nil {
			return "", err
		}
		startDir = filepath.Clean(startDir)
		if gitRoot, ok := gitRoots[startDir]; ok {
			return gitRoot, nil
		}
		return "", ErrNotGitRepository
	}
}

func newUtreeProject(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	mkdir(t, filepath.Join(projectRoot, ".utree"))
	return projectRoot
}

func mkdir(t *testing.T, path string) string {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs %s: %v", path, err)
	}
	return filepath.Clean(absPath)
}
