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

const fallbackSessionName = "utree"

type RemoveOptions struct {
	WorktreeName          string
	DefaultBranchOverride *string
}

type RemovePlan struct {
	Project       project.Project
	Worktree      git.Worktree
	WorktreeName  string
	WorktreePath  string
	BranchName    string
	SessionName   string
	GitCommandDir string
	Safety        RemovalSafety
	Config        config.Config
}

type RemoveDependencies interface {
	DetectProject(startDir string) (project.Project, error)
	loadConfig(projectRoot string, overrides config.Overrides) (config.Config, error)
	Worktrees(dir string) ([]git.Worktree, error)
	AssessSafety(proj project.Project, worktree git.Worktree, cfg config.Config, options SafetyOptions) (RemovalSafety, error)
}

type RemoveExecutionDependencies interface {
	HasSession(session string) (bool, error)
	KillSession(session string) error
	WorktreeRemove(path string) error
	DeleteLocalBranch(branch string) error
	ForceDeleteLocalBranch(branch string) error
	InsideTmux() bool
	CurrentSession() (string, error)
	SwitchToAdjacentSession(current string) (bool, error)
	OpenFallbackSession(session string, dir string) error
	HomeDir() string
}

func PlanRemove(startDir string, options RemoveOptions, deps RemoveDependencies) (RemovePlan, error) {
	deps = withDefaultRemoveDependencies(deps)
	targetName, err := removeTargetName(options.WorktreeName)
	if err != nil {
		return RemovePlan{}, err
	}
	proj, err := deps.DetectProject(startDir)
	if err != nil {
		return RemovePlan{}, err
	}
	cfg, err := deps.loadConfig(proj.Root, config.Overrides{GitDefaultBranch: options.DefaultBranchOverride})
	if err != nil {
		return RemovePlan{}, err
	}
	worktrees, err := deps.Worktrees(proj.GitRoot)
	if err != nil {
		return RemovePlan{}, err
	}
	worktree, err := findProjectWorktree(proj.Root, targetName, worktrees)
	if err != nil {
		return RemovePlan{}, fmt.Errorf("remove target worktree not found: %s", targetName)
	}
	sessionName, err := tmux.RenderSessionName(proj.Root, cfg.Project.Name, worktree.Name, worktree.Branch, cfg.Session.NameTemplate)
	if err != nil {
		return RemovePlan{}, err
	}
	safety, err := deps.AssessSafety(proj, worktree, cfg, SafetyOptions{DefaultBranchOverride: options.DefaultBranchOverride})
	if err != nil {
		return RemovePlan{}, err
	}
	if safety.Kind == SafetyDirty {
		return RemovePlan{}, dirtyRemoveError(worktree.Name, safety.Status)
	}
	if safety.Kind == SafetyNoLocalBranch {
		return RemovePlan{}, fmt.Errorf("cannot remove worktree %q: associated local branch not found", worktree.Name)
	}

	return RemovePlan{
		Project:       proj,
		Worktree:      worktree,
		WorktreeName:  worktree.Name,
		WorktreePath:  worktree.Path,
		BranchName:    worktree.Branch,
		SessionName:   sessionName,
		GitCommandDir: removeGitCommandDir(proj, worktree, worktrees),
		Safety:        safety,
		Config:        cfg,
	}, nil
}

func ExecuteRemove(plan RemovePlan, deps RemoveExecutionDependencies, confirmation io.Reader, stdout io.Writer) (bool, error) {
	if deps == nil {
		deps = DefaultRemoveExecutionDependencies{Project: plan.Project, GitDir: plan.GitCommandDir}
	}
	if stdout == nil {
		stdout = io.Discard
	}
	confirmationScanner := bufio.NewScanner(strings.NewReader(""))
	if confirmation != nil {
		confirmationScanner = bufio.NewScanner(confirmation)
	}
	if plan.Safety.Kind == SafetyDirty {
		return false, dirtyRemoveError(plan.WorktreeName, plan.Safety.Status)
	}
	if plan.Safety.Kind == SafetyNoLocalBranch {
		return false, fmt.Errorf("cannot remove worktree %q: associated local branch not found", plan.WorktreeName)
	}

	deleteUnmergedBranch := false
	if plan.Safety.Kind == SafetyCleanUnmerged {
		if err := writeUnmergedRemovalPrompt(stdout, plan); err != nil {
			return false, err
		}
		if !confirmedScan(confirmationScanner) {
			return false, nil
		}
		if _, err := fmt.Fprintf(stdout, "Delete unmerged local branch '%s'? [y/N] ", plan.BranchName); err != nil {
			return false, err
		}
		deleteUnmergedBranch = confirmedScan(confirmationScanner)
	} else if plan.Safety.Kind == SafetyCleanMerged {
		if err := writeMergedRemovalPrompt(stdout, plan); err != nil {
			return false, err
		}
		if !confirmedScan(confirmationScanner) {
			return false, nil
		}
	}

	sessionExists, err := prepareRemoveSession(plan, deps)
	if err != nil {
		return false, err
	}
	if err := deps.WorktreeRemove(plan.WorktreePath); err != nil {
		return false, err
	}

	if plan.Safety.Kind == SafetyCleanMerged {
		if err := deps.DeleteLocalBranch(plan.BranchName); err != nil {
			return true, fmt.Errorf("worktree removed at %s but branch deletion failed for %s: %w", plan.WorktreePath, plan.BranchName, err)
		}
		return true, finishRemoveSession(plan, deps, sessionExists)
	}

	if deleteUnmergedBranch {
		if err := deps.ForceDeleteLocalBranch(plan.BranchName); err != nil {
			return true, fmt.Errorf("worktree removed at %s but branch deletion failed for %s: %w", plan.WorktreePath, plan.BranchName, err)
		}
	}
	return true, finishRemoveSession(plan, deps, sessionExists)
}

func confirmedScan(scanner *bufio.Scanner) bool {
	if scanner == nil || !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}

func withDefaultRemoveDependencies(deps RemoveDependencies) RemoveDependencies {
	if deps != nil {
		return deps
	}
	return defaultRemoveDependencies{}
}

type defaultRemoveDependencies struct{}

func (d defaultRemoveDependencies) DetectProject(startDir string) (project.Project, error) {
	return project.Detect(startDir, project.GitRevParseRoot)
}

func (d defaultRemoveDependencies) loadConfig(projectRoot string, overrides config.Overrides) (config.Config, error) {
	return config.Load(projectRoot, overrides)
}

func (d defaultRemoveDependencies) Worktrees(dir string) ([]git.Worktree, error) {
	return git.Adapter{Dir: dir}.WorktreeList()
}

func (d defaultRemoveDependencies) AssessSafety(proj project.Project, worktree git.Worktree, cfg config.Config, options SafetyOptions) (RemovalSafety, error) {
	return AssessRemovalSafety(proj, worktree, cfg, options, nil)
}

func removeTargetName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("worktree name is required")
	}
	return openTargetName(name, "")
}

func removeGitCommandDir(proj project.Project, target git.Worktree, worktrees []git.Worktree) string {
	for _, worktree := range worktrees {
		if worktree.Path != "" && worktree.Path != target.Path {
			return worktree.Path
		}
	}
	return proj.GitRoot
}

func dirtyRemoveError(worktreeName string, status WorktreeStatus) error {
	lines := []string{}
	if status.HasUnstagedChanges {
		lines = append(lines, "  unstaged changes  yes")
	}
	if status.HasStagedChanges {
		lines = append(lines, "  staged changes    yes")
	}
	if status.HasUntrackedFiles {
		lines = append(lines, "  untracked files   yes")
	}
	if len(lines) == 0 {
		return fmt.Errorf("cannot remove worktree %q\nworktree has local changes", worktreeName)
	}
	return fmt.Errorf("cannot remove worktree %q\nworktree has local changes\n%s", worktreeName, strings.Join(lines, "\n"))
}

func writeUnmergedRemovalPrompt(stdout io.Writer, plan RemovePlan) error {
	_, err := fmt.Fprintf(stdout, "Branch '%s' does not appear to be merged into '%s'.\n\nRemoving the worktree is safe only if you no longer need this checkout.\nAny unmerged commits will remain reachable from the local branch unless you also delete it.\n\nRemove worktree? [y/N] ", plan.BranchName, plan.Safety.DefaultBranch)
	return err
}

func writeMergedRemovalPrompt(stdout io.Writer, plan RemovePlan) error {
	_, err := fmt.Fprintf(stdout, "Remove worktree %q and delete local branch %q? [y/N] ", plan.WorktreeName, plan.BranchName)
	return err
}

func prepareRemoveSession(plan RemovePlan, deps RemoveExecutionDependencies) (bool, error) {
	exists, err := deps.HasSession(plan.SessionName)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if deps.InsideTmux() {
		current, err := deps.CurrentSession()
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(current) == plan.SessionName {
			switched, err := deps.SwitchToAdjacentSession(current)
			if err != nil {
				return false, err
			}
			if !switched {
				if err := deps.OpenFallbackSession(fallbackSessionName, deps.HomeDir()); err != nil {
					return false, err
				}
			}
		}
	}
	return true, nil
}

func finishRemoveSession(plan RemovePlan, deps RemoveExecutionDependencies, sessionExists bool) error {
	if !sessionExists {
		return nil
	}
	return deps.KillSession(plan.SessionName)
}

type DefaultRemoveExecutionDependencies struct {
	Project project.Project
	GitDir  string
	TMUX    func() string
	Home    func() string
}

func (d DefaultRemoveExecutionDependencies) HasSession(session string) (bool, error) {
	return tmux.Adapter{}.HasSession(session)
}

func (d DefaultRemoveExecutionDependencies) KillSession(session string) error {
	return tmux.Adapter{}.KillSession(session)
}

func (d DefaultRemoveExecutionDependencies) WorktreeRemove(path string) error {
	return git.Adapter{Dir: d.gitDir()}.WorktreeRemove(path)
}

func (d DefaultRemoveExecutionDependencies) DeleteLocalBranch(branch string) error {
	return git.Adapter{Dir: d.gitDir()}.DeleteLocalBranch(branch)
}

func (d DefaultRemoveExecutionDependencies) ForceDeleteLocalBranch(branch string) error {
	return git.Adapter{Dir: d.gitDir()}.ForceDeleteLocalBranch(branch)
}

func (d DefaultRemoveExecutionDependencies) gitDir() string {
	if strings.TrimSpace(d.GitDir) != "" {
		return d.GitDir
	}
	return d.Project.GitRoot
}

func (d DefaultRemoveExecutionDependencies) InsideTmux() bool {
	if d.TMUX != nil {
		return tmux.IsInsideTmux(d.TMUX())
	}
	return tmux.IsInsideTmux(os.Getenv("TMUX"))
}

func (d DefaultRemoveExecutionDependencies) CurrentSession() (string, error) {
	return tmux.Adapter{}.CurrentSession()
}

func (d DefaultRemoveExecutionDependencies) SwitchToAdjacentSession(current string) (bool, error) {
	return tmux.Adapter{}.SwitchToAdjacentSession(current)
}

func (d DefaultRemoveExecutionDependencies) OpenFallbackSession(session string, dir string) error {
	adapter := tmux.Adapter{}
	exists, err := adapter.HasSession(session)
	if err != nil {
		return err
	}
	if !exists {
		if err := adapter.CreateFallbackSession(session, dir); err != nil {
			return err
		}
	}
	return adapter.OpenSession(session, d.InsideTmux())
}

func (d DefaultRemoveExecutionDependencies) HomeDir() string {
	if d.Home != nil {
		return d.Home()
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "."
	}
	return home
}
