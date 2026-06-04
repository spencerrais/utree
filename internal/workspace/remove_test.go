package workspace

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
)

func TestPlanRemoveFindsNamedProjectWorktreeAndSafety(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	cfg := config.Default()
	cfg.Project.Name = "infra"
	cfg.Session.NameTemplate = "{project}__{worktree}"
	deps := fakeRemovePlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
		{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
	}, RemovalSafety{Kind: SafetyCleanMerged, Branch: "feature-a", DefaultBranch: "main", BranchMerged: true})
	deps.LoadConfig = func(string, config.Overrides) (config.Config, error) { return cfg, nil }

	plan, err := PlanRemove(gitRoot, RemoveOptions{WorktreeName: "feature-a"}, deps)
	if err != nil {
		t.Fatalf("PlanRemove returned error: %v", err)
	}

	if plan.WorktreeName != "feature-a" || plan.WorktreePath != filepath.Join(projectRoot, "feature-a") || plan.BranchName != "feature-a" {
		t.Fatalf("unexpected remove plan: %+v", plan)
	}
	if plan.SessionName != "infra__feature-a" {
		t.Fatalf("expected configured session name, got %q", plan.SessionName)
	}
	if plan.Safety.Kind != SafetyCleanMerged {
		t.Fatalf("expected clean merged safety, got %+v", plan.Safety)
	}
}

func TestPlanRemoveRejectsMissingOrOutsideWorktree(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	outside := t.TempDir()
	deps := fakeRemovePlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
		{Path: filepath.Join(outside, "scratch"), Name: "scratch", Branch: "scratch"},
	}, RemovalSafety{Kind: SafetyCleanMerged})

	for _, name := range []string{"missing", "scratch"} {
		t.Run(name, func(t *testing.T) {
			_, err := PlanRemove(gitRoot, RemoveOptions{WorktreeName: name}, deps)
			if err == nil {
				t.Fatal("expected remove target error")
			}
			assertContains(t, err.Error(), name)
		})
	}
}

func TestPlanRemoveRejectsDirtyWorktreeBeforeExecution(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	_, err := PlanRemove(gitRoot, RemoveOptions{WorktreeName: "feature-a"}, fakeRemovePlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
	}, RemovalSafety{Kind: SafetyDirty, Branch: "feature-a", Status: WorktreeStatus{HasUnstagedChanges: true, HasUntrackedFiles: true}}))
	if err == nil {
		t.Fatal("expected dirty refusal")
	}
	assertContains(t, err.Error(), "cannot remove worktree \"feature-a\"\nworktree has local changes")
	assertContains(t, err.Error(), "local changes")
	assertContains(t, err.Error(), "unstaged changes  yes")
	assertContains(t, err.Error(), "untracked files   yes")
	assertNotContains(t, err.Error(), "  staged changes    yes")
}

func TestPlanRemoveRejectsDetachedOrNoLocalBranch(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	_, err := PlanRemove(gitRoot, RemoveOptions{WorktreeName: "detached"}, fakeRemovePlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "detached"), Name: "detached"},
	}, RemovalSafety{Kind: SafetyNoLocalBranch}))
	if err == nil {
		t.Fatal("expected no local branch refusal")
	}
	assertContains(t, err.Error(), "local branch")
}

func TestPlanRemovePassesDefaultBranchOverrideToSafety(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	override := "develop"
	deps := fakeRemovePlanDeps(projectRoot, gitRoot, []git.Worktree{
		{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
	}, RemovalSafety{Kind: SafetyCleanMerged, Branch: "feature-a", DefaultBranch: "develop", BranchMerged: true})

	_, err := PlanRemove(gitRoot, RemoveOptions{WorktreeName: "feature-a", DefaultBranchOverride: &override}, deps)
	if err != nil {
		t.Fatalf("PlanRemove returned error: %v", err)
	}
	if deps.safetyOptions.DefaultBranchOverride == nil || *deps.safetyOptions.DefaultBranchOverride != "develop" {
		t.Fatalf("expected develop override passed to safety, got %+v", deps.safetyOptions.DefaultBranchOverride)
	}
}

func TestExecuteRemoveCleanMergedPromptsKillsSessionRemovesWorktreeDeletesBranch(t *testing.T) {
	plan := fakeRemovePlan(SafetyCleanMerged)
	deps := &fakeRemoveExecuteDeps{sessionExists: true}
	var stdout bytes.Buffer

	removed, err := ExecuteRemove(plan, deps, strings.NewReader("yes\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	if !removed {
		t.Fatal("expected remove execution")
	}
	want := []string{"has:project:feature-a", "worktree-remove:/repo/feature-a", "branch-delete:feature-a", "kill:project:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
	assertContains(t, stdout.String(), "Remove worktree \"feature-a\" and delete local branch \"feature-a\"? [y/N]")
}

func TestExecuteRemoveCleanMergedSkipsMissingSessionAndStillRemoves(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanMerged), deps, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	want := []string{"has:project:feature-a", "worktree-remove:/repo/feature-a", "branch-delete:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteRemoveCleanMergedDeclineStopsBeforeMutation(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{sessionExists: true}
	var stdout bytes.Buffer

	removed, err := ExecuteRemove(fakeRemovePlan(SafetyCleanMerged), deps, strings.NewReader("n\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	if removed {
		t.Fatal("expected declined removal")
	}
	assertContains(t, stdout.String(), "Remove worktree")
	if len(deps.calls) != 0 {
		t.Fatalf("expected no mutation calls, got %v", deps.calls)
	}
}

func TestExecuteRemoveCleanUnmergedDeclineStopsBeforeMutation(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{sessionExists: true}
	var stdout bytes.Buffer

	removed, err := ExecuteRemove(fakeRemovePlan(SafetyCleanUnmerged), deps, strings.NewReader("n\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	if removed {
		t.Fatal("expected declined removal")
	}
	assertContains(t, stdout.String(), "does not appear to be merged")
	assertContains(t, stdout.String(), "Remove worktree? [y/N]")
	if len(deps.calls) != 0 {
		t.Fatalf("expected no mutation calls, got %v", deps.calls)
	}
}

func TestExecuteRemoveCleanUnmergedConfirmRemovesWorktreeKeepsBranchWhenBranchPromptDeclined(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{sessionExists: true}
	var stdout bytes.Buffer

	removed, err := ExecuteRemove(fakeRemovePlan(SafetyCleanUnmerged), deps, strings.NewReader("yes\nn\n"), &stdout)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	if !removed {
		t.Fatal("expected removal")
	}
	want := []string{"has:project:feature-a", "worktree-remove:/repo/feature-a", "kill:project:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
	assertContains(t, stdout.String(), "Delete unmerged local branch 'feature-a'? [y/N]")
}

func TestExecuteRemoveCleanUnmergedConfirmDeletesBranchWhenSecondPromptConfirmed(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanUnmerged), deps, strings.NewReader("yes\nyes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	want := []string{"has:project:feature-a", "worktree-remove:/repo/feature-a", "branch-force-delete:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteRemoveCleanUnmergedPromptsBeforeSwitchingFromCurrentSession(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{sessionExists: true, insideTmux: true, currentSession: "project:feature-a", adjacentSwitched: true}
	stdout := eventWriter{events: &deps.calls}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanUnmerged), deps, strings.NewReader("yes\nyes\n"), stdout)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}

	want := []string{"prompt:Remove worktree?", "prompt:Delete unmerged local branch", "has:project:feature-a", "current", "adjacent:project:feature-a", "worktree-remove:/repo/feature-a", "branch-force-delete:feature-a", "kill:project:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteRemoveCurrentSessionSwitchesFallbackBeforeKillingTargetSession(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{sessionExists: true, insideTmux: true, currentSession: "project:feature-a", homeDir: "/home/me"}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanMerged), deps, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	want := []string{"has:project:feature-a", "current", "adjacent:project:feature-a", "fallback:utree:/home/me", "worktree-remove:/repo/feature-a", "branch-delete:feature-a", "kill:project:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteRemoveCurrentSessionSwitchesAdjacentBeforeKillingTargetSession(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{sessionExists: true, insideTmux: true, currentSession: "project:feature-a", adjacentSwitched: true}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanMerged), deps, strings.NewReader("yes\n"), io.Discard)
	if err != nil {
		t.Fatalf("ExecuteRemove returned error: %v", err)
	}
	want := []string{"has:project:feature-a", "current", "adjacent:project:feature-a", "worktree-remove:/repo/feature-a", "branch-delete:feature-a", "kill:project:feature-a"}
	if strings.Join(deps.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("expected calls %v, got %v", want, deps.calls)
	}
}

func TestExecuteRemoveBranchDeletionFailureAfterWorktreeRemovalReturnsExplicitError(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{branchErr: errors.New("branch busy")}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanMerged), deps, strings.NewReader("yes\n"), io.Discard)
	if err == nil {
		t.Fatal("expected branch deletion error")
	}
	assertContains(t, err.Error(), "worktree removed")
	assertContains(t, err.Error(), "branch deletion failed")
}

func TestExecuteRemoveWorktreeRemovalFailureStopsBeforeBranchDeletion(t *testing.T) {
	deps := &fakeRemoveExecuteDeps{worktreeErr: errors.New("worktree busy")}

	_, err := ExecuteRemove(fakeRemovePlan(SafetyCleanMerged), deps, strings.NewReader("yes\n"), io.Discard)
	if err == nil {
		t.Fatal("expected worktree removal error")
	}
	for _, call := range deps.calls {
		if strings.HasPrefix(call, "branch-delete:") {
			t.Fatalf("expected no branch deletion after worktree failure, got calls %v", deps.calls)
		}
	}
}

func fakeRemovePlanDeps(projectRoot string, gitRoot string, worktrees []git.Worktree, safety RemovalSafety) *fakeRemoveDeps {
	return &fakeRemoveDeps{projectRoot: projectRoot, gitRoot: gitRoot, worktrees: worktrees, safety: safety}
}

type fakeRemoveDeps struct {
	projectRoot   string
	gitRoot       string
	worktrees     []git.Worktree
	safety        RemovalSafety
	safetyOptions SafetyOptions
	LoadConfig    func(string, config.Overrides) (config.Config, error)
}

func (f *fakeRemoveDeps) DetectProject(string) (project.Project, error) {
	return project.Project{Root: f.projectRoot, GitRoot: f.gitRoot, WorktreeName: filepath.Base(f.gitRoot)}, nil
}

func (f *fakeRemoveDeps) loadConfig(projectRoot string, overrides config.Overrides) (config.Config, error) {
	if f.LoadConfig != nil {
		return f.LoadConfig(projectRoot, overrides)
	}
	return config.Default(), nil
}

func (f *fakeRemoveDeps) Worktrees(string) ([]git.Worktree, error) {
	return f.worktrees, nil
}

func (f *fakeRemoveDeps) AssessSafety(_ project.Project, worktree git.Worktree, _ config.Config, options SafetyOptions) (RemovalSafety, error) {
	f.safetyOptions = options
	safety := f.safety
	safety.Worktree = worktree
	if safety.Branch == "" {
		safety.Branch = worktree.Branch
	}
	return safety, nil
}

func fakeRemovePlan(kind SafetyKind) RemovePlan {
	return RemovePlan{
		Project:      project.Project{Root: "/repo", GitRoot: "/repo/main", WorktreeName: "main"},
		Worktree:     git.Worktree{Path: "/repo/feature-a", Name: "feature-a", Branch: "feature-a"},
		WorktreeName: "feature-a",
		WorktreePath: "/repo/feature-a",
		BranchName:   "feature-a",
		SessionName:  "project:feature-a",
		Safety:       RemovalSafety{Kind: kind, Branch: "feature-a", DefaultBranch: "main", BranchMerged: kind == SafetyCleanMerged},
		Config:       config.Default(),
	}
}

type fakeRemoveExecuteDeps struct {
	calls            []string
	sessionExists    bool
	insideTmux       bool
	currentSession   string
	homeDir          string
	worktreeErr      error
	branchErr        error
	adjacentSwitched bool
}

func (f *fakeRemoveExecuteDeps) HasSession(session string) (bool, error) {
	f.calls = append(f.calls, "has:"+session)
	return f.sessionExists, nil
}

func (f *fakeRemoveExecuteDeps) KillSession(session string) error {
	f.calls = append(f.calls, "kill:"+session)
	return nil
}

func (f *fakeRemoveExecuteDeps) WorktreeRemove(path string) error {
	f.calls = append(f.calls, "worktree-remove:"+path)
	return f.worktreeErr
}

func (f *fakeRemoveExecuteDeps) DeleteLocalBranch(branch string) error {
	f.calls = append(f.calls, "branch-delete:"+branch)
	return f.branchErr
}

func (f *fakeRemoveExecuteDeps) ForceDeleteLocalBranch(branch string) error {
	f.calls = append(f.calls, "branch-force-delete:"+branch)
	return f.branchErr
}

func (f *fakeRemoveExecuteDeps) InsideTmux() bool {
	return f.insideTmux
}

func (f *fakeRemoveExecuteDeps) CurrentSession() (string, error) {
	f.calls = append(f.calls, "current")
	return f.currentSession, nil
}

func (f *fakeRemoveExecuteDeps) SwitchToAdjacentSession(current string) (bool, error) {
	f.calls = append(f.calls, "adjacent:"+current)
	return f.adjacentSwitched, nil
}

func (f *fakeRemoveExecuteDeps) OpenFallbackSession(session string, dir string) error {
	f.calls = append(f.calls, "fallback:"+session+":"+dir)
	return nil
}

func (f *fakeRemoveExecuteDeps) HomeDir() string {
	if f.homeDir != "" {
		return f.homeDir
	}
	return "/home/test"
}

type eventWriter struct {
	events *[]string
}

func (e eventWriter) Write(p []byte) (int, error) {
	text := string(p)
	if strings.Contains(text, "Remove worktree?") {
		*e.events = append(*e.events, "prompt:Remove worktree?")
	}
	if strings.Contains(text, "Delete unmerged local branch") {
		*e.events = append(*e.events, "prompt:Delete unmerged local branch")
	}
	return len(p), nil
}
