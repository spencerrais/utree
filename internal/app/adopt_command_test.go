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

func TestRunAdoptPreviewsAndExecutesAfterConfirmation(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var stdin bytes.Buffer
	stdin.WriteString("yes\n")
	plannedStartDir := ""
	executedPlan := convert.AdoptPlan{}

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: &stdin, Adopt: AdoptDependencies{
		WorkingDir: func() (string, error) { return "/repo/main", nil },
		Plan: func(startDir string) (convert.AdoptPlan, error) {
			plannedStartDir = startDir
			return convert.AdoptPlan{ProjectRoot: "/repo", WorktreeRoot: "/repo/main", WorktreeName: "main", MarkerPath: "/repo/.utree"}, nil
		},
		Execute: func(plan convert.AdoptPlan, confirmation io.Reader) (bool, error) {
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

	exitCode := app.Run([]string{"adopt"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	if plannedStartDir != "/repo/main" {
		t.Fatalf("expected start dir /repo/main, got %q", plannedStartDir)
	}
	if executedPlan.MarkerPath != "/repo/.utree" {
		t.Fatalf("expected execute plan, got %#v", executedPlan)
	}
	assertContains(t, stdout.String(), "Adopt existing worktree layout")
	assertContains(t, stdout.String(), "Project marker will be created at:")
	assertContains(t, stdout.String(), "/repo/.utree")
	assertContains(t, stdout.String(), "Optional project config can live at:")
	assertContains(t, stdout.String(), "/repo/.utree/config.toml")
	assertContains(t, stdout.String(), "Adoption complete")
}
func TestRunAdoptRejectsArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"adopt", "extra"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "usage: ut adopt")
}
func TestRunAdoptWithDefaultDependenciesAdoptsExistingLayoutFromSubdirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	projectRoot := t.TempDir()
	mainRoot := filepath.Join(projectRoot, "main")
	if err := os.Mkdir(mainRoot, 0o755); err != nil {
		t.Fatalf("create main worktree: %v", err)
	}
	runInfoGit(t, mainRoot, "init", "-b", "main")
	runInfoGit(t, mainRoot, "config", "user.email", "test@example.com")
	runInfoGit(t, mainRoot, "config", "user.name", "Test User")
	writeInfoFile(t, filepath.Join(mainRoot, "README.md"), "hello\n")
	runInfoGit(t, mainRoot, "add", "README.md")
	runInfoGit(t, mainRoot, "commit", "-m", "initial")
	startDir := filepath.Join(mainRoot, "subdir")
	if err := os.Mkdir(startDir, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Stdin: strings.NewReader("yes\n"), Adopt: AdoptDependencies{
		WorkingDir: func() (string, error) { return startDir, nil },
	}}

	exitCode := app.Run([]string{"adopt"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	assertContains(t, stdout.String(), "Adoption complete")
	if info, err := os.Stat(filepath.Join(projectRoot, ".utree")); err != nil || !info.IsDir() {
		t.Fatalf("expected .utree marker directory, info %#v err %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".utree", "metadata.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected no metadata.toml, stat err %v", err)
	}
}
func TestRunAdoptWithDefaultDependenciesRejectsExistingUtreeProject(t *testing.T) {
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
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Adopt: AdoptDependencies{
		WorkingDir: func() (string, error) { return mainRoot, nil },
	}}

	exitCode := app.Run([]string{"adopt"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "adopt: already inside a utree project")
}
