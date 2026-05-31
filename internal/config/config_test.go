package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultConfigMatchesPRD(t *testing.T) {
	cfg := Default()

	if cfg.Project.Name != "auto" {
		t.Fatalf("expected project name auto, got %q", cfg.Project.Name)
	}
	if cfg.Git.DefaultBranch != "auto" {
		t.Fatalf("expected git default branch auto, got %q", cfg.Git.DefaultBranch)
	}
	if cfg.Session.NameTemplate != "{project}--{worktree}--{branch}" {
		t.Fatalf("expected session template {project}--{worktree}--{branch}, got %q", cfg.Session.NameTemplate)
	}
	wantPanes := []PaneConfig{
		{Command: "nvim .; exec ${SHELL:-/bin/sh} -l", Selected: true},
		{Command: "git status; exec ${SHELL:-/bin/sh} -l", Split: "vertical", Size: "33%"},
	}
	if !reflect.DeepEqual(cfg.Layout.Default.Panes, wantPanes) {
		t.Fatalf("expected default panes %#v, got %#v", wantPanes, cfg.Layout.Default.Panes)
	}
}

func TestLoadProjectConfigOverridesDefaults(t *testing.T) {
	projectRoot := newProjectRoot(t)
	writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), `
[project]
name = "infra"

[git]
default_branch = "main"

[session]
name_template = "{project}__{worktree}"

[layout.default]

[[layout.default.panes]]
command = "nvim ."

[[layout.default.panes]]
split = "vertical"
size = "40%"
command = "git status --short; exec ${SHELL:-/bin/sh} -l"
selected = true
`)

	cfg, err := Load(projectRoot, Overrides{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Project.Name != "infra" {
		t.Fatalf("expected project name infra, got %q", cfg.Project.Name)
	}
	if cfg.Git.DefaultBranch != "main" {
		t.Fatalf("expected default branch main, got %q", cfg.Git.DefaultBranch)
	}
	if cfg.Session.NameTemplate != "{project}__{worktree}" {
		t.Fatalf("expected overridden session template, got %q", cfg.Session.NameTemplate)
	}
	wantPanes := []PaneConfig{
		{Command: "nvim ."},
		{Command: "git status --short; exec ${SHELL:-/bin/sh} -l", Split: "vertical", Size: "40%", Selected: true},
	}
	if !reflect.DeepEqual(cfg.Layout.Default.Panes, wantPanes) {
		t.Fatalf("expected panes %#v, got %#v", wantPanes, cfg.Layout.Default.Panes)
	}
}

func TestLoadUserConfigOverridesDefaults(t *testing.T) {
	projectRoot := newProjectRoot(t)
	userConfigPath := setUserConfigRoot(t)
	writeFile(t, userConfigPath, `
[project]
name = "personal"

[session]
name_template = "{project}:{worktree}"
`)

	cfg, err := Load(projectRoot, Overrides{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Project.Name != "personal" {
		t.Fatalf("expected user project name, got %q", cfg.Project.Name)
	}
	if cfg.Session.NameTemplate != "{project}:{worktree}" {
		t.Fatalf("expected user session template, got %q", cfg.Session.NameTemplate)
	}
}

func TestLoadProjectConfigOverridesUserConfig(t *testing.T) {
	projectRoot := newProjectRoot(t)
	userConfigPath := setUserConfigRoot(t)
	writeFile(t, userConfigPath, `
[project]
name = "personal"

[git]
default_branch = "main"
`)
	writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), `
[project]
name = "project"
`)

	cfg, err := Load(projectRoot, Overrides{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Project.Name != "project" {
		t.Fatalf("expected project config to override user config, got %q", cfg.Project.Name)
	}
	if cfg.Git.DefaultBranch != "main" {
		t.Fatalf("expected user default branch to remain, got %q", cfg.Git.DefaultBranch)
	}
}

func TestLoadCLIOverridesWinOverUserAndProjectConfig(t *testing.T) {
	projectRoot := newProjectRoot(t)
	userConfigPath := setUserConfigRoot(t)
	writeFile(t, userConfigPath, "[project]\nname = \"personal\"\n")
	writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), "[project]\nname = \"project\"\n")

	projectName := "cli"
	cfg, err := Load(projectRoot, Overrides{ProjectName: &projectName})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Project.Name != "cli" {
		t.Fatalf("expected CLI project name, got %q", cfg.Project.Name)
	}
}

func TestLoadInvalidUserConfigReturnsActionableError(t *testing.T) {
	projectRoot := newProjectRoot(t)
	userConfigPath := setUserConfigRoot(t)
	writeFile(t, userConfigPath, "[project]\nname =\n")

	_, err := Load(projectRoot, Overrides{})
	if err == nil {
		t.Fatal("expected invalid user config error")
	}
	if !strings.Contains(err.Error(), userConfigPath) {
		t.Fatalf("expected error to mention user config path %q, got %v", userConfigPath, err)
	}
}

func TestUserConfigPathHonorsXDGConfigHome(t *testing.T) {
	xdgRoot := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)

	path, err := UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath returned error: %v", err)
	}

	want := filepath.Join(xdgRoot, "utree", "config.toml")
	if path != want {
		t.Fatalf("expected user config path %q, got %q", want, path)
	}
}

func TestLoadConfigWhenProjectConfigMissingUsesDefaults(t *testing.T) {
	projectRoot := newProjectRoot(t)

	cfg, err := Load(projectRoot, Overrides{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected defaults, got %#v", cfg)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".utree", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected config.toml to remain absent, stat error: %v", err)
	}
}

func TestRenderConfigLoadsAsDefaults(t *testing.T) {
	projectRoot := newProjectRoot(t)
	rendered, err := Render(Default())
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), rendered)

	cfg, err := Load(projectRoot, Overrides{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected rendered defaults to load as defaults, got %#v", cfg)
	}
	if !strings.Contains(rendered, "[[layout.default.panes]]") {
		t.Fatalf("expected rendered config to include pane entries, got %q", rendered)
	}
}

func TestApplyCLIOverridesWinsOverProjectConfig(t *testing.T) {
	projectRoot := newProjectRoot(t)
	writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), `
[project]
name = "from-file"

[git]
default_branch = "develop"

[session]
name_template = "{project}:{worktree}"
`)

	projectName := "from-cli"
	defaultBranch := "main"
	sessionTemplate := "{project}__{worktree}"
	cfg, err := Load(projectRoot, Overrides{
		ProjectName:         &projectName,
		GitDefaultBranch:    &defaultBranch,
		SessionNameTemplate: &sessionTemplate,
	})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Project.Name != "from-cli" {
		t.Fatalf("expected CLI project name, got %q", cfg.Project.Name)
	}
	if cfg.Git.DefaultBranch != "main" {
		t.Fatalf("expected CLI default branch, got %q", cfg.Git.DefaultBranch)
	}
	if cfg.Session.NameTemplate != "{project}__{worktree}" {
		t.Fatalf("expected CLI session template, got %q", cfg.Session.NameTemplate)
	}
}

func TestLoadConfigInvalidTOMLReturnsActionableError(t *testing.T) {
	projectRoot := newProjectRoot(t)
	writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), `
[project]
name =
`)

	_, err := Load(projectRoot, Overrides{})
	if err == nil {
		t.Fatal("expected invalid config error")
	}
	if !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("expected error to mention config.toml, got %v", err)
	}
}

func TestLoadRejectsInvalidLayoutPanes(t *testing.T) {
	testCases := []struct {
		name     string
		contents string
		want     string
	}{
		{name: "missing command", contents: `
[layout.default]

[[layout.default.panes]]
selected = true
`, want: "command"},
		{name: "first split", contents: `
[layout.default]

[[layout.default.panes]]
command = "nvim ."
split = "vertical"
`, want: "panes[0].split"},
		{name: "invalid split", contents: `
[layout.default]

[[layout.default.panes]]
command = "nvim ."

[[layout.default.panes]]
command = "git status"
split = "diagonal"
`, want: "split"},
		{name: "multiple selected", contents: `
[layout.default]

[[layout.default.panes]]
command = "nvim ."
selected = true

[[layout.default.panes]]
command = "git status"
split = "vertical"
selected = true
`, want: "only select one"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			projectRoot := newProjectRoot(t)
			writeFile(t, filepath.Join(projectRoot, ".utree", "config.toml"), testCase.contents)

			_, err := Load(projectRoot, Overrides{})
			if err == nil {
				t.Fatal("expected layout validation error")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("expected error to contain %q, got %v", testCase.want, err)
			}
		})
	}
}

func TestValidateSessionTemplateRejectsUnsupportedVariables(t *testing.T) {
	err := ValidateSessionTemplate("{project}:{unknown}")
	if err == nil {
		t.Fatal("expected unsupported variable error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected error to mention unknown, got %v", err)
	}
}

func TestValidateSessionTemplateAcceptsSupportedVariablesAndLiterals(t *testing.T) {
	testCases := []string{
		"{project}:{worktree}",
		"{project}--{worktree}--{branch}",
		"{project}__{worktree}",
		"literal-{project}-literal-{worktree}-{branch}",
	}

	for _, testCase := range testCases {
		t.Run(testCase, func(t *testing.T) {
			if err := ValidateSessionTemplate(testCase); err != nil {
				t.Fatalf("expected valid session template, got %v", err)
			}
		})
	}
}

func newProjectRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))

	projectRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectRoot, ".utree"), 0o755); err != nil {
		t.Fatalf("create .utree: %v", err)
	}
	return projectRoot
}

func setUserConfigRoot(t *testing.T) string {
	t.Helper()

	xdgRoot := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	userConfigPath := filepath.Join(xdgRoot, "utree", "config.toml")
	if err := os.MkdirAll(filepath.Dir(userConfigPath), 0o755); err != nil {
		t.Fatalf("create user config dir: %v", err)
	}
	return userConfigPath
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
