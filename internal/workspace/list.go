package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/git"
	"github.com/spencerrais/utree/internal/project"
	"github.com/spencerrais/utree/internal/tmux"
)

type ListPlan struct {
	Project        project.Project
	UtreeWorktrees []ListWorktree
	OtherWorktrees []ListWorktree
}

type ListWorktree struct {
	Name          string
	Branch        string
	Path          string
	Session       string
	SessionExists bool
}

type ListDependencies struct {
	DetectProject func(startDir string) (project.Project, error)
	LoadConfig    func(projectRoot string, overrides config.Overrides) (config.Config, error)
	Worktrees     func(dir string) ([]git.Worktree, error)
	HasSession    func(session string) (bool, error)
}

func PlanList(startDir string, deps ListDependencies) (ListPlan, error) {
	deps = withDefaultListDependencies(deps)

	proj, err := deps.DetectProject(startDir)
	if err != nil {
		return ListPlan{}, err
	}
	cfg, err := deps.LoadConfig(proj.Root, config.Overrides{})
	if err != nil {
		return ListPlan{}, err
	}
	worktrees, err := deps.Worktrees(proj.GitRoot)
	if err != nil {
		return ListPlan{}, err
	}

	projectRoot, err := cleanAbs(proj.Root)
	if err != nil {
		return ListPlan{}, err
	}
	plan := ListPlan{Project: proj}
	for _, worktree := range worktrees {
		row, err := listRow(worktree)
		if err != nil {
			return ListPlan{}, err
		}
		if filepath.Dir(row.Path) != projectRoot {
			plan.OtherWorktrees = append(plan.OtherWorktrees, row)
			continue
		}

		session, err := tmux.RenderSessionName(projectRoot, cfg.Project.Name, row.Name, row.Branch, cfg.Session.NameTemplate)
		if err != nil {
			return ListPlan{}, err
		}
		exists, err := deps.HasSession(session)
		if err != nil {
			return ListPlan{}, err
		}
		row.Session = session
		row.SessionExists = exists
		plan.UtreeWorktrees = append(plan.UtreeWorktrees, row)
	}

	return plan, nil
}

func RenderList(plan ListPlan) string {
	var builder strings.Builder
	builder.WriteString("utree worktrees:\n")
	writer := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "  WORKTREE\tBRANCH\tSESSION")
	for _, worktree := range plan.UtreeWorktrees {
		fmt.Fprintf(writer, "  %s\t%s\t%s\n", worktree.Name, displayBranch(worktree.Branch), displaySession(worktree))
	}
	writer.Flush()

	if len(plan.OtherWorktrees) > 0 {
		builder.WriteString("\nother git worktrees:\n")
		writer = tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)
		fmt.Fprintln(writer, "  WORKTREE\tBRANCH\tPATH")
		for _, worktree := range plan.OtherWorktrees {
			fmt.Fprintf(writer, "  %s\t%s\t%s\n", worktree.Name, displayBranch(worktree.Branch), worktree.Path)
		}
		writer.Flush()
	}

	return builder.String()
}

func withDefaultListDependencies(deps ListDependencies) ListDependencies {
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
	if deps.HasSession == nil {
		deps.HasSession = func(session string) (bool, error) {
			return tmux.Adapter{}.HasSession(session)
		}
	}
	return deps
}

func listRow(worktree git.Worktree) (ListWorktree, error) {
	path, err := cleanAbs(worktree.Path)
	if err != nil {
		return ListWorktree{}, err
	}
	name := worktree.Name
	if name == "" {
		name = filepath.Base(path)
	}
	return ListWorktree{Name: name, Branch: worktree.Branch, Path: path}, nil
}

func displayBranch(branch string) string {
	if strings.TrimSpace(branch) == "" {
		return "-"
	}
	return branch
}

func displaySession(worktree ListWorktree) string {
	if !worktree.SessionExists || strings.TrimSpace(worktree.Session) == "" {
		return "-"
	}
	return worktree.Session
}
