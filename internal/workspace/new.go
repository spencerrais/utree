package workspace

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Detach                bool
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
	UseExistingBranch            bool
	Detach                       bool
	RequiresDefaultBranchWarning bool
	HasEnvFile                   bool
	EnvFilePath                  string
	EnvFileWorktreeName          string
	Config                       config.Config
}

type NewDependencies struct {
	DetectProject     func(startDir string) (project.Project, error)
	LoadConfig        func(projectRoot string, overrides config.Overrides) (config.Config, error)
	DefaultBranch     func(project.Project, string, *string) (string, error)
	CurrentBranch     func(dir string) (string, error)
	Worktrees         func(dir string) ([]git.Worktree, error)
	LocalBranchExists func(dir string, branch string) (bool, error)
	FileExists        func(path string) (bool, error)
}

type NewExecutionDependencies interface {
	SessionDependencies
	WorktreeAdd(dir string, path string, branch string) error
	WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error
	CopyFile(source string, target string) error
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
	useExistingBranch, err := deps.LocalBranchExists(proj.GitRoot, branchName)
	if err != nil {
		return NewPlan{}, err
	}
	envFilePath, envWorktreeName, hasEnvFile, err := planNewEnvSource(proj, defaultBranch, deps)
	if err != nil {
		return NewPlan{}, err
	}

	return NewPlan{
		Project:                      proj,
		WorktreeName:                 worktreeName,
		BranchName:                   branchName,
		TargetPath:                   targetPath,
		DefaultBranch:                defaultBranch,
		CurrentBranch:                currentBranch,
		StartPoint:                   startPoint,
		UseExistingBranch:            useExistingBranch,
		Detach:                       options.Detach,
		RequiresDefaultBranchWarning: requiresWarning,
		HasEnvFile:                   hasEnvFile,
		EnvFilePath:                  envFilePath,
		EnvFileWorktreeName:          envWorktreeName,
		Config:                       cfg,
	}, nil
}

func planNewEnvSource(proj project.Project, defaultBranch string, deps NewDependencies) (string, string, bool, error) {
	if strings.TrimSpace(proj.WorktreeName) != "" {
		return planEnvFileAtRoot(proj.GitRoot, proj.WorktreeName, deps)
	}

	name := filepath.Base(proj.GitRoot)
	if path, sourceName, ok, err := planEnvFileAtRoot(proj.GitRoot, name, deps); err != nil || ok {
		return path, sourceName, ok, err
	}

	worktrees, err := deps.Worktrees(proj.GitRoot)
	if err != nil {
		return "", "", false, err
	}
	projectRoot, err := cleanAbs(proj.Root)
	if err != nil {
		return "", "", false, err
	}
	for _, worktree := range worktrees {
		path, err := cleanAbs(worktree.Path)
		if err != nil {
			return "", "", false, err
		}
		if worktree.Branch != defaultBranch || filepath.Dir(path) != projectRoot {
			continue
		}
		sourceName := worktree.Name
		if strings.TrimSpace(sourceName) == "" {
			sourceName = filepath.Base(path)
		}
		return planEnvFileAtRoot(path, sourceName, deps)
	}
	return "", "", false, nil
}

func planEnvFileAtRoot(root string, name string, deps NewDependencies) (string, string, bool, error) {
	if strings.TrimSpace(root) == "" {
		return "", "", false, nil
	}
	envPath := filepath.Join(root, ".env")
	exists, err := deps.FileExists(envPath)
	if err != nil {
		return "", "", false, err
	}
	if !exists {
		return "", "", false, nil
	}
	return envPath, name, true, nil
}

func ExecuteNew(plan NewPlan, deps NewExecutionDependencies, confirmation io.Reader, stdout io.Writer) (bool, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	confirmationScanner := bufio.NewScanner(strings.NewReader(""))
	if confirmation != nil {
		confirmationScanner = bufio.NewScanner(confirmation)
	}
	if plan.RequiresDefaultBranchWarning {
		if _, err := fmt.Fprintf(stdout, "Current branch is not the detected default branch.\n\nDetected default branch: %s\nCurrent branch:          %s\n\n`ut new` will create the new worktree from `%s`.\n\nUse `--base %s` if you want to branch from the current branch.\n\nContinue? [y/N] ", plan.DefaultBranch, plan.CurrentBranch, plan.DefaultBranch, plan.CurrentBranch); err != nil {
			return false, err
		}
		if !confirmedScan(confirmationScanner) {
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

	if plan.UseExistingBranch {
		err = deps.WorktreeAdd(plan.Project.GitRoot, plan.TargetPath, plan.BranchName)
	} else {
		err = deps.WorktreeAddNewBranch(plan.Project.GitRoot, plan.TargetPath, plan.BranchName, plan.StartPoint)
	}
	if err != nil {
		return false, err
	}
	if plan.HasEnvFile {
		if _, err := fmt.Fprintf(stdout, "Copy .env from %s to new worktree? [y/N] ", plan.EnvFileWorktreeName); err != nil {
			return true, err
		}
		if confirmedScan(confirmationScanner) {
			if err := deps.CopyFile(plan.EnvFilePath, filepath.Join(plan.TargetPath, ".env")); err != nil {
				return true, fmt.Errorf("worktree created at %s but .env copy failed: %w", plan.TargetPath, err)
			}
		}
	}

	if err := deps.CreateDefaultLayout(session, plan.TargetPath, plan.Config.Layout.Default); err != nil {
		return false, fmt.Errorf("worktree created at %s but tmux open failed: %w", plan.TargetPath, err)
	}
	if plan.Detach {
		return true, nil
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
	if deps.Worktrees == nil {
		deps.Worktrees = func(dir string) ([]git.Worktree, error) {
			return git.Adapter{Dir: dir}.WorktreeList()
		}
	}
	if deps.LocalBranchExists == nil {
		deps.LocalBranchExists = func(dir string, branch string) (bool, error) {
			return git.Adapter{Dir: dir}.LocalBranchExists(branch)
		}
	}
	if deps.FileExists == nil {
		deps.FileExists = fileExists
	}
	return deps
}

func renderNewWorktreeSession(plan NewPlan) (string, error) {
	return tmux.RenderSessionName(plan.Project.Root, plan.Config.Project.Name, plan.WorktreeName, plan.BranchName, plan.Config.Session.NameTemplate)
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func copyFile(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source file %s: %w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("source file is a directory: %s", source)
	}

	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", source, err)
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("target file already exists: %s", target)
		}
		return fmt.Errorf("create target file %s: %w", target, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(target)
		return fmt.Errorf("copy %s to %s: %w", source, target, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("close target file %s: %w", target, err)
	}
	return nil
}

type DefaultNewExecutionDependencies struct {
	TMUX func() string
}

func (d DefaultNewExecutionDependencies) WorktreeAdd(dir string, path string, branch string) error {
	return git.Adapter{Dir: dir}.WorktreeAdd(path, branch)
}

func (d DefaultNewExecutionDependencies) WorktreeAddNewBranch(dir string, path string, branch string, startPoint string) error {
	return git.Adapter{Dir: dir}.WorktreeAddNewBranch(path, branch, startPoint)
}

func (d DefaultNewExecutionDependencies) CopyFile(source string, target string) error {
	return copyFile(source, target)
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
