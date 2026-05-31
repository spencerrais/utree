package app

import (
	"errors"
	"fmt"
	"github.com/spencerrais/utree/internal/config"
	"github.com/spencerrais/utree/internal/convert"
	"os"
	"strings"
)

func (a App) runConvert(args []string) error {
	options, err := parseConvertOptions(args)
	if err != nil {
		return err
	}
	deps := withDefaultConvertDependencies(a.Convert)
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	plan, err := deps.Plan(startDir, options)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Convert repository layout:\n\n  from: %s\n  to:   %s\n\nDetected default branch:\n\n  %s\n\nFuture worktrees will be created under:\n\n  %s%c\n\nProject marker will be created at:\n\n  %s\n\nOptional project config can live at:\n\n  %s\n\n", plan.From, plan.PrimaryTarget, plan.DefaultBranch, plan.From, os.PathSeparator, plan.MarkerPath, config.ProjectConfigPath(plan.From)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(a.Stdout, "Continue? [y/N] "); err != nil {
		return err
	}
	stdin := a.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	executed, err := deps.Execute(plan, stdin)
	if err != nil {
		return err
	}
	if executed {
		_, err = fmt.Fprintln(a.Stdout, "Conversion complete")
	} else {
		_, err = fmt.Fprintln(a.Stdout, "Conversion cancelled")
	}
	return err
}
func (a App) runAdopt(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ut adopt")
	}
	deps := withDefaultAdoptDependencies(a.Adopt)
	startDir, err := deps.WorkingDir()
	if err != nil {
		return err
	}
	plan, err := deps.Plan(startDir)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Adopt existing worktree layout:\n\n  project:  %s\n  worktree: %s\n\nProject marker will be created at:\n\n  %s\n\nOptional project config can live at:\n\n  %s\n\n", plan.ProjectRoot, plan.WorktreeRoot, plan.MarkerPath, config.ProjectConfigPath(plan.ProjectRoot)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(a.Stdout, "Continue? [y/N] "); err != nil {
		return err
	}
	stdin := a.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	executed, err := deps.Execute(plan, stdin)
	if err != nil {
		return err
	}
	if executed {
		_, err = fmt.Fprintln(a.Stdout, "Adoption complete")
	} else {
		_, err = fmt.Fprintln(a.Stdout, "Adoption cancelled")
	}
	return err
}
func parseConvertOptions(args []string) (convert.Options, error) {
	if len(args) == 0 {
		return convert.Options{}, nil
	}
	if len(args) == 2 && args[0] == "--default-branch" && strings.TrimSpace(args[1]) != "" {
		branch := args[1]
		return convert.Options{DefaultBranchOverride: &branch}, nil
	}
	return convert.Options{}, errors.New("usage: ut convert [--default-branch <branch>]")
}
func withDefaultConvertDependencies(deps ConvertDependencies) ConvertDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.Plan == nil {
		deps.Plan = defaultPlanConversion
	}
	if deps.Execute == nil {
		deps.Execute = convert.Execute
	}
	return deps
}
func withDefaultAdoptDependencies(deps AdoptDependencies) AdoptDependencies {
	if deps.WorkingDir == nil {
		deps.WorkingDir = os.Getwd
	}
	if deps.Plan == nil {
		deps.Plan = defaultPlanAdoption
	}
	if deps.Execute == nil {
		deps.Execute = convert.ExecuteAdoption
	}
	return deps
}
func defaultPlanConversion(startDir string, options convert.Options) (convert.Plan, error) {
	return convert.PlanConversion(startDir, options, convert.Dependencies{})
}
func defaultPlanAdoption(startDir string) (convert.AdoptPlan, error) {
	return convert.PlanAdoption(startDir, convert.Dependencies{})
}
