package app

import (
	"errors"
	"fmt"
	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/project"
	"os"
)

func (a App) runConfig(args []string) error {
	if len(args) != 1 || args[0] != "info" {
		return errors.New("usage: ut config info")
	}

	deps := withDefaultConfigDependencies(a.Config)
	userConfigPath, err := deps.UserConfigPath()
	if err != nil {
		return err
	}
	userConfigExists, err := deps.FileExists(userConfigPath)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "User config: %s\n", userConfigPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "User config exists: %s\n\n", yesNo(userConfigExists)); err != nil {
		return err
	}

	projectRoot := ""
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	proj, err := deps.DetectProject(startDir)
	insideProject := err == nil
	if err != nil && !errors.Is(err, project.ErrNotGitRepository) && !errors.Is(err, project.ErrNotUtreeProject) {
		return err
	}
	if insideProject {
		projectRoot = proj.Root
		projectConfigPath := config.ProjectConfigPath(proj.Root)
		projectConfigExists, err := deps.FileExists(projectConfigPath)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(a.Stdout, "Project config: %s\n", projectConfigPath); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(a.Stdout, "Project config exists: %s\n\n", yesNo(projectConfigExists)); err != nil {
			return err
		}
	} else if _, err := fmt.Fprint(a.Stdout, "Project config: unavailable outside a utree project\n\n"); err != nil {
		return err
	}

	cfg, err := deps.LoadConfig(projectRoot)
	if err != nil {
		return err
	}
	contents, err := deps.RenderConfig(cfg)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(a.Stdout, "Config precedence:"); err != nil {
		return err
	}
	for _, source := range []string{"built-in defaults", "user config"} {
		if _, err := fmt.Fprintf(a.Stdout, "  %s\n", source); err != nil {
			return err
		}
	}
	if insideProject {
		if _, err := fmt.Fprintln(a.Stdout, "  project config"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(a.Stdout, "  CLI overrides"); err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "\nEffective config:\n\n%s", contents)
	return err
}
func withDefaultConfigDependencies(deps ConfigDependencies) ConfigDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.DetectProject == nil {
		deps.DetectProject = func(startDir string) (project.Project, error) {
			return project.Detect(startDir, project.GitRevParseRoot)
		}
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = func(projectRoot string) (config.Config, error) {
			return config.Load(projectRoot, config.Overrides{})
		}
	}
	if deps.RenderConfig == nil {
		deps.RenderConfig = config.Render
	}
	if deps.UserConfigPath == nil {
		deps.UserConfigPath = config.UserConfigPath
	}
	if deps.FileExists == nil {
		deps.FileExists = config.FileExists
	}
	return deps
}
