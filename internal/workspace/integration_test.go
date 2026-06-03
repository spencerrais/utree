package workspace

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
)

func TestIntegrationNewWorktreeCreationWithRealGitAndFakeTmux(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	deps := &fakeNewExecutionDeps{insideTmux: true}

	plan, err := PlanNew(mainRoot, NewOptions{WorktreeName: "feature-a"}, NewDependencies{})
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}
	if plan.TargetPath != filepath.Join(projectRoot, "feature-a") {
		t.Fatalf("expected sibling target under project root, got %q", plan.TargetPath)
	}
	if plan.StartPoint != "main" || plan.DefaultBranch != "main" {
		t.Fatalf("expected default/start main, got default %q start %q", plan.DefaultBranch, plan.StartPoint)
	}

	executed, err := ExecuteNew(plan, deps, nil, io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected new worktree execution")
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "feature-a", ".git")); err != nil {
		t.Fatalf("expected created worktree .git file: %v", err)
	}
	runGit(t, mainRoot, "show-ref", "--verify", "--quiet", "refs/heads/feature-a")

	want := []string{
		"has:" + filepath.Base(projectRoot) + "--feature-a",
		"layout:" + filepath.Base(projectRoot) + "--feature-a:" + filepath.Join(projectRoot, "feature-a") + ":nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
		"open:" + filepath.Base(projectRoot) + "--feature-a:true",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected tmux calls %v, got %v", want, deps.calls)
	}
}

func TestIntegrationNewCopiesEnvFileAfterPrompt(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	if err := os.WriteFile(filepath.Join(mainRoot, ".env"), []byte("TOKEN=secret\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	deps := &fakeNewExecutionDeps{insideTmux: true}

	plan, err := PlanNew(mainRoot, NewOptions{WorktreeName: "feature-env"}, NewDependencies{})
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}
	if !plan.HasEnvFile || plan.EnvFilePath != filepath.Join(mainRoot, ".env") {
		t.Fatalf("expected env source in plan, got %+v", plan)
	}

	executed, err := ExecuteNew(plan, deps, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected new worktree execution")
	}
	contents, err := os.ReadFile(filepath.Join(projectRoot, "feature-env", ".env"))
	if err != nil {
		t.Fatalf("read copied .env: %v", err)
	}
	if string(contents) != "TOKEN=secret\n" {
		t.Fatalf("expected copied .env contents, got %q", string(contents))
	}
}

func TestIntegrationNewDetachCreatesLayoutWithoutOpeningSession(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	deps := &fakeNewExecutionDeps{insideTmux: true}

	plan, err := PlanNew(mainRoot, NewOptions{WorktreeName: "feature-detached", Detach: true}, NewDependencies{})
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}
	if !plan.Detach {
		t.Fatal("expected detach flag in plan")
	}

	executed, err := ExecuteNew(plan, deps, nil, io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected new worktree execution")
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "feature-detached", ".git")); err != nil {
		t.Fatalf("expected created worktree .git file: %v", err)
	}
	want := []string{
		"has:" + filepath.Base(projectRoot) + "--feature-detached",
		"layout:" + filepath.Base(projectRoot) + "--feature-detached:" + filepath.Join(projectRoot, "feature-detached") + ":nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected tmux calls %v, got %v", want, deps.calls)
	}
}

func TestIntegrationOpenSessionPlanningUsesRealGitWorktreeListAndFakeTmux(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	runGit(t, mainRoot, "worktree", "add", "-b", "feature-open", filepath.Join(projectRoot, "feature-open"), "main")

	plan, err := PlanOpen(mainRoot, OpenOptions{Target: "feature-open"}, OpenDependencies{})
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}
	if plan.WorktreePath != filepath.Join(projectRoot, "feature-open") {
		t.Fatalf("expected direct-child worktree path, got %q", plan.WorktreePath)
	}
	if plan.SessionName != filepath.Base(projectRoot)+"--feature-open" {
		t.Fatalf("expected default session name, got %q", plan.SessionName)
	}

	deps := &fakeOpenExecuteDeps{insideTmux: false}
	if err := ExecuteOpen(plan, deps); err != nil {
		t.Fatalf("ExecuteOpen returned error: %v", err)
	}
	assertContains(t, strings.Join(deps.calls, "\n"), "layout:"+plan.SessionName+":"+plan.WorktreePath)
	assertContains(t, strings.Join(deps.calls, "\n"), "open:"+plan.SessionName+":false")
}

func TestIntegrationOpenNamedWorktreeFromProjectRoot(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)

	plan, err := PlanOpen(projectRoot, OpenOptions{Target: "main"}, OpenDependencies{})
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}
	if plan.WorktreePath != mainRoot {
		t.Fatalf("expected main worktree path, got %q", plan.WorktreePath)
	}
	if plan.SessionName != filepath.Base(projectRoot)+"--main" {
		t.Fatalf("expected main session name, got %q", plan.SessionName)
	}
}

func TestIntegrationOpenWithoutTargetFromProjectRootFailsClearly(t *testing.T) {
	projectRoot, _ := newGitProject(t)

	_, err := PlanOpen(projectRoot, OpenOptions{}, OpenDependencies{})
	if !errors.Is(err, ErrOpenTargetRequired) {
		t.Fatalf("expected ErrOpenTargetRequired, got %v", err)
	}
}

func TestIntegrationListClassifiesProjectAndOtherWorktreesWithRealGit(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	projectFeature := filepath.Join(projectRoot, "feature-list")
	otherFeature := filepath.Join(t.TempDir(), "scratch")
	runGit(t, mainRoot, "worktree", "add", "-b", "feature-list", projectFeature, "main")
	runGit(t, mainRoot, "worktree", "add", "-b", "scratch", otherFeature, "main")

	plan, err := PlanList(mainRoot, ListDependencies{HasSession: fakeListSessions(map[string]bool{filepath.Base(projectRoot) + ":feature-list": true})})
	if err != nil {
		t.Fatalf("PlanList returned error: %v", err)
	}
	if len(plan.UtreeWorktrees) != 2 {
		t.Fatalf("expected main and feature-list as utree worktrees, got %+v", plan.UtreeWorktrees)
	}
	if len(plan.OtherWorktrees) != 1 || plan.OtherWorktrees[0].Path != otherFeature {
		t.Fatalf("expected scratch as other worktree, got %+v", plan.OtherWorktrees)
	}

	output := RenderList(plan)
	assertContains(t, output, "utree worktrees:")
	assertContains(t, output, "other git worktrees:")
	assertContains(t, output, "feature-list")
	assertContains(t, output, otherFeature)
}

func TestIntegrationListFromProjectRootWithRealGit(t *testing.T) {
	projectRoot, _ := newGitProject(t)

	plan, err := PlanList(projectRoot, ListDependencies{HasSession: fakeListSessions(nil)})
	if err != nil {
		t.Fatalf("PlanList returned error: %v", err)
	}
	if len(plan.UtreeWorktrees) != 1 || plan.UtreeWorktrees[0].Name != "main" {
		t.Fatalf("expected main worktree from project root, got %+v", plan.UtreeWorktrees)
	}
}

func TestIntegrationRemoveRefusesDirtyWorktreeWithRealGit(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	dirtyPath := filepath.Join(projectRoot, "dirty")
	runGit(t, mainRoot, "worktree", "add", "-b", "dirty", dirtyPath, "main")
	if err := os.WriteFile(filepath.Join(dirtyPath, "scratch.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write dirty file: %v", err)
	}

	_, err := PlanRemove(mainRoot, RemoveOptions{WorktreeName: "dirty"}, nil)
	if err == nil {
		t.Fatal("expected dirty worktree refusal")
	}
	assertContains(t, err.Error(), "local changes")
	assertContains(t, err.Error(), "untracked files   yes")
}

func TestIntegrationRemoveCurrentCleanWorktreeDeletesBranchWithRealGit(t *testing.T) {
	projectRoot, mainRoot := newGitProject(t)
	featurePath := filepath.Join(projectRoot, "done")
	runGit(t, mainRoot, "worktree", "add", "-b", "done", featurePath, "main")

	plan, err := PlanRemove(featurePath, RemoveOptions{WorktreeName: "done"}, nil)
	if err != nil {
		t.Fatalf("PlanRemove returned error: %v", err)
	}
	removed, err := ExecuteRemove(plan, nil, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	if !removed {
		t.Fatal("expected worktree removal")
	}
	if _, err := os.Stat(featurePath); !os.IsNotExist(err) {
		t.Fatalf("expected feature worktree to be removed, stat err %v", err)
	}
	output := runGit(t, mainRoot, "branch", "--list", "done")
	if strings.TrimSpace(output) != "" {
		t.Fatalf("expected local branch to be deleted, got %q", output)
	}
}

func newGitProject(t *testing.T) (string, string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	projectRoot := t.TempDir()
	mainRoot := filepath.Join(projectRoot, "main")
	if err := os.Mkdir(mainRoot, 0o755); err != nil {
		t.Fatalf("create main worktree: %v", err)
	}
	if err := os.Mkdir(filepath.Join(projectRoot, ".utree"), 0o755); err != nil {
		t.Fatalf("create .utree: %v", err)
	}
	runGit(t, mainRoot, "init", "-b", "main")
	runGit(t, mainRoot, "config", "user.email", "test@example.com")
	runGit(t, mainRoot, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(mainRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, mainRoot, "add", "README.md")
	runGit(t, mainRoot, "commit", "-m", "initial")
	return projectRoot, mainRoot
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := git.Command(dir, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

type fakeNewExecutionDeps struct {
	calls      []string
	insideTmux bool
}

func (v *fakeNewExecutionDeps) WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error {
	return git.Adapter{Dir: dir}.WorktreeAddNewBranch(path, branch, startPoint)
}

func (v *fakeNewExecutionDeps) CopyFile(source string, target string) error {
	v.calls = append(v.calls, "copy:"+source+":"+target)
	return copyFile(source, target)
}

func (v *fakeNewExecutionDeps) HasSession(session string) (bool, error) {
	v.calls = append(v.calls, "has:"+session)
	return false, nil
}

func (v *fakeNewExecutionDeps) CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error {
	v.calls = append(v.calls, "layout:"+session+":"+worktreePath+":"+formatTestLayout(layout))
	return nil
}

func (v *fakeNewExecutionDeps) OpenSession(session string, insideTmux bool) error {
	v.calls = append(v.calls, "open:"+session+":"+boolString(insideTmux))
	return nil
}

func (v *fakeNewExecutionDeps) InsideTmux() bool {
	return v.insideTmux
}
