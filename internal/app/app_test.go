package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
)

func TestRunWithoutCommandPrintsHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run(nil)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "Usage: ut <command>")
	assertContains(t, stdout.String(), "Commands:")
	assertContains(t, stdout.String(), "adopt        mark an existing")
	assertContains(t, stdout.String(), "config info  show config paths")
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}

func TestRunHelpFlagsPrintHelp(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "long help", args: []string{"--help"}},
		{name: "short help", args: []string{"-h"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run(testCase.args)

			if exitCode != 0 {
				t.Fatalf("expected exit code 0, got %d", exitCode)
			}
			assertContains(t, stdout.String(), "Usage: ut <command>")
			if stderr.String() != "" {
				t.Fatalf("expected empty stderr, got %q", stderr.String())
			}
		})
	}
}

func TestRunUnknownCommandReturnsNonZero(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"bogus"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "unknown command: bogus")
	assertContains(t, stderr.String(), "Usage: ut <command>")
}

func assertContains(t *testing.T, value string, want string) {
	t.Helper()

	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}

func assertNotContains(t *testing.T, value string, want string) {
	t.Helper()

	if strings.Contains(value, want) {
		t.Fatalf("expected %q not to contain %q", value, want)
	}
}

func fakeCommandChecker(available map[string]bool) func(string) bool {
	return func(name string) bool {
		return available[name]
	}
}

func minimalProjectInfoDeps(projectRoot string) InfoDependencies {
	gitRoot := filepath.Join(projectRoot, "feature-a")
	return InfoDependencies{
		CheckCommand: fakeCommandChecker(map[string]bool{"git": true, "tmux": true}),
		DetectProject: func() (project.Project, error) {
			return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: "feature-a"}, nil
		},
		TMUX:           func() string { return "" },
		CurrentBranch:  func(string) (string, error) { return "feature-a", nil },
		CurrentSession: func() (string, error) { return "", nil },
		DefaultBranch:  func(project.Project, string) (string, error) { return "main", nil },
		LoadConfig:     defaultLoadInfoConfig,
	}
}

func newInfoProjectRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))

	projectRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectRoot, ".utree"), 0o755); err != nil {
		t.Fatalf("create .utree: %v", err)
	}
	return projectRoot
}

func writeInfoFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertInfoFileContains(t *testing.T, path string, want string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(contents), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(contents))
	}
}

func runInfoGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := git.Command(dir, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
