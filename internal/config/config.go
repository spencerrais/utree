package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

const utreeDirName = ".utree"

const DefaultSessionNameTemplate = "{project}--{worktree}--{branch}"
const DefaultEditorCommand = "nvim .; exec ${SHELL:-/bin/sh} -l"
const DefaultStatusCommand = "git status; exec ${SHELL:-/bin/sh} -l"
const SplitVertical = "vertical"
const SplitHorizontal = "horizontal"

var sessionTemplateVariablePattern = regexp.MustCompile(`\{([^{}]+)\}`)

type Config struct {
	Project ProjectConfig
	Git     GitConfig
	Session SessionConfig
	Layout  LayoutConfig
}

type ProjectConfig struct {
	Name string
}

type GitConfig struct {
	DefaultBranch string
}

type SessionConfig struct {
	NameTemplate string
}

type LayoutConfig struct {
	Default DefaultLayoutConfig
}

type DefaultLayoutConfig struct {
	Panes []PaneConfig
}

type PaneConfig struct {
	Command  string
	Split    string
	Size     string
	Selected bool
}

type Overrides struct {
	ProjectName         *string
	GitDefaultBranch    *string
	SessionNameTemplate *string
}

func Default() Config {
	return Config{
		Project: ProjectConfig{
			Name: "auto",
		},
		Git: GitConfig{
			DefaultBranch: "auto",
		},
		Session: SessionConfig{
			NameTemplate: DefaultSessionNameTemplate,
		},
		Layout: LayoutConfig{
			Default: DefaultLayoutConfig{
				Panes: []PaneConfig{
					{Command: DefaultEditorCommand, Selected: true},
					{Command: DefaultStatusCommand, Split: SplitVertical, Size: "33%"},
				},
			},
		},
	}
}

func Render(cfg Config) (string, error) {
	var builder bytes.Buffer
	encoder := toml.NewEncoder(&builder)
	if err := encoder.Encode(toConfigFile(cfg)); err != nil {
		return "", fmt.Errorf("render config: %w", err)
	}
	return builder.String(), nil
}

func UserConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(configDir, "utree", "config.toml"), nil
}

func ProjectConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, utreeDirName, "config.toml")
}

func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func Load(projectRoot string, overrides Overrides) (Config, error) {
	cfg := Default()

	userConfigPath, err := UserConfigPath()
	if err != nil {
		return Config{}, err
	}
	if err := applyConfigFileIfExists(&cfg, userConfigPath); err != nil {
		return Config{}, err
	}

	if strings.TrimSpace(projectRoot) != "" {
		if err := applyConfigFileIfExists(&cfg, ProjectConfigPath(projectRoot)); err != nil {
			return Config{}, err
		}
	}

	applyOverrides(&cfg, overrides)

	if err := ValidateSessionTemplate(cfg.Session.NameTemplate); err != nil {
		return Config{}, err
	}
	if err := ValidateLayout(cfg.Layout.Default); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func applyConfigFileIfExists(cfg *Config, configPath string) error {
	exists, err := FileExists(configPath)
	if err != nil {
		return fmt.Errorf("stat config %s: %w", configPath, err)
	}
	if !exists {
		return nil
	}
	fileConfig := configFile{}
	if _, err := toml.DecodeFile(configPath, &fileConfig); err != nil {
		return fmt.Errorf("load config %s: %w", configPath, err)
	}
	applyFileConfig(cfg, fileConfig)
	return nil
}

func ValidateLayout(layout DefaultLayoutConfig) error {
	if len(layout.Panes) == 0 {
		return fmt.Errorf("layout.default.panes must contain at least one pane")
	}
	selectedCount := 0
	for i, pane := range layout.Panes {
		if pane.Selected {
			selectedCount++
		}
		if pane.Command == "" {
			return fmt.Errorf("layout.default.panes[%d].command is required", i)
		}
		if i == 0 {
			if pane.Split != "" {
				return fmt.Errorf("layout.default.panes[0].split is not supported")
			}
			if pane.Size != "" {
				return fmt.Errorf("layout.default.panes[0].size is not supported")
			}
			continue
		}
		if pane.Split != SplitVertical && pane.Split != SplitHorizontal {
			return fmt.Errorf("layout.default.panes[%d].split must be %q or %q", i, SplitVertical, SplitHorizontal)
		}
	}
	if selectedCount > 1 {
		return fmt.Errorf("layout.default.panes can only select one pane")
	}
	return nil
}

func ValidateSessionTemplate(template string) error {
	matches := sessionTemplateVariablePattern.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		variable := match[1]
		if variable != "project" && variable != "worktree" && variable != "branch" {
			return fmt.Errorf("unsupported session template variable %q", variable)
		}
	}

	return nil
}

type configFile struct {
	Project *projectConfigFile `toml:"project"`
	Git     *gitConfigFile     `toml:"git"`
	Session *sessionConfigFile `toml:"session"`
	Layout  *layoutConfigFile  `toml:"layout"`
}

type projectConfigFile struct {
	Name *string `toml:"name"`
}

type gitConfigFile struct {
	DefaultBranch *string `toml:"default_branch"`
}

type sessionConfigFile struct {
	NameTemplate *string `toml:"name_template"`
}

type layoutConfigFile struct {
	Default *defaultLayoutConfigFile `toml:"default"`
}

type defaultLayoutConfigFile struct {
	Panes []paneConfigFile `toml:"panes"`
}

type paneConfigFile struct {
	Command  *string `toml:"command"`
	Split    *string `toml:"split"`
	Size     *string `toml:"size"`
	Selected *bool   `toml:"selected"`
}

func applyFileConfig(cfg *Config, fileConfig configFile) {
	if fileConfig.Project != nil && fileConfig.Project.Name != nil {
		cfg.Project.Name = *fileConfig.Project.Name
	}
	if fileConfig.Git != nil && fileConfig.Git.DefaultBranch != nil {
		cfg.Git.DefaultBranch = *fileConfig.Git.DefaultBranch
	}
	if fileConfig.Session != nil && fileConfig.Session.NameTemplate != nil {
		cfg.Session.NameTemplate = *fileConfig.Session.NameTemplate
	}
	if fileConfig.Layout != nil && fileConfig.Layout.Default != nil {
		applyDefaultLayoutFileConfig(cfg, *fileConfig.Layout.Default)
	}
}

func applyDefaultLayoutFileConfig(cfg *Config, fileConfig defaultLayoutConfigFile) {
	if fileConfig.Panes != nil {
		panes := make([]PaneConfig, 0, len(fileConfig.Panes))
		for _, paneFile := range fileConfig.Panes {
			pane := PaneConfig{}
			if paneFile.Command != nil {
				pane.Command = *paneFile.Command
			}
			if paneFile.Split != nil {
				pane.Split = *paneFile.Split
			}
			if paneFile.Size != nil {
				pane.Size = *paneFile.Size
			}
			if paneFile.Selected != nil {
				pane.Selected = *paneFile.Selected
			}
			panes = append(panes, pane)
		}
		cfg.Layout.Default.Panes = panes
	}
}

func applyOverrides(cfg *Config, overrides Overrides) {
	if overrides.ProjectName != nil {
		cfg.Project.Name = *overrides.ProjectName
	}
	if overrides.GitDefaultBranch != nil {
		cfg.Git.DefaultBranch = *overrides.GitDefaultBranch
	}
	if overrides.SessionNameTemplate != nil {
		cfg.Session.NameTemplate = *overrides.SessionNameTemplate
	}
}

func toConfigFile(cfg Config) configFile {
	return configFile{
		Project: &projectConfigFile{Name: &cfg.Project.Name},
		Git:     &gitConfigFile{DefaultBranch: &cfg.Git.DefaultBranch},
		Session: &sessionConfigFile{NameTemplate: &cfg.Session.NameTemplate},
		Layout:  &layoutConfigFile{Default: &defaultLayoutConfigFile{Panes: toPaneConfigFiles(cfg.Layout.Default.Panes)}},
	}
}

func toPaneConfigFiles(panes []PaneConfig) []paneConfigFile {
	files := make([]paneConfigFile, 0, len(panes))
	for _, pane := range panes {
		paneFile := paneConfigFile{Command: &pane.Command}
		if pane.Split != "" {
			paneFile.Split = &pane.Split
		}
		if pane.Size != "" {
			paneFile.Size = &pane.Size
		}
		if pane.Selected {
			paneFile.Selected = &pane.Selected
		}
		files = append(files, paneFile)
	}
	return files
}
