package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
	"github.com/spencerrais/utree/internal/tmux"
)

type NewOptions struct {
	WorktreeName          string
	BranchName            string
	BaseBranch            string
	DefaultBranchOverride *string
}

type NewPlan struct {
	Project                      project.Project
	WorktreeName                 string
	BranchName                   string
	TargetPath                   string
	DefaultBranch                string
	CurrentBranch                string
	StartPoint                   string
	RequiresDefaultBranchWarning bool
	Config                       config.Config
}

type NewDependencies struct {
	DetectProject func(startDir string) (project.Project, error)
	LoadConfig    func(projectRoot string, overrides config.Overrides) (config.Config, error)
	DefaultBranch func(project.Project, string, *string) (string, error)
	CurrentBranch func(dir string) (string, error)
}

type NewExecutionDependencies interface {
	SessionDependencies
	WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error
}

func PlanNew(startDir string, options NewOptions, deps NewDependencies) (NewPlan, error) {
	deps = withDefaultNewDependencies(deps)

	worktreeName := strings.TrimSpace(options.WorktreeName)
	branchName := strings.TrimSpace(options.BranchName)
	if branchName == "" {
		branchName = worktreeName
	}
	if worktreeName == "" || branchName == "" {
		return NewPlan{}, fmt.Errorf("worktree and branch names are required")
	}

	proj, err := deps.DetectProject(startDir)
	if err != nil {
		return NewPlan{}, err
	}
	cfg, err := deps.LoadConfig(proj.Root, config.Overrides{GitDefaultBranch: options.DefaultBranchOverride})
	if err != nil {
		return NewPlan{}, err
	}
	defaultBranch, err := deps.DefaultBranch(proj, cfg.Git.DefaultBranch, options.DefaultBranchOverride)
	if err != nil {
		return NewPlan{}, err
	}
	currentBranch, err := deps.CurrentBranch(proj.GitRoot)
	if err != nil {
		return NewPlan{}, err
	}
	targetPath, err := proj.PlanSiblingWorktreePath(worktreeName)
	if err != nil {
		return NewPlan{}, err
	}

	startPoint := strings.TrimSpace(options.BaseBranch)
	requiresWarning := false
	if startPoint == "" {
		startPoint = defaultBranch
		requiresWarning = currentBranch != "" && currentBranch != defaultBranch
	}

	return NewPlan{
		Project:                      proj,
		WorktreeName:                 worktreeName,
		BranchName:                   branchName,
		TargetPath:                   targetPath,
		DefaultBranch:                defaultBranch,
		CurrentBranch:                currentBranch,
		StartPoint:                   startPoint,
		RequiresDefaultBranchWarning: requiresWarning,
		Config:                       cfg,
	}, nil
}

func ExecuteNew(plan NewPlan, deps NewExecutionDependencies, confirmation io.Reader, stdout io.Writer) (bool, error) {
	if plan.RequiresDefaultBranchWarning {
		if stdout == nil {
			stdout = io.Discard
		}
		if _, err := fmt.Fprintf(stdout, "Current branch is not the detected default branch.\n\nDetected default branch: %s\nCurrent branch:          %s\n\n`ut new` will create the new worktree from `%s`.\n\nUse `--base %s` if you want to branch from the current branch.\n\nContinue? [y/N] ", plan.DefaultBranch, plan.CurrentBranch, plan.DefaultBranch, plan.CurrentBranch); err != nil {
			return false, err
		}
		if !confirmed(confirmation) {
			return false, nil
		}
	}
	session, err := renderNewWorktreeSession(plan)
	if err != nil {
		return false, err
	}
	exists, err := deps.HasSession(session)
	if err != nil {
		return false, err
	}
	if exists {
		return false, fmt.Errorf("tmux session already exists: %s", session)
	}

	if err := deps.WorktreeAddNewBranch(plan.Project.GitRoot, plan.TargetPath, plan.BranchName, plan.StartPoint); err != nil {
		return false, err
	}

	if err := deps.CreateDefaultLayout(session, plan.TargetPath, plan.Config.Layout.Default); err != nil {
		return false, fmt.Errorf("worktree created at %s but tmux open failed: %w", plan.TargetPath, err)
	}
	if err := deps.OpenSession(session, deps.InsideTmux()); err != nil {
		return false, fmt.Errorf("worktree created at %s but tmux open failed: %w", plan.TargetPath, err)
	}
	return true, nil
}

func withDefaultNewDependencies(deps NewDependencies) NewDependencies {
	if deps.DetectProject == nil {
		deps.DetectProject = func(startDir string) (project.Project, error) {
			return project.Detect(startDir, project.GitRevParseRoot)
		}
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = config.Load
	}
	if deps.DefaultBranch == nil {
		deps.DefaultBranch = func(proj project.Project, configDefaultBranch string, override *string) (string, error) {
			return git.ResolveDefaultBranch(git.FallbackMode, git.DefaultBranchOptions{CLIOverride: override, ConfigDefaultBranch: configDefaultBranch}, git.CommandBranchSource{Dir: proj.GitRoot})
		}
	}
	if deps.CurrentBranch == nil {
		deps.CurrentBranch = func(dir string) (string, error) {
			cmd := git.Command(dir, "branch", "--show-current")
			output, err := cmd.Output()
			if err != nil {
				return "", fmt.Errorf("git branch --show-current: %w", err)
			}
			return strings.TrimSpace(string(output)), nil
		}
	}
	return deps
}

func renderNewWorktreeSession(plan NewPlan) (string, error) {
	return tmux.RenderSessionName(plan.Project.Root, plan.Config.Project.Name, plan.WorktreeName, plan.BranchName, plan.Config.Session.NameTemplate)
}

func confirmed(reader io.Reader) bool {
	if reader == nil {
		return false
	}
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

type DefaultNewExecutionDependencies struct {
	TMUX func() string
}

func (d DefaultNewExecutionDependencies) WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error {
	return git.Adapter{Dir: dir}.WorktreeAddNewBranch(path, branch, startPoint)
}

func (d DefaultNewExecutionDependencies) HasSession(session string) (bool, error) {
	return tmux.Adapter{}.HasSession(session)
}

func (d DefaultNewExecutionDependencies) CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error {
	return tmux.Adapter{}.CreateDefaultLayout(session, worktreePath, layout)
}

func (d DefaultNewExecutionDependencies) OpenSession(session string, insideTmux bool) error {
	return tmux.Adapter{}.OpenSession(session, insideTmux)
}

func (d DefaultNewExecutionDependencies) InsideTmux() bool {
	tmuxEnv := ""
	if d.TMUX != nil {
		tmuxEnv = d.TMUX()
	} else {
		tmuxEnv = os.Getenv("TMUX")
	}
	return tmux.IsInsideTmux(tmuxEnv)
}
