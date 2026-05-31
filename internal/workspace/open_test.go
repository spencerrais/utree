package workspace

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
)

func TestPlanOpenCurrentWorktreeForNoArg(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "feature-a")
	plan, err := PlanOpen(gitRoot, OpenOptions{}, fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
	}))
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}

	if plan.WorktreeName != "feature-a" {
		t.Fatalf("expected feature-a, got %q", plan.WorktreeName)
	}
	if plan.WorktreePath != filepath.Join(projectRoot, "feature-a") {
		t.Fatalf("expected current worktree path, got %q", plan.WorktreePath)
	}
	if plan.SessionName != filepath.Base(projectRoot)+"--feature-a" {
		t.Fatalf("expected default session name, got %q", plan.SessionName)
	}
}

func TestPlanOpenCurrentWorktreeForDot(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "feature-a")
	plan, err := PlanOpen(gitRoot, OpenOptions{Target: "."}, fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
	}))
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}

	if plan.WorktreeName != "feature-a" {
		t.Fatalf("expected dot to resolve current worktree, got %q", plan.WorktreeName)
	}
}

func TestPlanOpenRequiresTargetFromProjectRoot(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	deps := fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{{Path: gitRoot, Name: "main", Branch: "main"}})
	deps.DetectProject = func(string) (project.Project, error) {
		return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: ""}, nil
	}

	_, err := PlanOpen(projectRoot, OpenOptions{}, deps)
	if !errors.Is(err, ErrOpenTargetRequired) {
		t.Fatalf("expected ErrOpenTargetRequired, got %v", err)
	}
}

func TestPlanOpenNamedWorktreeFromProjectRoot(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	deps := fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{{Path: gitRoot, Name: "main", Branch: "main"}})
	deps.DetectProject = func(string) (project.Project, error) {
		return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: ""}, nil
	}

	plan, err := PlanOpen(projectRoot, OpenOptions{Target: "main"}, deps)
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}
	if plan.WorktreeName != "main" {
		t.Fatalf("expected main worktree, got %q", plan.WorktreeName)
	}
}

func TestPlanOpenNamedDirectChildWorktree(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	cfg := config.Default()
	cfg.Project.Name = "infra"
	cfg.Session.NameTemplate = "{project}__{worktree}"
	deps := fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
		{Path: filepath.Join(projectRoot, "bugfix-b"), Name: "bugfix-b", Branch: "bugfix-b"},
	})
	deps.LoadConfig = func(string, config.Overrides) (config.Config, error) { return cfg, nil }

	plan, err := PlanOpen(gitRoot, OpenOptions{Target: "bugfix-b"}, deps)
	if err != nil {
		t.Fatalf("PlanOpen returned error: %v", err)
	}

	if plan.WorktreePath != filepath.Join(projectRoot, "bugfix-b") {
		t.Fatalf("expected named worktree path, got %q", plan.WorktreePath)
	}
	if plan.SessionName != "infra__bugfix-b" {
		t.Fatalf("expected configured session name, got %q", plan.SessionName)
	}
}

func TestPlanOpenRejectsUnknownWorktreeName(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	_, err := PlanOpen(gitRoot, OpenOptions{Target: "missing"}, fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
	}))
	if err == nil {
		t.Fatal("expected missing worktree error")
	}
	assertContains(t, err.Error(), "missing")
}

func TestPlanOpenRejectsUnsafeOrArbitraryPathTargets(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	deps := fakeOpenPlanDeps(projectRoot, gitRoot, nil)
	for _, target := range []string{"/tmp/foo", "../escape", "nested/name", "~/scratch", ".."} {
		t.Run(target, func(t *testing.T) {
			_, err := PlanOpen(gitRoot, OpenOptions{Target: target}, deps)
			if err == nil {
				t.Fatal("expected invalid target error")
			}
		})
	}
}

func TestPlanOpenRejectsWorktreeOutsideProjectRoot(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	_, err := PlanOpen(gitRoot, OpenOptions{Target: "scratch"}, fakeOpenPlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
		{Path: filepath.Join(t.TempDir(), "scratch"), Name: "scratch", Branch: "scratch"},
	}))
	if err == nil {
		t.Fatal("expected outside project worktree to be refused")
	}
}

func TestExecuteOpenUsesExistingSessionWithoutCreatingLayout(t *testing.T) {
	plan := OpenPlan{SessionName: "project:feature-a", WorktreePath: "/repo/feature-a", Config: config.Default()}
	deps := &fakeOpenExecuteDeps{sessionExists: true, insideTmux: true}

	if err := ExecuteOpen(plan, deps); err != nil {
		t.Fatalf("ExecuteOpen returned error: %v", err)
	}

	want := []string{"has:project:feature-a", "open:project:feature-a:true"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteOpenCreatesLayoutThenOpensWhenSessionMissing(t *testing.T) {
	cfg := config.Default()
	cfg.Layout.Default.Panes = []config.PaneConfig{
		{Command: "nvim .", Selected: true},
		{Command: "git status --short; exec ${SHELL:-/bin/sh} -l", Split: config.SplitVertical, Size: "40%"},
	}
	plan := OpenPlan{SessionName: "project:feature-a", WorktreePath: "/repo/feature-a", Config: cfg}
	deps := &fakeOpenExecuteDeps{insideTmux: false}

	if err := ExecuteOpen(plan, deps); err != nil {
		t.Fatalf("ExecuteOpen returned error: %v", err)
	}

	want := []string{
		"has:project:feature-a",
		"layout:project:feature-a:/repo/feature-a:nvim .|selected,vertical:40%:git status --short; exec ${SHELL:-/bin/sh} -l",
		"open:project:feature-a:false",
	}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func fakeOpenPlanDeps(projectRoot string, gitRoot string, worktrees []git.Worktree) OpenDependencies {
	return OpenDependencies{
		DetectProject: func(string) (project.Project, error) {
			return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: filepath.Base(gitRoot)}, nil
		},
		LoadConfig: func(string, config.Overrides) (config.Config, error) { return config.Default(), nil },
		Worktrees:  func(string) ([]git.Worktree, error) { return worktrees, nil },
	}
}

type fakeOpenExecuteDeps struct {
	calls         []string
	sessionExists bool
	insideTmux    bool
	err           error
}

func (f *fakeOpenExecuteDeps) HasSession(session string) (bool, error) {
	f.calls = append(f.calls, "has:"+session)
	return f.sessionExists, f.err
}

func (f *fakeOpenExecuteDeps) CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error {
	f.calls = append(f.calls, "layout:"+session+":"+worktreePath+":"+formatTestLayout(layout))
	return f.err
}

func (f *fakeOpenExecuteDeps) OpenSession(session string, insideTmux bool) error {
	f.calls = append(f.calls, "open:"+session+":"+boolString(insideTmux))
	return f.err
}

func (f *fakeOpenExecuteDeps) InsideTmux() bool {
	return f.insideTmux
}
