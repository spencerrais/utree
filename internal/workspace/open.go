package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
	"github.com/spencerrais/utree/internal/tmux"
)

var ErrOpenTargetNotFound = errors.New("open target worktree not found")
var ErrOpenTargetRequired = errors.New("open target is required from project root")

type OpenOptions struct {
	Target string
}

type OpenPlan struct {
	Project      project.Project
	Worktree     git.Worktree
	WorktreeName string
	WorktreePath string
	SessionName  string
	Config       config.Config
}

type OpenDependencies struct {
	DetectProject func(startDir string) (project.Project, error)
	LoadConfig    func(projectRoot string, overrides config.Overrides) (config.Config, error)
	Worktrees     func(dir string) ([]git.Worktree, error)
}

type SessionDependencies interface {
	HasSession(session string) (bool, error)
	CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error
	OpenSession(session string, insideTmux bool) error
	InsideTmux() bool
}

type OpenExecutionDependencies interface {
	SessionDependencies
}

func PlanOpen(startDir string, options OpenOptions, deps OpenDependencies) (OpenPlan, error) {
	deps = withDefaultOpenDependencies(deps)

	proj, err := deps.DetectProject(startDir)
	if err != nil {
		return OpenPlan{}, err
	}
	targetName, err := openTargetName(options.Target, proj.WorktreeName)
	if err != nil {
		return OpenPlan{}, err
	}
	cfg, err := deps.LoadConfig(proj.Root, config.Overrides{})
	if err != nil {
		return OpenPlan{}, err
	}
	worktrees, err := deps.Worktrees(proj.GitRoot)
	if err != nil {
		return OpenPlan{}, err
	}

	worktree, err := findProjectWorktree(proj.Root, targetName, worktrees)
	if err != nil {
		return OpenPlan{}, err
	}
	sessionName, err := tmux.RenderSessionName(proj.Root, cfg.Project.Name, worktree.Name, worktree.Branch, cfg.Session.NameTemplate)
	if err != nil {
		return OpenPlan{}, err
	}

	return OpenPlan{
		Project:      proj,
		Worktree:     worktree,
		WorktreeName: worktree.Name,
		WorktreePath: worktree.Path,
		SessionName:  sessionName,
		Config:       cfg,
	}, nil
}

func ExecuteOpen(plan OpenPlan, deps OpenExecutionDependencies) error {
	return OpenWorktreeSession(plan.SessionName, plan.WorktreePath, plan.Config.Layout.Default, deps)
}

func OpenWorktreeSession(session string, worktreePath string, layout config.DefaultLayoutConfig, deps SessionDependencies) error {
	exists, err := deps.HasSession(session)
	if err != nil {
		return err
	}
	if !exists {
		if err := deps.CreateDefaultLayout(session, worktreePath, layout); err != nil {
			return err
		}
	}
	return deps.OpenSession(session, deps.InsideTmux())
}

func withDefaultOpenDependencies(deps OpenDependencies) OpenDependencies {
	if deps.DetectProject == nil {
		deps.DetectProject = func(startDir string) (project.Project, error) {
			return project.Detect(startDir, project.GitRevParseRoot)
		}
	}
	if deps.LoadConfig == nil {
		deps.LoadConfig = config.Load
	}
	if deps.Worktrees == nil {
		deps.Worktrees = func(dir string) ([]git.Worktree, error) {
			return git.Adapter{Dir: dir}.WorktreeList()
		}
	}
	return deps
}

func openTargetName(target string, currentWorktreeName string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || target == "." {
		if currentWorktreeName == "" {
			return "", ErrOpenTargetRequired
		}
		return currentWorktreeName, nil
	}
	if target == ".." || filepath.IsAbs(target) || filepath.Clean(target) != target || strings.Contains(target, "/") || strings.ContainsRune(target, os.PathSeparator) {
		return "", fmt.Errorf("invalid open target %q", target)
	}
	return target, nil
}

func findProjectWorktree(projectRoot string, targetName string, worktrees []git.Worktree) (git.Worktree, error) {
	projectRoot, err := cleanAbs(projectRoot)
	if err != nil {
		return git.Worktree{}, err
	}
	for _, worktree := range worktrees {
		path, err := cleanAbs(worktree.Path)
		if err != nil {
			return git.Worktree{}, err
		}
		if filepath.Dir(path) != projectRoot {
			continue
		}
		name := worktree.Name
		if name == "" {
			name = filepath.Base(path)
		}
		if name == targetName {
			worktree.Path = path
			worktree.Name = name
			return worktree, nil
		}
	}
	return git.Worktree{}, fmt.Errorf("%w: %s", ErrOpenTargetNotFound, targetName)
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make path absolute %s: %w", path, err)
	}
	return filepath.Clean(abs), nil
}

type DefaultOpenExecutionDependencies struct {
	TMUX func() string
}

func (d DefaultOpenExecutionDependencies) HasSession(session string) (bool, error) {
	return tmux.Adapter{}.HasSession(session)
}

func (d DefaultOpenExecutionDependencies) CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error {
	return tmux.Adapter{}.CreateDefaultLayout(session, worktreePath, layout)
}

func (d DefaultOpenExecutionDependencies) OpenSession(session string, insideTmux bool) error {
	return tmux.Adapter{}.OpenSession(session, insideTmux)
}

func (d DefaultOpenExecutionDependencies) InsideTmux() bool {
	tmuxEnv := ""
	if d.TMUX != nil {
		tmuxEnv = d.TMUX()
	} else {
		tmuxEnv = os.Getenv("TMUX")
	}
	return tmux.IsInsideTmux(tmuxEnv)
}
