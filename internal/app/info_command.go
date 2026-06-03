package app

import (
	"errors"
	"fmt"
	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
	"github.com/spencerrais/utree/internal/tmux"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (a App) runInfo() error {
	deps := withDefaultInfoDependencies(a.Info)

	gitAvailable := deps.CheckCommand("git")
	tmuxAvailable := deps.CheckCommand("tmux")
	fprintf := func(format string, args ...any) error {
		_, err := fmt.Fprintf(a.Stdout, format, args...)
		return err
	}

	if err := fprintf("git: %s\n", availability(gitAvailable)); err != nil {
		return err
	}
	if err := fprintf("tmux: %s\n", availability(tmuxAvailable)); err != nil {
		return err
	}

	projectRoot := ""
	proj, err := deps.DetectProject()
	if err != nil {
		if !errors.Is(err, project.ErrNotGitRepository) && !errors.Is(err, project.ErrNotUtreeProject) {
			return err
		}
		if err := fprintf("utree project: no\n"); err != nil {
			return err
		}
	} else {
		projectRoot = proj.Root
		if err := writeProjectInfo(fprintf, deps, proj); err != nil {
			return err
		}
	}
	if err := writeInfoConfig(fprintf, deps, projectRoot); err != nil {
		return err
	}

	insideTmux := tmux.IsInsideTmux(deps.TMUX())
	if err := fprintf("inside tmux: %s\n", yesNo(insideTmux)); err != nil {
		return err
	}
	if insideTmux {
		session, err := deps.CurrentSession()
		if err != nil || strings.TrimSpace(session) == "" {
			session = "unavailable"
		}
		if err := fprintf("tmux session: %s\n", strings.TrimSpace(session)); err != nil {
			return err
		}
	}

	return nil
}
func writeProjectInfo(fprintf func(string, ...any) error, deps InfoDependencies, proj project.Project) error {
	if err := fprintf("utree project: yes\n"); err != nil {
		return err
	}
	if err := fprintf("project root: %s\n", proj.Root); err != nil {
		return err
	}
	loaded, err := deps.LoadConfig(proj.Root)
	if err != nil {
		loaded = LoadedConfig{ProjectName: "unavailable", GitDefaultBranch: "auto"}
	}
	projectName := loaded.ProjectName
	if projectName == "" || projectName == "auto" {
		projectName = filepath.Base(proj.Root)
	}
	if err := fprintf("project name: %s\n", projectName); err != nil {
		return err
	}

	defaultBranch, err := deps.DefaultBranch(proj, loaded.GitDefaultBranch)
	if err != nil || strings.TrimSpace(defaultBranch) == "" {
		defaultBranch = "unavailable"
	}
	if err := fprintf("default branch: %s\n", defaultBranch); err != nil {
		return err
	}
	if err := fprintf("current worktree: %s\n", proj.WorktreeName); err != nil {
		return err
	}
	currentBranch, err := deps.CurrentBranch(proj.GitRoot)
	if err != nil || strings.TrimSpace(currentBranch) == "" {
		currentBranch = "unavailable"
	}
	if err := fprintf("current branch: %s\n", strings.TrimSpace(currentBranch)); err != nil {
		return err
	}
	return nil
}
func writeInfoConfig(fprintf func(string, ...any) error, deps InfoDependencies, projectRoot string) error {
	loaded, err := deps.LoadConfig(projectRoot)
	if err != nil {
		return fprintf("active config: unavailable\n")
	}
	activeConfig := strings.TrimSpace(loaded.ActiveConfig)
	if activeConfig == "" {
		activeConfig = "built-in defaults"
	}
	return fprintf("active config: %s\n", activeConfig)
}
func withDefaultInfoDependencies(deps InfoDependencies) InfoDependencies {
	if deps.CheckCommand == nil {
		deps.CheckCommand = defaultCheckCommand
	}
	if deps.DetectProject == nil {
		deps.DetectProject = defaultDetectProject
	}
	if deps.TMUX == nil {
		deps.TMUX = func() string { return os.Getenv("TMUX") }
	}
	if deps.CurrentSession == nil {
		deps.CurrentSession = defaultCurrentTmuxSession
	}
	if deps.CurrentBranch == nil {
		deps.CurrentBranch = defaultCurrentBranch
	}
	if deps.DefaultBranch == nil {
		deps.DefaultBranch = defaultInfoDefaultBranch
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = defaultLoadInfoConfig
	}
	return deps
}
func defaultCheckCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
func defaultDetectProject() (project.Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return project.Project{}, err
	}
	return project.Detect(wd, project.GitRevParseRoot)
}
func defaultLoadInfoConfig(projectRoot string) (LoadedConfig, error) {
	cfg, err := config.Load(projectRoot, config.Overrides{})
	if err != nil {
		return LoadedConfig{}, err
	}
	activeConfig := "built-in defaults"
	userConfigPath, err := config.UserConfigPath()
	if err != nil {
		return LoadedConfig{}, err
	}
	userConfigExists, err := config.FileExists(userConfigPath)
	if err != nil {
		return LoadedConfig{}, err
	}
	if userConfigExists {
		activeConfig = userConfigPath
	}
	if strings.TrimSpace(projectRoot) != "" {
		projectConfigPath := config.ProjectConfigPath(projectRoot)
		projectConfigExists, err := config.FileExists(projectConfigPath)
		if err != nil {
			return LoadedConfig{}, err
		}
		if projectConfigExists {
			activeConfig = projectConfigPath
		}
	}
	return LoadedConfig{
		ProjectName:         cfg.Project.Name,
		GitDefaultBranch:    cfg.Git.DefaultBranch,
		SessionNameTemplate: cfg.Session.NameTemplate,
		ActiveConfig:        activeConfig,
	}, nil
}
func defaultInfoDefaultBranch(proj project.Project, configDefaultBranch string) (string, error) {
	return git.ResolveDefaultBranch(git.FallbackMode, git.DefaultBranchOptions{ConfigDefaultBranch: configDefaultBranch}, git.CommandBranchSource{Dir: proj.GitRoot})
}
func defaultCurrentBranch(dir string) (string, error) {
	cmd := git.Command(dir, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
func defaultCurrentTmuxSession() (string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
func availability(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}
