package convert

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/git"
)

func TestPlanRequiresRepositoryRoot(t *testing.T) {
	repoRoot := t.TempDir()
	startDir := mkdir(t, filepath.Join(repoRoot, "subdir"))
	deps := cleanPlanDeps(repoRoot)

	_, err := PlanConversion(startDir, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrNotRepositoryRoot) {
		t.Fatalf("expected ErrNotRepositoryRoot, got %v", err)
	}
}

func TestPlanFallsBackToLocalMainWhenOriginHeadUnavailable(t *testing.T) {
	repoRoot := t.TempDir()
	deps := cleanPlanDeps(repoRoot)
	deps.DefaultBranch = nil
	deps.ConfigDefaultBranch = func(string) (string, error) { return "auto", nil }
	deps.BranchSource = func(string) git.BranchSource {
		return branchSource{originHeadErr: errors.New("origin head missing"), localBranches: map[string]bool{"main": true}}
	}

	plan, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if err != nil {
		t.Fatalf("PlanConversion returned error: %v", err)
	}
	if plan.DefaultBranch != "main" {
		t.Fatalf("expected fallback default branch main, got %q", plan.DefaultBranch)
	}
}

func TestPlanFailsWhenCurrentBranchDiffersFromDefaultBranch(t *testing.T) {
	repoRoot := t.TempDir()
	deps := cleanPlanDeps(repoRoot)
	deps.CurrentBranch = func(string) (string, error) { return "feature-a", nil }

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrCurrentBranchNotDefault) {
		t.Fatalf("expected ErrCurrentBranchNotDefault, got %v", err)
	}
}

func TestPlanFailsForInvalidPrimaryWorktreeName(t *testing.T) {
	for _, branch := range []string{".", "..", "../outside", "/tmp/outside", "feature/foo"} {
		t.Run(fmt.Sprintf("branch_%q", branch), func(t *testing.T) {
			repoRoot := t.TempDir()
			deps := cleanPlanDeps(repoRoot)
			deps.DefaultBranch = func(string, *string) (string, error) { return branch, nil }
			deps.CurrentBranch = func(string) (string, error) { return branch, nil }

			_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
			if !errors.Is(err, ErrInvalidPrimaryWorktreeName) {
				t.Fatalf("expected ErrInvalidPrimaryWorktreeName, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(repoRoot, ".utree")); !os.IsNotExist(err) {
				t.Fatalf("expected invalid plan not to create .utree, stat err %v", err)
			}
		})
	}
}

func TestPlanUsesConfiguredDefaultBranchBeforeOriginHead(t *testing.T) {
	repoRoot := t.TempDir()
	deps := cleanPlanDeps(repoRoot)
	deps.ConfigDefaultBranch = func(string) (string, error) { return "trunk", nil }
	deps.DefaultBranch = nil
	deps.CurrentBranch = func(string) (string, error) { return "trunk", nil }
	deps.BranchSource = func(string) git.BranchSource {
		return branchSource{originHead: "origin/main"}
	}

	plan, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if err != nil {
		t.Fatalf("PlanConversion returned error: %v", err)
	}
	if plan.DefaultBranch != "trunk" {
		t.Fatalf("expected configured trunk branch, got %q", plan.DefaultBranch)
	}
}

func TestPlanFailsForExistingTargetPrimaryDirectory(t *testing.T) {
	repoRoot := t.TempDir()
	mkdir(t, filepath.Join(repoRoot, "main"))
	deps := cleanPlanDeps(repoRoot)

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrPrimaryTargetExists) {
		t.Fatalf("expected ErrPrimaryTargetExists, got %v", err)
	}
}

func TestPlanFailsWhenUtreeDirectoryAlreadyExists(t *testing.T) {
	repoRoot := t.TempDir()
	mkdir(t, filepath.Join(repoRoot, ".utree"))
	deps := cleanPlanDeps(repoRoot)

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrUtreeAlreadyExists) {
		t.Fatalf("expected ErrUtreeAlreadyExists, got %v", err)
	}
}

func TestPlanFailsWhenUtreeMarkerPathAlreadyExistsAsFile(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, ".utree"), "not a directory\n")
	deps := cleanPlanDeps(repoRoot)

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrUtreeAlreadyExists) {
		t.Fatalf("expected ErrUtreeAlreadyExists, got %v", err)
	}
}

func TestPlanFailsInsideExistingUtreeProject(t *testing.T) {
	projectRoot := t.TempDir()
	mkdir(t, filepath.Join(projectRoot, ".utree"))
	repoRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	deps := cleanPlanDeps(repoRoot)

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrAlreadyUtreeProject) {
		t.Fatalf("expected ErrAlreadyUtreeProject, got %v", err)
	}
}

func TestPlanFailsForLinkedWorktrees(t *testing.T) {
	repoRoot := t.TempDir()
	deps := cleanPlanDeps(repoRoot)
	deps.Worktrees = func(string) ([]git.Worktree, error) {
		return []git.Worktree{
			{Path: repoRoot, Branch: "main", Name: filepath.Base(repoRoot)},
			{Path: filepath.Join(filepath.Dir(repoRoot), "feature-a"), Branch: "feature-a", Name: "feature-a"},
		}, nil
	}

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrLinkedWorktrees) {
		t.Fatalf("expected ErrLinkedWorktrees, got %v", err)
	}
}

func TestPlanFailsForSubmodules(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, ".gitmodules"), "[submodule \"lib\"]\n")
	deps := cleanPlanDeps(repoRoot)

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if !errors.Is(err, ErrSubmodules) {
		t.Fatalf("expected ErrSubmodules, got %v", err)
	}
}

func TestPlanFailsForDirtyRepository(t *testing.T) {
	repoRoot := t.TempDir()
	for _, status := range []string{" M README.md\n", "M  README.md\n", "?? scratch.txt\n"} {
		t.Run(strings.TrimSpace(status), func(t *testing.T) {
			deps := cleanPlanDeps(repoRoot)
			deps.StatusPorcelain = func(string) (string, error) { return status, nil }

			_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
			if !errors.Is(err, ErrDirtyRepository) {
				t.Fatalf("expected ErrDirtyRepository, got %v", err)
			}
		})
	}
}

func TestPlanAllowsIgnoredFiles(t *testing.T) {
	repoRoot := t.TempDir()
	deps := cleanPlanDeps(repoRoot)
	deps.StatusPorcelain = func(string) (string, error) { return "!! ignored.log\n", nil }

	_, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if err != nil {
		t.Fatalf("PlanConversion returned error: %v", err)
	}
}

func TestPlanBuildsPreviewWithoutMutation(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	deps := cleanPlanDeps(repoRoot)

	plan, err := PlanConversion(repoRoot, Options{}, deps.Dependencies)
	if err != nil {
		t.Fatalf("PlanConversion returned error: %v", err)
	}

	if plan.From != repoRoot {
		t.Fatalf("expected From %q, got %q", repoRoot, plan.From)
	}
	if plan.PrimaryTarget != filepath.Join(repoRoot, "main") {
		t.Fatalf("expected primary target under main, got %q", plan.PrimaryTarget)
	}
	if plan.MarkerPath != filepath.Join(repoRoot, ".utree") {
		t.Fatalf("expected marker path, got %q", plan.MarkerPath)
	}
	if plan.DefaultBranch != "main" {
		t.Fatalf("expected main default branch, got %q", plan.DefaultBranch)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "README.md")); err != nil {
		t.Fatalf("expected planning not to move README: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree")); !os.IsNotExist(err) {
		t.Fatalf("expected planning not to create .utree, stat err %v", err)
	}
}

func TestExecuteDeclineDoesNotMutate(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	plan := Plan{From: repoRoot, PrimaryTarget: filepath.Join(repoRoot, "main"), MarkerPath: filepath.Join(repoRoot, ".utree"), DefaultBranch: "main"}

	executed, err := Execute(plan, strings.NewReader("n\n"))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if executed {
		t.Fatal("expected decline not to execute")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "README.md")); err != nil {
		t.Fatalf("expected README to remain in place: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree")); !os.IsNotExist(err) {
		t.Fatalf("expected .utree absent after decline, stat err %v", err)
	}
}

func TestExecuteCleansCreatedUtreeDirectoryWhenPrimaryCreationFails(t *testing.T) {
	repoRoot := t.TempDir()
	writeFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	plan := Plan{From: repoRoot, PrimaryTarget: filepath.Join(repoRoot, "missing", "main"), MarkerPath: filepath.Join(repoRoot, ".utree"), DefaultBranch: "main"}

	executed, err := Execute(plan, strings.NewReader("yes\n"))
	if err == nil {
		t.Fatal("expected primary creation error")
	}
	if executed {
		t.Fatal("expected failed execution")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree")); !os.IsNotExist(err) {
		t.Fatalf("expected .utree cleanup after failed primary creation, stat err %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "README.md")); err != nil {
		t.Fatalf("expected README to remain in place: %v", err)
	}
}

func TestExecuteCreatesMarkerAndMovesRepositoryContents(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init", "-b", "main")
	runGit(t, repoRoot, "config", "user.email", "test@example.com")
	runGit(t, repoRoot, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	writeFile(t, filepath.Join(repoRoot, ".gitignore"), "ignored.log\n")
	runGit(t, repoRoot, "add", "README.md", ".gitignore")
	runGit(t, repoRoot, "commit", "-m", "initial")
	plan := Plan{From: repoRoot, PrimaryTarget: filepath.Join(repoRoot, "main"), MarkerPath: filepath.Join(repoRoot, ".utree"), DefaultBranch: "main"}

	executed, err := Execute(plan, strings.NewReader("yes\n"))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}

	if info, err := os.Stat(filepath.Join(repoRoot, ".utree")); err != nil || !info.IsDir() {
		t.Fatalf("expected .utree marker directory, info %#v err %v", info, err)
	}
	assertFileContains(t, filepath.Join(repoRoot, "main", "README.md"), "hello")
	assertFileContains(t, filepath.Join(repoRoot, "main", ".gitignore"), "ignored.log")
	if _, err := os.Stat(filepath.Join(repoRoot, "main", ".git")); err != nil {
		t.Fatalf("expected .git to move under main: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no config.toml by default, stat err %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree", "metadata.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no metadata.toml by default, stat err %v", err)
	}
}

func TestPlanAdoptionAcceptsExistingSiblingWorktreeLayout(t *testing.T) {
	projectRoot := t.TempDir()
	mainRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	featureRoot := mkdir(t, filepath.Join(projectRoot, "feature-a"))
	deps := cleanPlanDeps(mainRoot)
	deps.Worktrees = func(string) ([]git.Worktree, error) {
		return []git.Worktree{
			{Path: mainRoot, Branch: "main", Name: "main"},
			{Path: featureRoot, Branch: "feature-a", Name: "feature-a"},
		}, nil
	}

	plan, err := PlanAdoption(filepath.Join(mainRoot, "subdir"), deps.Dependencies)
	if err != nil {
		t.Fatalf("PlanAdoption returned error: %v", err)
	}

	if plan.ProjectRoot != projectRoot {
		t.Fatalf("expected project root %q, got %q", projectRoot, plan.ProjectRoot)
	}
	if plan.WorktreeRoot != mainRoot {
		t.Fatalf("expected worktree root %q, got %q", mainRoot, plan.WorktreeRoot)
	}
	if plan.WorktreeName != "main" {
		t.Fatalf("expected worktree name main, got %q", plan.WorktreeName)
	}
	if plan.MarkerPath != filepath.Join(projectRoot, ".utree") {
		t.Fatalf("expected marker path under project root, got %q", plan.MarkerPath)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".utree")); !os.IsNotExist(err) {
		t.Fatalf("expected planning not to create .utree, stat err %v", err)
	}
}

func TestPlanAdoptionRejectsLinkedWorktreeOutsideProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	mainRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	otherRoot := mkdir(t, filepath.Join(t.TempDir(), "feature-a"))
	deps := cleanPlanDeps(mainRoot)
	deps.Worktrees = func(string) ([]git.Worktree, error) {
		return []git.Worktree{{Path: mainRoot, Branch: "main"}, {Path: otherRoot, Branch: "feature-a"}}, nil
	}

	_, err := PlanAdoption(mainRoot, deps.Dependencies)
	if !errors.Is(err, ErrInvalidAdoptLayout) {
		t.Fatalf("expected ErrInvalidAdoptLayout, got %v", err)
	}
}

func TestPlanAdoptionRejectsRepoThatDoesNotMatchBranchName(t *testing.T) {
	repoRoot := mkdir(t, filepath.Join(t.TempDir(), "ordinary-repo"))
	deps := cleanPlanDeps(repoRoot)
	deps.CurrentBranch = func(string) (string, error) { return "main", nil }

	_, err := PlanAdoption(repoRoot, deps.Dependencies)
	if !errors.Is(err, ErrInvalidAdoptLayout) {
		t.Fatalf("expected ErrInvalidAdoptLayout, got %v", err)
	}
}

func TestPlanAdoptionRejectsExistingUtreeDirectory(t *testing.T) {
	projectRoot := t.TempDir()
	mainRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	mkdir(t, filepath.Join(projectRoot, ".utree"))
	deps := cleanPlanDeps(mainRoot)

	_, err := PlanAdoption(mainRoot, deps.Dependencies)
	if !errors.Is(err, ErrAlreadyUtreeProject) {
		t.Fatalf("expected ErrAlreadyUtreeProject, got %v", err)
	}
}

func TestPlanAdoptionRejectsExistingUtreeMarkerPathAsFile(t *testing.T) {
	projectRoot := t.TempDir()
	mainRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	writeFile(t, filepath.Join(projectRoot, ".utree"), "not a directory\n")
	deps := cleanPlanDeps(mainRoot)

	_, err := PlanAdoption(mainRoot, deps.Dependencies)
	if !errors.Is(err, ErrUtreeAlreadyExists) {
		t.Fatalf("expected ErrUtreeAlreadyExists, got %v", err)
	}
}

func TestExecuteAdoptionCreatesMarkerOnly(t *testing.T) {
	projectRoot := t.TempDir()
	mainRoot := mkdir(t, filepath.Join(projectRoot, "main"))
	writeFile(t, filepath.Join(mainRoot, "README.md"), "hello\n")
	plan := AdoptPlan{ProjectRoot: projectRoot, WorktreeRoot: mainRoot, WorktreeName: "main", MarkerPath: filepath.Join(projectRoot, ".utree")}

	executed, err := ExecuteAdoption(plan, strings.NewReader("yes\n"))
	if err != nil {
		t.Fatalf("ExecuteAdoption returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}

	if info, err := os.Stat(filepath.Join(projectRoot, ".utree")); err != nil || !info.IsDir() {
		t.Fatalf("expected .utree marker directory, info %#v err %v", info, err)
	}
	assertFileContains(t, filepath.Join(mainRoot, "README.md"), "hello")
	if _, err := os.Stat(filepath.Join(projectRoot, ".utree", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no config.toml by default, stat err %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".utree", "metadata.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no metadata.toml by default, stat err %v", err)
	}
}

type planDeps struct {
	Dependencies
	localBranchChecks int
}

type branchSource struct {
	originHead    string
	originHeadErr error
	localBranches map[string]bool
}

func (s branchSource) OriginHead() (string, error) {
	if s.originHeadErr != nil {
		return "", s.originHeadErr
	}
	return s.originHead, nil
}

func (s branchSource) LocalBranchExists(name string) (bool, error) {
	return s.localBranches[name], nil
}

func cleanPlanDeps(repoRoot string) *planDeps {
	deps := &planDeps{}
	deps.GitRoot = func(string) (string, error) { return repoRoot, nil }
	deps.DefaultBranch = func(string, *string) (string, error) { return "main", nil }
	deps.CurrentBranch = func(string) (string, error) { return "main", nil }
	deps.Worktrees = func(string) ([]git.Worktree, error) {
		return []git.Worktree{{Path: repoRoot, Branch: "main", Name: filepath.Base(repoRoot)}}, nil
	}
	deps.StatusPorcelain = func(string) (string, error) { return "", nil }
	deps.HasSubmodules = func(string) (bool, error) {
		_, err := os.Stat(filepath.Join(repoRoot, ".gitmodules"))
		if err == nil {
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return deps
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	return path
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(contents), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(contents))
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
