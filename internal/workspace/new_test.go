package workspace

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/project"
)

func TestPlanNewOneNameUsesSameWorktreeAndBranch(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "feature-a"}, fakePlanDeps(projectRoot, gitRoot, "main", "main"))
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if plan.WorktreeName != "feature-a" {
		t.Fatalf("expected worktree feature-a, got %q", plan.WorktreeName)
	}
	if plan.BranchName != "feature-a" {
		t.Fatalf("expected branch feature-a, got %q", plan.BranchName)
	}
	if plan.TargetPath != filepath.Join(projectRoot, "feature-a") {
		t.Fatalf("expected target under project root, got %q", plan.TargetPath)
	}
	if plan.DefaultBranch != "main" || plan.StartPoint != "main" {
		t.Fatalf("expected default/start main, got default %q start %q", plan.DefaultBranch, plan.StartPoint)
	}
	if plan.RequiresDefaultBranchWarning {
		t.Fatal("expected no warning when current branch is default branch")
	}
}

func TestPlanNewSeparateWorktreeAndBranchNames(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "firehose-role", BranchName: "tg-123-firehose-role"}, fakePlanDeps(projectRoot, gitRoot, "main", "main"))
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if plan.WorktreeName != "firehose-role" || plan.BranchName != "tg-123-firehose-role" {
		t.Fatalf("expected separate names, got plan %+v", plan)
	}
}

func TestPlanNewBaseOverridesStartPoint(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "feature-current")
	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "bugfix-b", BaseBranch: "feature/current-task"}, fakePlanDeps(projectRoot, gitRoot, "main", "feature/current-task"))
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if plan.StartPoint != "feature/current-task" {
		t.Fatalf("expected start point from --base, got %q", plan.StartPoint)
	}
	if plan.RequiresDefaultBranchWarning {
		t.Fatal("expected no default-branch warning when --base is provided")
	}
}

func TestPlanNewDefaultBranchOverrideWins(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	override := "develop"
	deps := fakePlanDeps(projectRoot, gitRoot, "develop", "develop")
	deps.DefaultBranch = func(_ project.Project, _ string, gotOverride *string) (string, error) {
		if gotOverride == nil || *gotOverride != "develop" {
			t.Fatalf("expected develop override, got %#v", gotOverride)
		}
		return "develop", nil
	}

	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "bugfix-b", DefaultBranchOverride: &override}, deps)
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if plan.DefaultBranch != "develop" || plan.StartPoint != "develop" {
		t.Fatalf("expected override default/start develop, got %+v", plan)
	}
}

func TestPlanNewWarnsWhenCurrentBranchDiffersFromDefaultWithoutBase(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "feature-current")
	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "bugfix-b"}, fakePlanDeps(projectRoot, gitRoot, "main", "feature/current-task"))
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if !plan.RequiresDefaultBranchWarning {
		t.Fatal("expected default branch warning")
	}
	if plan.CurrentBranch != "feature/current-task" || plan.DefaultBranch != "main" || plan.StartPoint != "main" {
		t.Fatalf("expected warning context in plan, got %+v", plan)
	}
}

func TestPlanNewRejectsExistingTargetPathBeforeGit(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	if err := os.Mkdir(filepath.Join(projectRoot, "feature-a"), 0o755); err != nil {
		t.Fatalf("create existing target: %v", err)
	}

	_, err := PlanNew(gitRoot, NewOptions{WorktreeName: "feature-a"}, fakePlanDeps(projectRoot, gitRoot, "main", "main"))
	if !errors.Is(err, project.ErrTargetExists) {
		t.Fatalf("expected ErrTargetExists, got %v", err)
	}
}

func TestExecuteNewDeclineStopsBeforeGitWhenWarningRequired(t *testing.T) {
	plan := NewPlan{WorktreeName: "bugfix-b", BranchName: "bugfix-b", TargetPath: "/repo/bugfix-b", DefaultBranch: "main", CurrentBranch: "feature/current-task", StartPoint: "main", RequiresDefaultBranchWarning: true}
	deps := &fakeExecuteDeps{}
	var stdout bytes.Buffer

	executed, err := ExecuteNew(plan, deps, strings.NewReader("n\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if executed {
		t.Fatal("expected declined execution")
	}
	if deps.gitCalls != 0 || deps.tmuxCalls != 0 {
		t.Fatalf("expected no git/tmux calls, got git %d tmux %d", deps.gitCalls, deps.tmuxCalls)
	}
	assertContains(t, stdout.String(), "Current branch is not the detected default branch.")
	assertContains(t, stdout.String(), "Use `--base feature/current-task`")
}

func TestExecuteNewCreatesGitWorktreeThenOpensTmux(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{insideTmux: true}

	executed, err := ExecuteNew(plan, deps, nil, io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}

	want := []string{
		"has:repo--feature-a",
		"git:/repo/main:/repo/feature-a:feature-a:main",
		"layout:repo--feature-a:/repo/feature-a:nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
		"open:repo--feature-a:true",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteNewIncludesBranchWhenWorktreeNameDiffers(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "temp", BranchName: "chore/test", TargetPath: "/repo/temp", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{insideTmux: true}

	executed, err := ExecuteNew(plan, deps, nil, io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}

	want := []string{
		"has:repo--temp--chore-test",
		"git:/repo/main:/repo/temp:chore/test:main",
		"layout:repo--temp--chore-test:/repo/temp:nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
		"open:repo--temp--chore-test:true",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteNewRejectsExistingTmuxSessionBeforeCreatingWorktree(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{sessionExists: true}

	_, err := ExecuteNew(plan, deps, nil, io.Discard)
	if err == nil {
		t.Fatal("expected existing session error")
	}
	assertContains(t, err.Error(), "tmux session already exists")
	want := []string{"has:repo--feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
	if deps.gitCalls != 0 {
		t.Fatalf("expected no git worktree creation, got %d git calls", deps.gitCalls)
	}
}

func TestExecuteNewKeepsCreatedWorktreeWhenTmuxOpenFails(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", StartPoint: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{tmuxErr: errors.New("tmux unavailable")}

	_, err := ExecuteNew(plan, deps, nil, io.Discard)
	if err == nil {
		t.Fatal("expected tmux failure")
	}
	assertContains(t, err.Error(), "worktree created")
	assertContains(t, err.Error(), "tmux open failed")
	for _, call := range deps.calls {
		if strings.HasPrefix(call, "remove:") {
			t.Fatalf("expected no rollback removal, got calls %v", deps.calls)
		}
	}
}

func fakePlanDeps(projectRoot string, gitRoot string, defaultBranch string, currentBranch string) NewDependencies {
	return NewDependencies{
		DetectProject: func(startDir string) (project.Project, error) {
			return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: filepath.Base(gitRoot)}, nil
		},
		LoadConfig:    func(string, config.Overrides) (config.Config, error) { return config.Default(), nil },
		DefaultBranch: func(project.Project, string, *string) (string, error) { return defaultBranch, nil },
		CurrentBranch: func(string) (string, error) { return currentBranch, nil },
	}
}

func newWorkspaceProject(t *testing.T, currentWorktree string) (string, string) {
	t.Helper()

	projectRoot := t.TempDir()
	utreeDir := filepath.Join(projectRoot, ".utree")
	if err := os.Mkdir(utreeDir, 0o755); err != nil {
		t.Fatalf("create .utree: %v", err)
	}
	gitRoot := filepath.Join(projectRoot, currentWorktree)
	if err := os.Mkdir(gitRoot, 0o755); err != nil {
		t.Fatalf("create git root: %v", err)
	}
	return projectRoot, gitRoot
}

type fakeExecuteDeps struct {
	calls         []string
	gitCalls      int
	tmuxCalls     int
	insideTmux    bool
	tmuxErr       error
	sessionExists bool
}

func (f *fakeExecuteDeps) WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error {
	f.gitCalls++
	f.calls = append(f.calls, "git:"+dir+":"+path+":"+branch+":"+startPoint)
	return nil
}

func (f *fakeExecuteDeps) HasSession(session string) (bool, error) {
	f.tmuxCalls++
	f.calls = append(f.calls, "has:"+session)
	return f.sessionExists, nil
}

func (f *fakeExecuteDeps) CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error {
	f.tmuxCalls++
	f.calls = append(f.calls, "layout:"+session+":"+worktreePath+":"+formatTestLayout(layout))
	return f.tmuxErr
}

func (f *fakeExecuteDeps) OpenSession(session string, insideTmux bool) error {
	f.tmuxCalls++
	f.calls = append(f.calls, "open:"+session+":"+boolString(insideTmux))
	return f.tmuxErr
}

func (f *fakeExecuteDeps) InsideTmux() bool {
	return f.insideTmux
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func formatTestLayout(layout config.DefaultLayoutConfig) string {
	parts := make([]string, 0, len(layout.Panes))
	for _, pane := range layout.Panes {
		part := pane.Command
		if pane.Split != "" || pane.Size != "" {
			part = pane.Split + ":" + pane.Size + ":" + pane.Command
		}
		if pane.Selected {
			part += "|selected"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ",")
}

func assertContains(t *testing.T, value string, want string) {
	t.Helper()

	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}
