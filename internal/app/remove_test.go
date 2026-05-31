package app

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/workspace"
)

func TestRunRemoveParsesNameAndDefaultBranchOverride(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	gotOptions := workspace.RemoveOptions{}

	app := App{Stdout: &stdout, Stderr: &stderr, Remove: RemoveDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(startDir string, options workspace.RemoveOptions) (workspace.RemovePlan, error) {
			if startDir != "/repo/main" {
				t.Fatalf("expected start dir /repo/main, got %q", startDir)
			}
			gotOptions = options
			return workspace.RemovePlan{}, nil
		},
		Execute: func(workspace.RemovePlan, io.Reader, io.Writer) (bool, error) { return true, nil },
	}}

	exitCode := app.Run([]string{"remove", "feature-a", "--default-branch", "develop"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if gotOptions.WorktreeName != "feature-a" || gotOptions.DefaultBranchOverride == nil || *gotOptions.DefaultBranchOverride != "develop" {
		t.Fatalf("unexpected options: %+v", gotOptions)
	}
}

func TestRunRemoveRejectsMissingNameAllAndTooManyArgs(t *testing.T) {
	for _, args := range [][]string{{"remove"}, {"remove", "--all"}, {"remove", "feature-a", "extra"}, {"remove", "feature-a", "--yes"}} {
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
			assertContains(t, stderr.String(), "usage: ut remove")
		})
	}
}

func TestRunRemovePassesStdinStdoutToExecute(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdin := strings.NewReader("yes\n")
	readConfirmation := ""

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: stdin, Remove: RemoveDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(string, workspace.RemoveOptions) (workspace.RemovePlan, error) {
			return workspace.RemovePlan{}, nil
		},
		Execute: func(_ workspace.RemovePlan, confirmation io.Reader, output io.Writer) (bool, error) {
			contents, err := io.ReadAll(confirmation)
			if err != nil {
				t.Fatalf("read confirmation: %v", err)
			}
			readConfirmation = string(contents)
			_, err = output.Write([]byte("removed\n"))
			return true, err
		},
	}}

	exitCode := app.Run([]string{"remove", "feature-a"})
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if readConfirmation != "yes\n" {
		t.Fatalf("expected stdin to be passed through, got %q", readConfirmation)
	}
	assertContains(t, stdout.String(), "removed")
}

func TestRunRemovePropagatesDirtyRefusal(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Remove: RemoveDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(string, workspace.RemoveOptions) (workspace.RemovePlan, error) {
			return workspace.RemovePlan{}, errors.New("worktree has local changes")
		},
		Execute: func(workspace.RemovePlan, io.Reader, io.Writer) (bool, error) {
			t.Fatal("did not expect execute")
			return false, nil
		},
	}}

	exitCode := app.Run([]string{"remove", "feature-a"})
	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	assertContains(t, stderr.String(), "remove:")
	assertContains(t, stderr.String(), "local changes")
}
