package app

import (
	"bytes"
	"github.com/spencerrais/utree/internal/convert"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunConvertPlansPreviewsAndExecutesAfterConfirmation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var stdin bytes.Buffer
	stdin.WriteString("yes\n")
	plannedStartDir := ""
	executedPlan := convert.Plan{}

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: &stdin, Convert: ConvertDependencies{
		WorkingDir: func() (string, error) { return "/repo", nil },
		Plan: func(startDir string, options convert.Options) (convert.Plan, error) {
			plannedStartDir = startDir
			if options.DefaultBranchOverride != nil {
				t.Fatalf("expected no default branch override, got %q", *options.DefaultBranchOverride)
			}
			return convert.Plan{From: "/repo", PrimaryTarget: "/repo/main", MarkerPath: "/repo/.utree", DefaultBranch: "main"}, nil
		},
		Execute: func(plan convert.Plan, confirmation io.Reader) (bool, error) {
			executedPlan = plan
			contents, err := io.ReadAll(confirmation)
			if err != nil {
				t.Fatalf("read confirmation: %v", err)
			}
			if string(contents) != "yes\n" {
				t.Fatalf("expected confirmation to be passed through, got %q", string(contents))
			}
			return true, nil
		},
	}}

	exitCode := app.Run([]string{"convert"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if plannedStartDir != "/repo" {
		t.Fatalf("expected planned start dir /repo, got %q", plannedStartDir)
	}
	if executedPlan.DefaultBranch != "main" {
		t.Fatalf("expected executed plan, got %+v", executedPlan)
	}
	assertContains(t, stdout.String(), "Convert repository layout:")
	assertContains(t, stdout.String(), "from: /repo")
	assertContains(t, stdout.String(), "to:   /repo/main")
	assertContains(t, stdout.String(), "Detected default branch:")
	assertContains(t, stdout.String(), "main")
	assertContains(t, stdout.String(), "Project marker will be created at:")
	assertContains(t, stdout.String(), "/repo/.utree")
	assertContains(t, stdout.String(), "Optional project config can live at:")
	assertContains(t, stdout.String(), "/repo/.utree/config.toml")
	assertContains(t, stdout.String(), "Continue? [y/N]")
	assertContains(t, stdout.String(), "Conversion complete")
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
func TestRunConvertPassesDefaultBranchOverride(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("n\n"), Convert: ConvertDependencies{
		WorkingDir: func() (string, error) { return "/repo", nil },
		Plan: func(_ string, options convert.Options) (convert.Plan, error) {
			if options.DefaultBranchOverride == nil || *options.DefaultBranchOverride != "trunk" {
				t.Fatalf("expected trunk override, got %#v", options.DefaultBranchOverride)
			}
			return convert.Plan{From: "/repo", PrimaryTarget: "/repo/trunk", MarkerPath: "/repo/.utree", DefaultBranch: "trunk"}, nil
		},
		Execute: func(convert.Plan, io.Reader) (bool, error) { return false, nil },
	}}

	exitCode := app.Run([]string{"convert", "--default-branch", "trunk"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "Conversion cancelled")
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
func TestRunConvertRejectsInvalidArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"convert", "--bogus"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "usage: ut convert [--default-branch <branch>]")
}
func TestRunConvertWithDefaultDependenciesConvertsTempRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repoRoot := t.TempDir()
	runInfoGit(t, repoRoot, "init", "-b", "main")
	runInfoGit(t, repoRoot, "config", "user.email", "test@example.com")
	runInfoGit(t, repoRoot, "config", "user.name", "Test User")
	writeInfoFile(t, filepath.Join(repoRoot, "README.md"), "hello\n")
	runInfoGit(t, repoRoot, "add", "README.md")
	runInfoGit(t, repoRoot, "commit", "-m", "initial")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("yes\n"), Convert: ConvertDependencies{
		WorkingDir: func() (string, error) { return repoRoot, nil },
	}}

	exitCode := app.Run([]string{"convert", "--default-branch", "main"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	assertContains(t, stdout.String(), "Conversion complete")
	if info, err := os.Stat(filepath.Join(repoRoot, ".utree")); err != nil || !info.IsDir() {
		t.Fatalf("expected .utree marker directory, info %#v err %v", info, err)
	}
	assertInfoFileContains(t, filepath.Join(repoRoot, "main", "README.md"), "hello")
	if _, err := os.Stat(filepath.Join(repoRoot, "main", ".git")); err != nil {
		t.Fatalf("expected .git under converted primary worktree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no config.toml by default, stat err %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".utree", "metadata.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no metadata.toml, stat err %v", err)
	}
}
func TestRunConvertWithDefaultDependenciesRejectsExistingUtreeProject(t *testing.T) {
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
	runInfoGit(t, mainRoot, "init", "-b", "main")
	runInfoGit(t, mainRoot, "config", "user.email", "test@example.com")
	runInfoGit(t, mainRoot, "config", "user.name", "Test User")
	writeInfoFile(t, filepath.Join(mainRoot, "README.md"), "hello\n")
	runInfoGit(t, mainRoot, "add", "README.md")
	runInfoGit(t, mainRoot, "commit", "-m", "initial")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Convert: ConvertDependencies{
		WorkingDir: func() (string, error) { return mainRoot, nil },
	}}

	exitCode := app.Run([]string{"convert", "--default-branch", "main"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "convert: already inside a utree project")
}
