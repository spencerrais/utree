package git

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveStrictUsesCLIOverrideFirst(t *testing.T) {
	cliOverride := "trunk"
	source := &fakeBranchSource{originHead: "origin/develop"}

	branch, err := ResolveDefaultBranch(StrictMode, DefaultBranchOptions{
		CLIOverride:         &cliOverride,
		ConfigDefaultBranch: "main",
	}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "trunk" {
		t.Fatalf("expected trunk, got %q", branch)
	}
	if len(source.calls) != 0 {
		t.Fatalf("expected no git calls, got %v", source.calls)
	}
}

func TestResolveFallbackUsesCLIOverrideFirst(t *testing.T) {
	cliOverride := "trunk"
	source := &fakeBranchSource{originHead: "origin/develop"}

	branch, err := ResolveDefaultBranch(FallbackMode, DefaultBranchOptions{
		CLIOverride:         &cliOverride,
		ConfigDefaultBranch: "main",
	}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "trunk" {
		t.Fatalf("expected trunk, got %q", branch)
	}
	if len(source.calls) != 0 {
		t.Fatalf("expected no git calls, got %v", source.calls)
	}
}

func TestResolveStrictUsesExplicitProjectConfigBeforeOriginHead(t *testing.T) {
	source := &fakeBranchSource{originHead: "origin/main"}

	branch, err := ResolveDefaultBranch(StrictMode, DefaultBranchOptions{ConfigDefaultBranch: "develop"}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "develop" {
		t.Fatalf("expected develop, got %q", branch)
	}
	if len(source.calls) != 0 {
		t.Fatalf("expected no git calls, got %v", source.calls)
	}
}

func TestResolveFallbackUsesExplicitProjectConfigBeforeOriginHead(t *testing.T) {
	source := &fakeBranchSource{originHead: "origin/main"}

	branch, err := ResolveDefaultBranch(FallbackMode, DefaultBranchOptions{ConfigDefaultBranch: "develop"}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "develop" {
		t.Fatalf("expected develop, got %q", branch)
	}
	if len(source.calls) != 0 {
		t.Fatalf("expected no git calls, got %v", source.calls)
	}
}

func TestResolveStrictTreatsAutoConfigAsAutomaticDetection(t *testing.T) {
	source := &fakeBranchSource{originHead: "origin/main"}

	branch, err := ResolveDefaultBranch(StrictMode, DefaultBranchOptions{ConfigDefaultBranch: "auto"}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
	assertCalls(t, source.calls, []string{"origin-head"})
}

func TestResolveFallbackTreatsAutoConfigAsAutomaticDetection(t *testing.T) {
	source := &fakeBranchSource{originHead: "origin/main"}

	branch, err := ResolveDefaultBranch(FallbackMode, DefaultBranchOptions{ConfigDefaultBranch: "auto"}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
	assertCalls(t, source.calls, []string{"origin-head"})
}

func TestResolveStrictParsesOriginHead(t *testing.T) {
	source := &fakeBranchSource{originHead: "origin/main\n"}

	branch, err := ResolveDefaultBranch(StrictMode, DefaultBranchOptions{}, source)
	if err != nil {
		t.Fatalf("ResolveDefaultBranch returned error: %v", err)
	}

	if branch != "main" {
		t.Fatalf("expected main, got %q", branch)
	}
}

func TestResolveStrictFailsWithoutOriginHeadAndDoesNotGuess(t *testing.T) {
	source := &fakeBranchSource{
		originHeadErr: errors.New("origin head missing"),
		localBranches: map[string]bool{
			"main":   true,
			"master": true,
		},
	}

	_, err := ResolveDefaultBranch(StrictMode, DefaultBranchOptions{ConfigDefaultBranch: "auto"}, source)
	if !errors.Is(err, ErrDefaultBranchNotDetected) {
		t.Fatalf("expected ErrDefaultBranchNotDetected, got %v", err)
	}
	assertCalls(t, source.calls, []string{"origin-head"})
}

func TestResolveFallbackUsesLocalMainThenMaster(t *testing.T) {
	t.Run("main", func(t *testing.T) {
		source := &fakeBranchSource{
			originHeadErr: errors.New("origin head missing"),
			localBranches: map[string]bool{"main": true, "master": true},
		}

		branch, err := ResolveDefaultBranch(FallbackMode, DefaultBranchOptions{ConfigDefaultBranch: "auto"}, source)
		if err != nil {
			t.Fatalf("ResolveDefaultBranch returned error: %v", err)
		}
		if branch != "main" {
			t.Fatalf("expected main, got %q", branch)
		}
		assertCalls(t, source.calls, []string{"origin-head", "local:main"})
	})

	t.Run("master", func(t *testing.T) {
		source := &fakeBranchSource{
			originHeadErr: errors.New("origin head missing"),
			localBranches: map[string]bool{"master": true},
		}

		branch, err := ResolveDefaultBranch(FallbackMode, DefaultBranchOptions{ConfigDefaultBranch: "auto"}, source)
		if err != nil {
			t.Fatalf("ResolveDefaultBranch returned error: %v", err)
		}
		if branch != "master" {
			t.Fatalf("expected master, got %q", branch)
		}
		assertCalls(t, source.calls, []string{"origin-head", "local:main", "local:master"})
	})
}

func TestResolveFallbackFailsWhenNoSourceAvailable(t *testing.T) {
	source := &fakeBranchSource{originHeadErr: errors.New("origin head missing")}

	_, err := ResolveDefaultBranch(FallbackMode, DefaultBranchOptions{ConfigDefaultBranch: "auto"}, source)
	if !errors.Is(err, ErrDefaultBranchNotDetected) {
		t.Fatalf("expected ErrDefaultBranchNotDetected, got %v", err)
	}
	assertCalls(t, source.calls, []string{"origin-head", "local:main", "local:master"})
}

func TestParseOriginHeadRejectsUnexpectedOutput(t *testing.T) {
	testCases := []string{
		"",
		"main",
		"upstream/main",
		"origin/",
		"origin/main\norigin/master",
	}

	for _, testCase := range testCases {
		t.Run(testCase, func(t *testing.T) {
			_, err := ParseOriginHead(testCase)
			if err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestGitCommandRunnerUsesOnlyNonNetworkCommands(t *testing.T) {
	commands := GitDefaultBranchCommands("/repo", "main")
	joined := strings.Join(commands, "\n")

	assertContains(t, joined, "git symbolic-ref --quiet --short refs/remotes/origin/HEAD")
	assertContains(t, joined, "git show-ref --verify --quiet refs/heads/main")

	for _, forbidden := range []string{"remote show", "fetch", "pull"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("expected no %q command in %q", forbidden, joined)
		}
	}
}

type fakeBranchSource struct {
	originHead    string
	originHeadErr error
	localBranches map[string]bool
	calls         []string
}

func (f *fakeBranchSource) OriginHead() (string, error) {
	f.calls = append(f.calls, "origin-head")
	if f.originHeadErr != nil {
		return "", f.originHeadErr
	}
	return f.originHead, nil
}

func (f *fakeBranchSource) LocalBranchExists(name string) (bool, error) {
	f.calls = append(f.calls, "local:"+name)
	return f.localBranches[name], nil
}

func assertCalls(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected calls %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected calls %v, got %v", want, got)
		}
	}
}

func assertContains(t *testing.T, value string, want string) {
	t.Helper()

	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}
