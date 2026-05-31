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

func TestListPlanClassifiesProjectAndOtherWorktrees(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	otherRoot := t.TempDir()
	sessions := fakeListSessions(map[string]bool{
		filepath.Base(projectRoot) + "--main":      true,
		filepath.Base(projectRoot) + "--feature-a": false,
	})

	plan, err := PlanList(gitRoot, ListDependencies{
		DetectProject: fakeListProjectDetector(projectRoot, gitRoot),
		LoadConfig:    func(string, config.Overrides) (config.Config, error) { return config.Default(), nil },
		Worktrees: func(string) ([]git.Worktree, error) {
			return []git.Worktree{
				{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
				{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
				{Path: filepath.Join(otherRoot, "scratch"), Name: "scratch", Branch: "scratch"},
			}, nil
		},
		HasSession: sessions,
	})
	if err != nil {
		t.Fatalf("PlanList returned error: %v", err)
	}

	if len(plan.UtreeWorktrees) != 2 {
		t.Fatalf("expected 2 utree worktrees, got %d", len(plan.UtreeWorktrees))
	}
	if len(plan.OtherWorktrees) != 1 {
		t.Fatalf("expected 1 other git worktree, got %d", len(plan.OtherWorktrees))
	}
	if plan.UtreeWorktrees[0].Name != "main" || plan.UtreeWorktrees[0].Session != filepath.Base(projectRoot)+"--main" || !plan.UtreeWorktrees[0].SessionExists {
		t.Fatalf("unexpected main row: %+v", plan.UtreeWorktrees[0])
	}
	if plan.UtreeWorktrees[1].Name != "feature-a" || plan.UtreeWorktrees[1].Session != filepath.Base(projectRoot)+"--feature-a" || plan.UtreeWorktrees[1].SessionExists {
		t.Fatalf("unexpected feature row: %+v", plan.UtreeWorktrees[1])
	}
	if plan.OtherWorktrees[0].Name != "scratch" || plan.OtherWorktrees[0].Path != filepath.Join(otherRoot, "scratch") {
		t.Fatalf("unexpected other row: %+v", plan.OtherWorktrees[0])
	}
}

func TestListPlanDerivesNamesFromPathsWhenMissing(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	plan, err := PlanList(gitRoot, ListDependencies{
		DetectProject: fakeListProjectDetector(projectRoot, gitRoot),
		LoadConfig:    func(string, config.Overrides) (config.Config, error) { return config.Default(), nil },
		Worktrees: func(string) ([]git.Worktree, error) {
			return []git.Worktree{{Path: filepath.Join(projectRoot, "feature-a"), Branch: "feature-a"}}, nil
		},
		HasSession: fakeListSessions(nil),
	})
	if err != nil {
		t.Fatalf("PlanList returned error: %v", err)
	}

	if plan.UtreeWorktrees[0].Name != "feature-a" {
		t.Fatalf("expected name derived from path, got %+v", plan.UtreeWorktrees[0])
	}
}

func TestListPlanRendersConfiguredSessionNamesAndStatuses(t *testing.T) {
	projectRoot, gitRoot := newWorkspaceProject(t, "main")
	cfg := config.Default()
	cfg.Project.Name = "infra"
	cfg.Session.NameTemplate = "{project}__{worktree}"

	plan, err := PlanList(gitRoot, ListDependencies{
		DetectProject: fakeListProjectDetector(projectRoot, gitRoot),
		LoadConfig:    func(string, config.Overrides) (config.Config, error) { return cfg, nil },
		Worktrees: func(string) ([]git.Worktree, error) {
			return []git.Worktree{
				{Path: filepath.Join(projectRoot, "main"), Name: "main", Branch: "main"},
				{Path: filepath.Join(projectRoot, "feature-a"), Name: "feature-a", Branch: "feature-a"},
			}, nil
		},
		HasSession: fakeListSessions(map[string]bool{"infra__feature-a": true}),
	})
	if err != nil {
		t.Fatalf("PlanList returned error: %v", err)
	}

	output := RenderList(plan)
	assertContains(t, output, "main")
	assertContains(t, output, "-")
	assertContains(t, output, "infra__feature-a")
	assertNotContains(t, output, "infra__main")
}

func TestListPlanRequiresDetectedUtreeProject(t *testing.T) {
	_, err := PlanList("/repo/main", ListDependencies{
		DetectProject: func(string) (project.Project, error) { return project.Project{}, project.ErrNotUtreeProject },
		Worktrees:     func(string) ([]git.Worktree, error) { t.Fatal("did not expect worktree list"); return nil, nil },
		HasSession:    fakeListSessions(nil),
	})
	if !errors.Is(err, project.ErrNotUtreeProject) {
		t.Fatalf("expected project detection error, got %v", err)
	}
}

func TestListRenderIncludesUtreeSectionAndHeaders(t *testing.T) {
	output := RenderList(ListPlan{UtreeWorktrees: []ListWorktree{{Name: "main", Branch: "main", Session: "project:main", SessionExists: true}}})

	assertContains(t, output, "utree worktrees:")
	assertContains(t, output, "WORKTREE")
	assertContains(t, output, "BRANCH")
	assertContains(t, output, "SESSION")
	assertContains(t, output, "project:main")
}

func TestListRenderIncludesOtherWorktreesSectionOnlyWhenApplicable(t *testing.T) {
	withoutOther := RenderList(ListPlan{UtreeWorktrees: []ListWorktree{{Name: "main", Branch: "main", Session: "project:main", SessionExists: true}}})
	assertNotContains(t, withoutOther, "other git worktrees:")

	withOther := RenderList(ListPlan{
		UtreeWorktrees: []ListWorktree{{Name: "main", Branch: "main", Session: "project:main", SessionExists: true}},
		OtherWorktrees: []ListWorktree{{Name: "scratch", Branch: "scratch", Path: "/tmp/project-scratch"}},
	})
	assertContains(t, withOther, "other git worktrees:")
	assertContains(t, withOther, "PATH")
	assertContains(t, withOther, "/tmp/project-scratch")
}

func TestListRenderHandlesDetachedOrMissingBranch(t *testing.T) {
	output := RenderList(ListPlan{UtreeWorktrees: []ListWorktree{{Name: "detached", Session: "project:detached", SessionExists: true}}})

	assertContains(t, output, "detached")
	assertContains(t, output, "-")
}

func fakeListProjectDetector(projectRoot string, gitRoot string) func(string) (project.Project, error) {
	return func(string) (project.Project, error) {
		return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: filepath.Base(gitRoot)}, nil
	}
}

func fakeListSessions(sessions map[string]bool) func(string) (bool, error) {
	return func(session string) (bool, error) {
		return sessions[session], nil
	}
}

func assertNotContains(t *testing.T, value string, unwanted string) {
	t.Helper()

	if strings.Contains(value, unwanted) {
		t.Fatalf("expected %q not to contain %q", value, unwanted)
	}
}
