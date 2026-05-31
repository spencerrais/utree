package app

import (
	"fmt"
	"io"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/convert"
	"github.com/spencerrais/utree/internal/project"
	"github.com/spencerrais/utree/internal/workspace"
)

const helpText = `Usage: ut <command>

Commands:
  adopt        mark an existing project/worktree layout as a utree project
  convert      convert a single-worktree repo into utree layout
  new          create a sibling git worktree and open it in tmux
  open         open a project worktree in tmux
  list         list project worktrees
  remove       safely remove a worktree
  config info  show config paths, precedence, and effective TOML
  info         show environment and project diagnostics
`

type App struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
	Info    InfoDependencies
	Adopt   AdoptDependencies
	Convert ConvertDependencies
	New     NewDependencies
	Open    OpenDependencies
	List    ListDependencies
	Remove  RemoveDependencies
	Config  ConfigDependencies
}

type InfoDependencies struct {
	CheckCommand   func(name string) bool
	DetectProject  func() (project.Project, error)
	TMUX           func() string
	CurrentSession func() (string, error)
	CurrentBranch  func(dir string) (string, error)
	DefaultBranch  func(project.Project, string) (string, error)
	LoadConfig     func(projectRoot string) (LoadedConfig, error)
}

type ConvertDependencies struct {
	WorkingDir func() (string, error)
	Plan       func(startDir string, options convert.Options) (convert.Plan, error)
	Execute    func(plan convert.Plan, confirmation io.Reader) (bool, error)
}

type AdoptDependencies struct {
	WorkingDir func() (string, error)
	Plan       func(startDir string) (convert.AdoptPlan, error)
	Execute    func(plan convert.AdoptPlan, confirmation io.Reader) (bool, error)
}

type NewDependencies struct {
	WorkingDir func() (string, error)
	Plan       func(startDir string, options workspace.NewOptions) (workspace.NewPlan, error)
	Execute    func(plan workspace.NewPlan, confirmation io.Reader, stdout io.Writer) (bool, error)
}

type OpenDependencies struct {
	WorkingDir func() (string, error)
	Plan       func(startDir string, options workspace.OpenOptions) (workspace.OpenPlan, error)
	Execute    func(plan workspace.OpenPlan) error
}

type ListDependencies struct {
	WorkingDir func() (string, error)
	Plan       func(startDir string) (workspace.ListPlan, error)
	Render     func(plan workspace.ListPlan) string
}

type RemoveDependencies struct {
	WorkingDir func() (string, error)
	Plan       func(startDir string, options workspace.RemoveOptions) (workspace.RemovePlan, error)
	Execute    func(plan workspace.RemovePlan, confirmation io.Reader, stdout io.Writer) (bool, error)
}

type ConfigDependencies struct {
	WorkingDir     func() (string, error)
	DetectProject  func(startDir string) (project.Project, error)
	LoadConfig     func(projectRoot string) (config.Config, error)
	RenderConfig   func(config.Config) (string, error)
	UserConfigPath func() (string, error)
	FileExists     func(path string) (bool, error)
}

type LoadedConfig struct {
	ProjectName         string
	GitDefaultBranch    string
	SessionNameTemplate string
	ActiveConfig        string
}

func (a App) Run(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, helpText)
		return 0
	}
	if args[0] == "info" {
		if len(args) != 1 {
			fmt.Fprintln(a.Stderr, "usage: ut info")
			return 1
		}
		if err := a.runInfo(); err != nil {
			fmt.Fprintf(a.Stderr, "info: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "adopt" {
		if err := a.runAdopt(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "adopt: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "convert" {
		if err := a.runConvert(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "convert: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "new" {
		if err := a.runNew(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "new: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "open" {
		if err := a.runOpen(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "open: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "list" {
		if err := a.runList(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "list: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "remove" {
		if err := a.runRemove(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "remove: %v\n", err)
			return 1
		}
		return 0
	}
	if args[0] == "config" {
		if err := a.runConfig(args[1:]); err != nil {
			fmt.Fprintf(a.Stderr, "config: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(a.Stderr, "unknown command: %s\n\n%s", args[0], helpText)
	return 1
}
