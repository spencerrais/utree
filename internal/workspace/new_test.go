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
	"github.com/spencerrais/utree/internal/git"
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

func TestPlanNewUsesExistingLocalBranchWhenPresent(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	deps := fakePlanDeps(projectRoot, gitRoot, "main", "main")
	deps.LocalBranchExists = func(dir string, branch string) (bool, error) {
		if dir != gitRoot || branch != "feature-a" {
			t.Fatalf("unexpected branch existence check %q %q", dir, branch)
		}
		return true, nil
	}

	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "feature-a"}, deps)
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if !plan.UseExistingBranch {
		t.Fatal("expected existing branch mode")
	}
	if plan.StartPoint != "main" {
		t.Fatalf("expected start point to remain main, got %q", plan.StartPoint)
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

func TestPlanNewDetectsEnvFileFromCurrentWorktree(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	writeWorkspaceFile(t, filepath.Join(gitRoot, ".env"), "TOKEN=source\n")

	plan, err := PlanNew(gitRoot, NewOptions{WorktreeName: "feature-a"}, fakePlanDeps(projectRoot, gitRoot, "main", "main"))
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if !plan.HasEnvFile {
		t.Fatal("expected env file detection")
	}
	if plan.EnvFilePath != filepath.Join(gitRoot, ".env") || plan.EnvFileWorktreeName != "main" {
		t.Fatalf("unexpected env source: %+v", plan)
	}
}

func TestPlanNewFromProjectRootDetectsEnvFileFromDefaultBranchWorktree(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	featureRoot := mkdirWorkspace(t, filepath.Join(projectRoot, "feature-a"))
	writeWorkspaceFile(t, filepath.Join(gitRoot, ".env"), "TOKEN=main\n")
	deps := fakePlanDeps(projectRoot, gitRoot, "main", "main")
	deps.DetectProject = func(string) (project.Project, error) {
		return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: ""}, nil
	}
	deps.Worktrees = func(string) ([]git.Worktree, error) {
		return []git.Worktree{
			{Path: gitRoot, Branch: "main", Name: "main"},
			{Path: featureRoot, Branch: "feature-a", Name: "feature-a"},
		}, nil
	}

	plan, err := PlanNew(projectRoot, NewOptions{WorktreeName: "bugfix-b"}, deps)
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if !plan.HasEnvFile || plan.EnvFilePath != filepath.Join(gitRoot, ".env") || plan.EnvFileWorktreeName != "main" {
		t.Fatalf("unexpected env source: %+v", plan)
	}
}

func TestPlanNewFromProjectRootDetectsEnvFromDetectedGitRootWhenDefaultBranchWorktreeMissing(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "feature-a")
	writeWorkspaceFile(t, filepath.Join(gitRoot, ".env"), "TOKEN=feature\n")
	deps := fakePlanDeps(projectRoot, gitRoot, "main", "feature-a")
	deps.DetectProject = func(string) (project.Project, error) {
		return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: ""}, nil
	}
	deps.Worktrees = func(string) ([]git.Worktree, error) {
		return []git.Worktree{{Path: gitRoot, Branch: "feature-a", Name: "feature-a"}}, nil
	}

	plan, err := PlanNew(projectRoot, NewOptions{WorktreeName: "bugfix-b"}, deps)
	if err != nil {
		t.Fatalf("PlanNew returned error: %v", err)
	}

	if !plan.HasEnvFile || plan.EnvFilePath != filepath.Join(gitRoot, ".env") || plan.EnvFileWorktreeName != "feature-a" {
		t.Fatalf("unexpected env source: %+v", plan)
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

func TestExecuteNewUsesExistingBranchWithoutCreatingBranch(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", UseExistingBranch: true, Config: config.Default()}
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
		"git-existing:/repo/main:/repo/feature-a:feature-a",
		"layout:repo--feature-a:/repo/feature-a:nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
		"open:repo--feature-a:true",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteNewDeclinesEnvCopyAndStillOpensTmux(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", HasEnvFile: true, EnvFilePath: "/repo/main/.env", EnvFileWorktreeName: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{insideTmux: true}
	var stdout bytes.Buffer

	executed, err := ExecuteNew(plan, deps, strings.NewReader("n\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}
	assertContains(t, stdout.String(), "Copy .env from main to new worktree? [y/N]")
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

func TestExecuteNewAcceptsEnvCopyBeforeOpeningTmux(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", HasEnvFile: true, EnvFilePath: "/repo/main/.env", EnvFileWorktreeName: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{insideTmux: true}

	executed, err := ExecuteNew(plan, deps, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}
	want := []string{
		"has:repo--feature-a",
		"git:/repo/main:/repo/feature-a:feature-a:main",
		"copy:/repo/main/.env:/repo/feature-a/.env",
		"layout:repo--feature-a:/repo/feature-a:nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
		"open:repo--feature-a:true",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteNewDetachCreatesLayoutWithoutOpeningTmux(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", Detach: true, Config: config.Default()}
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
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteNewDetachStillCopiesEnvBeforeLayout(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "main", StartPoint: "main", Detach: true, HasEnvFile: true, EnvFilePath: "/repo/main/.env", EnvFileWorktreeName: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{insideTmux: true}

	executed, err := ExecuteNew(plan, deps, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}
	want := []string{
		"has:repo--feature-a",
		"git:/repo/main:/repo/feature-a:feature-a:main",
		"copy:/repo/main/.env:/repo/feature-a/.env",
		"layout:repo--feature-a:/repo/feature-a:nvim .; exec ${SHELL:-/bin/sh} -l|selected,vertical:33%:git status; exec ${SHELL:-/bin/sh} -l",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteNewHandlesDefaultBranchWarningAndEnvCopyPromptsInOrder(t *testing.T) {
	plan := NewPlan{Project: project.Project{Root: "/repo", GitRoot: "/repo/main"}, WorktreeName: "feature-a", BranchName: "feature-a", TargetPath: "/repo/feature-a", DefaultBranch: "main", CurrentBranch: "feature/current-task", StartPoint: "main", RequiresDefaultBranchWarning: true, HasEnvFile: true, EnvFilePath: "/repo/main/.env", EnvFileWorktreeName: "main", Config: config.Default()}
	deps := &fakeExecuteDeps{insideTmux: true}
	var stdout bytes.Buffer

	executed, err := ExecuteNew(plan, deps, strings.NewReader("yes\nyes\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteNew returned error: %v", err)
	}
	if !executed {
		t.Fatal("expected execution")
	}
	assertContains(t, stdout.String(), "Current branch is not the detected default branch.")
	assertContains(t, stdout.String(), "Copy .env from main to new worktree? [y/N]")
	assertContains(t, strings.Join(deps.calls, "\n"), "copy:/repo/main/.env:/repo/feature-a/.env")
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
		LoadConfig:        func(string, config.Overrides) (config.Config, error) { return config.Default(), nil },
		DefaultBranch:     func(project.Project, string, *string) (string, error) { return defaultBranch, nil },
		CurrentBranch:     func(string) (string, error) { return currentBranch, nil },
		LocalBranchExists: func(string, string) (bool, error) { return false, nil },
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

func mkdirWorkspace(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create dir %s: %v", path, err)
	}
	return path
}

func writeWorkspaceFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type fakeExecuteDeps struct {
	calls         []string
	gitCalls      int
	tmuxCalls     int
	insideTmux    bool
	tmuxErr       error
	sessionExists bool
}

func (f *fakeExecuteDeps) WorktreeAdd(dir string, path string, branch string) error {
	f.gitCalls++
	f.calls = append(f.calls, "git-existing:"+dir+":"+path+":"+branch)
	return nil
}

func (f *fakeExecuteDeps) WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error {
	f.gitCalls++
	f.calls = append(f.calls, "git:"+dir+":"+path+":"+branch+":"+startPoint)
	return nil
}

func (f *fakeExecuteDeps) CopyFile(source string, target string) error {
	f.calls = append(f.calls, "copy:"+source+":"+target)
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
