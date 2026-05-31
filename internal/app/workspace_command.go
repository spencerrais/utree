package app

import (
	"errors"
	"fmt"
	"github.com/spencerrais/utree/internal/workspace"
	"io"
	"os"
	"strings"
)

func (a App) runRemove(args []string) error {
	options, err := parseRemoveOptions(args)
	if err != nil {
		return err
	}
	deps := withDefaultRemoveDependencies(a.Remove)
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	plan, err := deps.Plan(startDir, options)
	if err != nil {
		return err
	}
	stdin := a.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	_, err = deps.Execute(plan, stdin, a.Stdout)
	return err
}
func parseRemoveOptions(args []string) (workspace.RemoveOptions, error) {
	var options workspace.RemoveOptions
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--default-branch":
			value, ok := nextFlagValue(args, &i)
			if !ok {
				return workspace.RemoveOptions{}, errors.New("usage: ut remove <worktree-name> [--default-branch <branch>]")
			}
			options.DefaultBranchOverride = &value
		default:
			if strings.HasPrefix(args[i], "--") {
				return workspace.RemoveOptions{}, errors.New("usage: ut remove <worktree-name> [--default-branch <branch>]")
			}
			positionals = append(positionals, args[i])
		}
	}
	if len(positionals) != 1 || strings.TrimSpace(positionals[0]) == "" {
		return workspace.RemoveOptions{}, errors.New("usage: ut remove <worktree-name> [--default-branch <branch>]")
	}
	options.WorktreeName = positionals[0]
	return options, nil
}
func (a App) runList(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ut list")
	}
	deps := withDefaultListDependencies(a.List)
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	plan, err := deps.Plan(startDir)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(a.Stdout, deps.Render(plan))
	return err
}
func (a App) runOpen(args []string) error {
	options, err := parseOpenOptions(args)
	if err != nil {
		return err
	}
	deps := withDefaultOpenDependencies(a.Open)
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	plan, err := deps.Plan(startDir, options)
	if err != nil {
		return err
	}
	return deps.Execute(plan)
}
func parseOpenOptions(args []string) (workspace.OpenOptions, error) {
	if len(args) == 0 {
		return workspace.OpenOptions{}, nil
	}
	if len(args) == 1 && strings.TrimSpace(args[0]) != "" {
		return workspace.OpenOptions{Target: args[0]}, nil
	}
	return workspace.OpenOptions{}, errors.New("usage: ut open [.]|[<worktree-name>]")
}
func (a App) runNew(args []string) error {
	options, err := parseNewOptions(args)
	if err != nil {
		return err
	}
	deps := withDefaultNewDependencies(a.New)
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	plan, err := deps.Plan(startDir, options)
	if err != nil {
		return err
	}
	stdin := a.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	executed, err := deps.Execute(plan, stdin, a.Stdout)
	if err != nil {
		return err
	}
	if !executed {
		_, err = fmt.Fprintln(a.Stdout, "New worktree cancelled")
	}
	return err
}
func parseNewOptions(args []string) (workspace.NewOptions, error) {
	var options workspace.NewOptions
	positionals := []string{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--base":
			value, ok := nextFlagValue(args, &i)
			if !ok {
				return workspace.NewOptions{}, errors.New("usage: ut new <name> [<branch-name>] [--base <base-branch>] [--default-branch <branch>]")
			}
			options.BaseBranch = value
		case "--default-branch":
			value, ok := nextFlagValue(args, &i)
			if !ok {
				return workspace.NewOptions{}, errors.New("usage: ut new <name> [<branch-name>] [--base <base-branch>] [--default-branch <branch>]")
			}
			options.DefaultBranchOverride = &value
		default:
			if strings.HasPrefix(args[i], "--") {
				return workspace.NewOptions{}, errors.New("usage: ut new <name> [<branch-name>] [--base <base-branch>] [--default-branch <branch>]")
			}
			positionals = append(positionals, args[i])
		}
	}

	if len(positionals) < 1 || len(positionals) > 2 || strings.TrimSpace(positionals[0]) == "" {
		return workspace.NewOptions{}, errors.New("usage: ut new <name> [<branch-name>] [--base <base-branch>] [--default-branch <branch>]")
	}
	options.WorktreeName = positionals[0]
	if len(positionals) == 2 {
		if strings.TrimSpace(positionals[1]) == "" {
			return workspace.NewOptions{}, errors.New("usage: ut new <name> [<branch-name>] [--base <base-branch>] [--default-branch <branch>]")
		}
		options.BranchName = positionals[1]
	} else {
		options.BranchName = positionals[0]
	}
	return options, nil
}
func nextFlagValue(args []string, index *int) (string, bool) {
	if *index+1 >= len(args) {
		return "", false
	}
	value := strings.TrimSpace(args[*index+1])
	if value == "" || strings.HasPrefix(value, "--") {
		return "", false
	}
	(*index)++
	return value, true
}
func withDefaultNewDependencies(deps NewDependencies) NewDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.Plan == nil {
		deps.Plan = defaultPlanNew
	}
	if deps.Execute == nil {
		deps.Execute = defaultExecuteNew
	}
	return deps
}
func withDefaultOpenDependencies(deps OpenDependencies) OpenDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.Plan == nil {
		deps.Plan = defaultPlanOpen
	}
	if deps.Execute == nil {
		deps.Execute = defaultExecuteOpen
	}
	return deps
}
func withDefaultListDependencies(deps ListDependencies) ListDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.Plan == nil {
		deps.Plan = defaultPlanList
	}
	if deps.Render == nil {
		deps.Render = workspace.RenderList
	}
	return deps
}
func withDefaultRemoveDependencies(deps RemoveDependencies) RemoveDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.Plan == nil {
		deps.Plan = defaultPlanRemove
	}
	if deps.Execute == nil {
		deps.Execute = defaultExecuteRemove
	}
	return deps
}
func defaultPlanNew(startDir string, options workspace.NewOptions) (workspace.NewPlan, error) {
	return workspace.PlanNew(startDir, options, workspace.NewDependencies{})
}
func defaultExecuteNew(plan workspace.NewPlan, confirmation io.Reader, stdout io.Writer) (bool, error) {
	return workspace.ExecuteNew(plan, workspace.DefaultNewExecutionDependencies{}, confirmation, stdout)
}
func defaultPlanOpen(startDir string, options workspace.OpenOptions) (workspace.OpenPlan, error) {
	return workspace.PlanOpen(startDir, options, workspace.OpenDependencies{})
}
func defaultExecuteOpen(plan workspace.OpenPlan) error {
	return workspace.ExecuteOpen(plan, workspace.DefaultOpenExecutionDependencies{})
}
func defaultPlanList(startDir string) (workspace.ListPlan, error) {
	return workspace.PlanList(startDir, workspace.ListDependencies{})
}
func defaultPlanRemove(startDir string, options workspace.RemoveOptions) (workspace.RemovePlan, error) {
	return workspace.PlanRemove(startDir, options, nil)
}
func defaultExecuteRemove(plan workspace.RemovePlan, confirmation io.Reader, stdout io.Writer) (bool, error) {
	return workspace.ExecuteRemove(plan, workspace.DefaultRemoveExecutionDependencies{Project: plan.Project, GitDir: plan.GitCommandDir}, confirmation, stdout)
}
