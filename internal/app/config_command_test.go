package app

import (
	"bytes"
	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/project"
	"path/filepath"
	"testing"
)

func TestRunConfigInfoInsideProjectShowsPathsAndEffectiveConfig(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	gitRoot := filepath.Join(projectRoot, "main")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Config: ConfigDependencies{
		WorkingDir: func() (string, error) { return gitRoot, nil },
		DetectProject: func(string) (project.Project, error) {
			return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: "main"}, nil
		},
		UserConfigPath: func() (string, error) { return "/home/test/.config/utree/config.toml", nil },
		FileExists: func(path string) (bool, error) {
			return path == filepath.Join(projectRoot, ".utree", "config.toml"), nil
		},
		LoadConfig: func(projectRoot string) (config.Config, error) {
			cfg := config.Default()
			cfg.Project.Name = "merged"
			return cfg, nil
		},
	}}

	exitCode := app.Run([]string{"config", "info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	configPath := filepath.Join(projectRoot, ".utree", "config.toml")
	assertContains(t, stdout.String(), "User config: /home/test/.config/utree/config.toml")
	assertContains(t, stdout.String(), "User config exists: no")
	assertContains(t, stdout.String(), "Project config: "+configPath)
	assertContains(t, stdout.String(), "Project config exists: yes")
	assertContains(t, stdout.String(), "Config precedence:")
	assertContains(t, stdout.String(), "  project config")
	assertContains(t, stdout.String(), "Effective config:")
	assertContains(t, stdout.String(), "name = \"merged\"")
}
func TestRunConfigInfoOutsideProjectShowsUserPathOnly(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Config: ConfigDependencies{
		WorkingDir:     func() (string, error) { return "/tmp", nil },
		DetectProject:  func(string) (project.Project, error) { return project.Project{}, project.ErrNotUtreeProject },
		UserConfigPath: func() (string, error) { return "/home/test/.config/utree/config.toml", nil },
		FileExists:     func(string) (bool, error) { return false, nil },
		LoadConfig: func(projectRoot string) (config.Config, error) {
			if projectRoot != "" {
				t.Fatalf("expected no project root outside project, got %q", projectRoot)
			}
			return config.Default(), nil
		},
	}}

	exitCode := app.Run([]string{"config", "info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	assertContains(t, stdout.String(), "User config: /home/test/.config/utree/config.toml")
	assertContains(t, stdout.String(), "Project config: unavailable outside a utree project")
	assertNotContains(t, stdout.String(), "  project config")
	assertContains(t, stdout.String(), "Effective config:")
}
func TestRunConfigInfoShowsMergedUserAndProjectConfig(t *testing.T) {
	projectRoot := newInfoProjectRoot(t)
	gitRoot := filepath.Join(projectRoot, "main")
	xdgRoot := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	writeInfoFile(t, filepath.Join(xdgRoot, "utree", "config.toml"), "[project]\nname = \"user\"\n\n[git]\ndefault_branch = \"trunk\"\n")
	writeInfoFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), "[project]\nname = \"project\"\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	app := App{Stdout: &stdout, Stderr: &stderr, Config: ConfigDependencies{
		WorkingDir: func() (string, error) { return gitRoot, nil },
		DetectProject: func(string) (project.Project, error) {
			return project.Project{Root: projectRoot, GitRoot: gitRoot, WorktreeName: "main"}, nil
		},
	}}

	exitCode := app.Run([]string{"config", "info"})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr %q", exitCode, stderr.String())
	}
	assertContains(t, stdout.String(), "User config exists: yes")
	assertContains(t, stdout.String(), "Project config exists: yes")
	assertContains(t, stdout.String(), "name = \"project\"")
	assertContains(t, stdout.String(), "default_branch = \"trunk\"")
}
func TestRunConfigRejectsInvalidArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := App{Stdout: &stdout, Stderr: &stderr}.Run([]string{"config", "show"})

	if exitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	assertContains(t, stderr.String(), "usage: ut config info")
}
