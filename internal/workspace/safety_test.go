package workspace

import (
	"errors"
	"testing"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
)

func TestParseWorktreeStatusClean(t *testing.T) {
	status := ParseWorktreeStatusPorcelain("")

	if status.HasUnstagedChanges || status.HasStagedChanges || status.HasUntrackedFiles || status.Dirty() {
		t.Fatalf("expected clean status, got %+v", status)
	}
}

func TestParseWorktreeStatusDetectsUnstagedTrackedChanges(t *testing.T) {
	for _, output := range []string{" M README.md\n", " D old.go\n"} {
		t.Run(output, func(t *testing.T) {
			status := ParseWorktreeStatusPorcelain(output)

			if !status.HasUnstagedChanges {
				t.Fatalf("expected unstaged changes, got %+v", status)
			}
			if status.HasStagedChanges || status.HasUntrackedFiles == true {
				t.Fatalf("expected only unstaged changes, got %+v", status)
			}
		})
	}
}

func TestParseWorktreeStatusDetectsStagedChanges(t *testing.T) {
	for _, output := range []string{"M  README.md\n", "A  new.go\n", "D  old.go\n", "R  old.go -> new.go\n"} {
		t.Run(output, func(t *testing.T) {
			status := ParseWorktreeStatusPorcelain(output)

			if !status.HasStagedChanges {
				t.Fatalf("expected staged changes, got %+v", status)
			}
			if status.HasUnstagedChanges || status.HasUntrackedFiles {
				t.Fatalf("expected only staged changes, got %+v", status)
			}
		})
	}
}

func TestParseWorktreeStatusDetectsUntrackedFiles(t *testing.T) {
	status := ParseWorktreeStatusPorcelain("?? scratch.txt\n")

	if !status.HasUntrackedFiles {
		t.Fatalf("expected untracked files, got %+v", status)
	}
	if status.HasStagedChanges || status.HasUnstagedChanges {
		t.Fatalf("expected only untracked files, got %+v", status)
	}
}

func TestParseWorktreeStatusDetectsMixedDirtyStates(t *testing.T) {
	status := ParseWorktreeStatusPorcelain("M  staged.go\n M unstaged.go\n?? scratch.txt\n")

	if !status.HasStagedChanges || !status.HasUnstagedChanges || !status.HasUntrackedFiles || !status.Dirty() {
		t.Fatalf("expected all dirty states, got %+v", status)
	}
}

func TestAssessRemovalSafetyCleanMerged(t *testing.T) {
	deps := fakeSafetyDeps("", "main", true)

	safety, err := AssessRemovalSafety(fakeSafetyProject(), fakeSafetyWorktree("feature-a"), config.Default(), SafetyOptions{}, deps)
	if err != nil {
		t.Fatalf("AssessRemovalSafety returned error: %v", err)
	}

	if safety.Kind != SafetyCleanMerged || !safety.BranchMerged {
		t.Fatalf("expected clean merged safety, got %+v", safety)
	}
	if safety.Branch != "feature-a" || safety.DefaultBranch != "main" || safety.Status.Dirty() {
		t.Fatalf("unexpected safety details: %+v", safety)
	}
}

func TestAssessRemovalSafetyCleanUnmerged(t *testing.T) {
	deps := fakeSafetyDeps("", "main", false)

	safety, err := AssessRemovalSafety(fakeSafetyProject(), fakeSafetyWorktree("feature-a"), config.Default(), SafetyOptions{}, deps)
	if err != nil {
		t.Fatalf("AssessRemovalSafety returned error: %v", err)
	}

	if safety.Kind != SafetyCleanUnmerged || safety.BranchMerged {
		t.Fatalf("expected clean unmerged safety, got %+v", safety)
	}
}

func TestAssessRemovalSafetyDirtySkipsMergeCheck(t *testing.T) {
	deps := fakeSafetyDeps(" M README.md\n?? scratch.txt\n", "main", true)

	safety, err := AssessRemovalSafety(fakeSafetyProject(), fakeSafetyWorktree("feature-a"), config.Default(), SafetyOptions{}, deps)
	if err != nil {
		t.Fatalf("AssessRemovalSafety returned error: %v", err)
	}

	if safety.Kind != SafetyDirty || !safety.Status.HasUnstagedChanges || !safety.Status.HasUntrackedFiles {
		t.Fatalf("expected dirty safety, got %+v", safety)
	}
	if deps.mergeCalls != 0 {
		t.Fatalf("expected no merge checks for dirty worktree, got %d", deps.mergeCalls)
	}
}

func TestAssessRemovalSafetyRequiresAssociatedBranch(t *testing.T) {
	safety, err := AssessRemovalSafety(fakeSafetyProject(), fakeSafetyWorktree(""), config.Default(), SafetyOptions{}, fakeSafetyDeps("", "main", true))
	if err != nil {
		t.Fatalf("AssessRemovalSafety returned error: %v", err)
	}

	if safety.Kind != SafetyNoLocalBranch {
		t.Fatalf("expected no local branch safety, got %+v", safety)
	}
}

func TestAssessRemovalSafetyUsesDetectedDefaultBranch(t *testing.T) {
	deps := fakeSafetyDeps("", "develop", true)

	safety, err := AssessRemovalSafety(fakeSafetyProject(), fakeSafetyWorktree("feature-a"), config.Default(), SafetyOptions{}, deps)
	if err != nil {
		t.Fatalf("AssessRemovalSafety returned error: %v", err)
	}

	if safety.DefaultBranch != "develop" || deps.mergedDefaultBranch != "develop" {
		t.Fatalf("expected develop default branch, got safety %+v deps %+v", safety, deps)
	}
}

func TestAssessRemovalSafetyPropagatesDependencyErrors(t *testing.T) {
	for _, testCase := range []struct {
		name string
		deps SafetyDependencies
	}{
		{
			name: "status",
			deps: errorSafetyDeps{statusErr: errors.New("status failed")},
		},
		{
			name: "default branch",
			deps: errorSafetyDeps{defaultErr: errors.New("default failed")},
		},
		{
			name: "merge",
			deps: errorSafetyDeps{mergeErr: errors.New("merge failed")},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := AssessRemovalSafety(fakeSafetyProject(), fakeSafetyWorktree("feature-a"), config.Default(), SafetyOptions{}, testCase.deps)
			if err == nil {
				t.Fatal("expected dependency error")
			}
		})
	}
}

func fakeSafetyProject() project.Project {
	return project.Project{Root: "/repo", GitRoot: "/repo/main", WorktreeName: "main"}
}

func fakeSafetyWorktree(branch string) git.Worktree {
	return git.Worktree{Path: "/repo/feature-a", Branch: branch, Name: "feature-a"}
}

func fakeSafetyDeps(statusOutput string, defaultBranch string, merged bool) *recordingSafetyDeps {
	return &recordingSafetyDeps{statusOutput: statusOutput, defaultBranch: defaultBranch, merged: merged}
}

type recordingSafetyDeps struct {
	statusOutput        string
	defaultBranch       string
	merged              bool
	mergeCalls          int
	mergedDefaultBranch string
}

func (r *recordingSafetyDeps) StatusPorcelain(string) (string, error) {
	return r.statusOutput, nil
}

func (r *recordingSafetyDeps) DefaultBranch(project.Project, string, *string) (string, error) {
	return r.defaultBranch, nil
}

func (r *recordingSafetyDeps) BranchMerged(defaultBranch string, branch string) (bool, error) {
	r.mergeCalls++
	r.mergedDefaultBranch = defaultBranch
	return r.merged, nil
}

type errorSafetyDeps struct {
	statusErr  error
	defaultErr error
	mergeErr   error
}

func (e errorSafetyDeps) StatusPorcelain(string) (string, error) {
	if e.statusErr != nil {
		return "", e.statusErr
	}
	return "", nil
}

func (e errorSafetyDeps) DefaultBranch(project.Project, string, *string) (string, error) {
	if e.defaultErr != nil {
		return "", e.defaultErr
	}
	return "main", nil
}

func (e errorSafetyDeps) BranchMerged(string, string) (bool, error) {
	if e.mergeErr != nil {
		return false, e.mergeErr
	}
	return true, nil
}
