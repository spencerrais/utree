package app

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/project"
	"github.com/spencerrais/utree/internal/workspace"
)

func TestRunNewParsesOneNameAndFlagsAfterPositionals(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	gotOptions := workspace.NewOptions{}

	app := App{Stdout: &stdout, Stderr: &stderr, New: NewDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(startDir string, options workspace.NewOptions) (workspace.NewPlan, error) {
			if startDir != "/repo/main" {
				t.Fatalf("expected start dir /repo/main, got %q", startDir)
			}
			gotOptions = options
			return workspace.NewPlan{WorktreeName: options.WorktreeName, BranchName: options.BranchName}, nil
		},
		Execute: func(workspace.NewPlan, io.Reader, io.Writer) (bool, error) { return true, nil },
	}}

	exitCode := app.Run([]string{"new", "feature-a", "--default-branch", "develop", "--base", "main"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if gotOptions.WorktreeName != "feature-a" || gotOptions.BranchName != "feature-a" {
		t.Fatalf("expected one name to set worktree and branch, got %+v", gotOptions)
	}
	if gotOptions.DefaultBranchOverride == nil || *gotOptions.DefaultBranchOverride != "develop" {
		t.Fatalf("expected default branch override develop, got %+v", gotOptions.DefaultBranchOverride)
	}
	if gotOptions.BaseBranch != "main" {
		t.Fatalf("expected base main, got %q", gotOptions.BaseBranch)
	}
}
func TestRunNewPassesWarningConfirmationReader(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdin := strings.NewReader("yes\n")
	readConfirmation := ""
	plan := workspace.NewPlan{RequiresDefaultBranchWarning: true}

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: stdin, New: NewDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan:       func(string, workspace.NewOptions) (workspace.NewPlan, error) { return plan, nil },
		Execute: func(_ workspace.NewPlan, confirmation io.Reader, _ io.Writer) (bool, error) {
			contents, err := io.ReadAll(confirmation)
			if err != nil {
				t.Fatalf("read confirmation: %v", err)
			}
			readConfirmation = string(contents)
			return true, nil
		},
	}}

	exitCode := app.Run([]string{"new", "feature-a"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if readConfirmation != "yes\n" {
		t.Fatalf("expected confirmation reader contents, got %q", readConfirmation)
	}
}
func TestRunNewRejectsInvalidArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"new", "feature-a", "branch", "extra"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "usage: ut new")
}
func TestRunNewReportsCreatedButTmuxFailedError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, New: NewDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan:       func(string, workspace.NewOptions) (workspace.NewPlan, error) { return workspace.NewPlan{}, nil },
		Execute: func(workspace.NewPlan, io.Reader, io.Writer) (bool, error) {
			return false, errors.New("worktree created at /repo/feature-a but tmux open failed")
		},
	}}

	exitCode := app.Run([]string{"new", "feature-a"})
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	assertContains(t, stderr.String(), "worktree created")
	assertContains(t, stderr.String(), "tmux open failed")
}
func TestRunOpenWithNoArgsPlansAndExecutesCurrentWorktree(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	gotOptions := workspace.OpenOptions{Target: "unset"}

	app := App{Stdout: &stdout, Stderr: &stderr, Open: OpenDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(startDir string, options workspace.OpenOptions) (workspace.OpenPlan, error) {
			if startDir != "/repo/main" {
				t.Fatalf("expected start dir /repo/main, got %q", startDir)
			}
			gotOptions = options
			return workspace.OpenPlan{}, nil
		},
		Execute: func(workspace.OpenPlan) error { return nil },
	}}

	exitCode := app.Run([]string{"open"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if gotOptions.Target != "" {
		t.Fatalf("expected empty open target, got %q", gotOptions.Target)
	}
}
func TestRunOpenWithDotPlansAndExecutesCurrentWorktree(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	gotOptions := workspace.OpenOptions{}

	app := App{Stdout: &stdout, Stderr: &stderr, Open: OpenDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(_ string, options workspace.OpenOptions) (workspace.OpenPlan, error) {
			gotOptions = options
			return workspace.OpenPlan{}, nil
		},
		Execute: func(workspace.OpenPlan) error { return nil },
	}}

	exitCode := app.Run([]string{"open", "."})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if gotOptions.Target != "." {
		t.Fatalf("expected dot open target, got %q", gotOptions.Target)
	}
}
func TestRunOpenWithNamePassesTargetName(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	gotOptions := workspace.OpenOptions{}

	app := App{Stdout: &stdout, Stderr: &stderr, Open: OpenDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(_ string, options workspace.OpenOptions) (workspace.OpenPlan, error) {
			gotOptions = options
			return workspace.OpenPlan{}, nil
		},
		Execute: func(workspace.OpenPlan) error { return nil },
	}}

	exitCode := app.Run([]string{"open", "feature-a"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if gotOptions.Target != "feature-a" {
		t.Fatalf("expected target feature-a, got %q", gotOptions.Target)
	}
}
func TestRunOpenRejectsTooManyArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"open", "one", "two"})
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	assertContains(t, stderr.String(), "usage: ut open")
}
func TestRunListPlansRendersAndWritesOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	plannedStartDir := ""

	app := App{Stdout: &stdout, Stderr: &stderr, List: ListDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(startDir string) (workspace.ListPlan, error) {
			plannedStartDir = startDir
			return workspace.ListPlan{UtreeWorktrees: []workspace.ListWorktree{{Name: "main", Branch: "main", Session: "project:main", SessionExists: true}}}, nil
		},
		Render: func(plan workspace.ListPlan) string { return workspace.RenderList(plan) },
	}}

	exitCode := app.Run([]string{"list"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if plannedStartDir != "/repo/main" {
		t.Fatalf("expected planned start dir /repo/main, got %q", plannedStartDir)
	}
	assertContains(t, stdout.String(), "utree worktrees:")
	assertContains(t, stdout.String(), "project:main")
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
func TestRunListRejectsArgsAndAllFlag(t *testing.T) {
	for _, args := range [][]string{{"list", "unexpected"}, {"list", "--all"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run(args)

			if exitCode == 0 {
				t.Fatal("expected non-zero exit code")
			}
			if stdout.String() != "" {
				t.Fatalf("expected empty stdout, got %q", stdout.String())
			}
			assertContains(t, stderr.String(), "usage: ut list")
		})
	}
}
func TestRunListPropagatesProjectDetectionFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, List: ListDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan:       func(string) (workspace.ListPlan, error) { return workspace.ListPlan{}, project.ErrNotUtreeProject },
		Render:     func(workspace.ListPlan) string { t.Fatal("did not expect render"); return "" },
	}}

	exitCode := app.Run([]string{"list"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "list:")
	assertContains(t, stderr.String(), project.ErrNotUtreeProject.Error())
}
