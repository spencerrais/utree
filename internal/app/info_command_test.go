package app

import (
	"bytes"
	"errors"
	"github.com/spencerrais/utree/internal/project"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRunInfoOutsideUtreeProjectReportsDiagnosticsAndReturnsZero(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{
		Stdout: &stdout,
		Stderr: &stderr,
		Info: InfoDependencies{
			CheckCommand:   fakeCommandChecker(map[string]bool{"git": true, "tmux": false}),
			DetectProject:  func() (project.Project, error) { return project.Project{}, project.ErrNotUtreeProject },
			TMUX:           func() string { return "" },
			CurrentBranch:  func(string) (string, error) { return "", nil },
			CurrentSession: func() (string, error) { return "", nil },
			DefaultBranch:  func(project.Project, string) (string, error) { return "", nil },
			LoadConfig:     func(string) (LoadedConfig, error) { return LoadedConfig{}, nil },
		},
	}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "git: available")
	assertContains(t, stdout.String(), "tmux: unavailable")
	assertContains(t, stdout.String(), "utree project: no")
	assertContains(t, stdout.String(), "active config: built-in defaults")
	assertContains(t, stdout.String(), "inside tmux: no")
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
func TestRunInfoInsideProjectReportsProjectConfigBranchAndWorktree(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	gitRoot := filepath.Join(projectRoot, "feature-a")
	writeInfoFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), "")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{
		Stdout: &stdout,
		Stderr: &stderr,
		Info: InfoDependencies{
			CheckCommand: fakeCommandChecker(map[string]bool{"git": true, "tmux": true}),
			DetectProject: func() (project.Project, error) {
				return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: "feature-a"}, nil
			},
			TMUX:           func() string { return "" },
			CurrentBranch:  func(dir string) (string, error) { return "feature-a", nil },
			CurrentSession: func() (string, error) { return "", nil },
			DefaultBranch:  func(project.Project, string) (string, error) { return "main", nil },
			LoadConfig:     defaultLoadInfoConfig,
		},
	}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "git: available")
	assertContains(t, stdout.String(), "tmux: available")
	assertContains(t, stdout.String(), "utree project: yes")
	assertContains(t, stdout.String(), "project root: "+projectRoot)
	assertContains(t, stdout.String(), "active config: "+filepath.Join(projectRoot, ".utree", "config.toml"))
	assertNotContains(t, stdout.String(), "config path:")
	assertContains(t, stdout.String(), "project name: "+filepath.Base(projectRoot))
	assertContains(t, stdout.String(), "default branch: main")
	assertContains(t, stdout.String(), "current worktree: feature-a")
	assertContains(t, stdout.String(), "current branch: feature-a")
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
}
func TestRunInfoFromProjectRootWithDefaultDependencies(t *testing.T) {
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

	app := App{Stdout: &stdout, Stderr: &stderr, Info: InfoDependencies{
		CheckCommand: fakeCommandChecker(map[string]bool{"git": true, "tmux": true}),
		DetectProject: func() (project.Project, error) {
			return project.Detect(projectRoot, project.GitRevParseRoot)
		},
		TMUX:           func() string { return "" },
		CurrentBranch:  defaultCurrentBranch,
		CurrentSession: func() (string, error) { return "", nil },
		DefaultBranch:  defaultInfoDefaultBranch,
		LoadConfig:     defaultLoadInfoConfig,
	}}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	assertContains(t, stdout.String(), "utree project: yes")
	assertContains(t, stdout.String(), "project root: "+projectRoot)
	assertContains(t, stdout.String(), "current worktree: ")
	assertContains(t, stdout.String(), "current branch: main")
}
func TestRunInfoReportsConfiguredProjectName(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	writeInfoFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), "[project]\nname = \"infra\"\n")
	var stdout bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Info: minimalProjectInfoDeps(projectRoot)}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "project name: infra")
}
func TestRunInfoReportsBuiltInDefaultsWhenNoConfigFilesExist(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	var stdout bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Info: minimalProjectInfoDeps(projectRoot)}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "active config: built-in defaults")
}
func TestRunInfoReportsUserConfigWhenOnlyUserConfigExists(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	xdgRoot := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	userConfigPath := filepath.Join(xdgRoot, "utree", "config.toml")
	writeInfoFile(t, userConfigPath, "[project]\nname = \"personal\"\n")
	var stdout bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Info: minimalProjectInfoDeps(projectRoot)}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "active config: "+userConfigPath)
}
func TestRunInfoReportsDefaultBranchDetectionFailureWithoutFailingCommand(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	var stdout bytes.Buffer

	deps := minimalProjectInfoDeps(projectRoot)
	deps.DefaultBranch = func(project.Project, string) (string, error) { return "", errors.New("default branch missing") }
	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Info: deps}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "default branch: unavailable")
}
func TestRunInfoReportsMissingDependenciesWithoutFailingCommand(t *testing.T) {
	var stdout bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Info: InfoDependencies{
		CheckCommand:  fakeCommandChecker(map[string]bool{"git": false, "tmux": false}),
		DetectProject: func() (project.Project, error) { return project.Project{}, project.ErrNotGitRepository },
		TMUX:          func() string { return "" },
	}}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "git: unavailable")
	assertContains(t, stdout.String(), "tmux: unavailable")
}
func TestRunInfoInsideTmuxReportsCurrentSession(t *testing.T) {
	var stdout bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &bytes.Buffer{}, Info: InfoDependencies{
		CheckCommand:   fakeCommandChecker(map[string]bool{"git": true, "tmux": true}),
		DetectProject:  func() (project.Project, error) { return project.Project{}, project.ErrNotUtreeProject },
		TMUX:           func() string { return "/tmp/tmux-1000/default,123,0" },
		CurrentSession: func() (string, error) { return "project:feature-a", nil },
		CurrentBranch:  func(string) (string, error) { return "", nil },
		DefaultBranch:  func(project.Project, string) (string, error) { return "", nil },
		LoadConfig:     func(string) (LoadedConfig, error) { return LoadedConfig{}, nil },
	}}

	exitCode := app.Run([]string{"info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	assertContains(t, stdout.String(), "inside tmux: yes")
	assertContains(t, stdout.String(), "active config: built-in defaults")
	assertContains(t, stdout.String(), "tmux session: project:feature-a")
}
func TestRunInfoRejectsExtraArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"info", "unexpected"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "usage: ut info")
}
