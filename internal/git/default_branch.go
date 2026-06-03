package git

import (
	"errors"
	"fmt"
	"strings"
)

var ErrDefaultBranchNotDetected = errors.New("default branch not detected")

type DetectionMode int

const (
	StrictMode DetectionMode = iota
	FallbackMode
)

type DefaultBranchOptions struct {
	CLIOverride         *string
	ConfigDefaultBranch string
}

type BranchSource interface {
	OriginHead() (string, error)
	LocalBranchExists(name string) (bool, error)
}

type CommandRunner func(dir string, name string, args ...string) ([]byte, error)

type CommandBranchSource struct {
	Dir string
	Run CommandRunner
}

func ResolveDefaultBranch(mode DetectionMode, options DefaultBranchOptions, source BranchSource) (string, error) {
	if options.CLIOverride != nil && *options.CLIOverride != "" {
		return *options.CLIOverride, nil
	}

	configuredBranch := strings.TrimSpace(options.ConfigDefaultBranch)
	if configuredBranch != "" && configuredBranch != "auto" {
		return configuredBranch, nil
	}

	if originHead, err := source.OriginHead(); err == nil {
		branch, err := ParseOriginHead(originHead)
		if err == nil {
			return branch, nil
		}
	}

	if mode == StrictMode {
		return "", fmt.Errorf("%w: origin/HEAD unavailable", ErrDefaultBranchNotDetected)
	}

	for _, branch := range []string{"main", "master"} {
		exists, err := source.LocalBranchExists(branch)
		if err != nil {
			return "", fmt.Errorf("check local branch %q: %w", branch, err)
		}
		if exists {
			return branch, nil
		}
	}

	return "", fmt.Errorf("%w: tried origin/HEAD, main, master", ErrDefaultBranchNotDetected)
}

func ParseOriginHead(output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" || strings.Contains(output, "\n") || strings.Contains(output, "\r") {
		return "", fmt.Errorf("parse origin head %q: expected origin/<branch>", output)
	}

	branch, ok := strings.CutPrefix(output, "origin/")
	if !ok || branch == "" {
		return "", fmt.Errorf("parse origin head %q: expected origin/<branch>", output)
	}

	return branch, nil
}

func (s CommandBranchSource) OriginHead() (string, error) {
	output, err := s.run("git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (s CommandBranchSource) LocalBranchExists(name string) (bool, error) {
	_, err := s.run("git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err == nil {
		return true, nil
	}
	return false, nil
}

func (s CommandBranchSource) run(name string, args ...string) ([]byte, error) {
	if s.Run != nil {
		return s.Run(s.Dir, name, args...)
	}

	cmd := Command(s.Dir, args...)
	return cmd.Output()
}

func GitDefaultBranchCommands(dir string, localBranch string) []string {
	return []string{
		"git symbolic-ref --quiet --short refs/remotes/origin/HEAD",
		"git show-ref --verify --quiet refs/heads/" + localBranch,
	}
}
