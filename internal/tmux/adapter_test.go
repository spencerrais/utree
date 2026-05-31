package tmux

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/spencerrais/utree/internal/config"
)

func TestRenderSessionNameUsesDefaultTemplateAndProjectRootBase(t *testing.T) {
	name, err := RenderSessionName("/home/me/code/infrastructure-live", "auto", "feature-a", "chore/test", "{project}--{worktree}--{branch}")
	if err != nil {
		t.Fatalf("RenderSessionName returned error: %v", err)
	}

	if name != "infrastructure-live--feature-a--chore-test" {
		t.Fatalf("expected infrastructure-live--feature-a--chore-test, got %q", name)
	}
}

func TestRenderSessionNameOmitsDuplicateBranchForDefaultTemplate(t *testing.T) {
	name, err := RenderSessionName("/home/me/code/infrastructure-live", "auto", "feature-a", "feature-a", "{project}--{worktree}--{branch}")
	if err != nil {
		t.Fatalf("RenderSessionName returned error: %v", err)
	}

	if name != "infrastructure-live--feature-a" {
		t.Fatalf("expected infrastructure-live--feature-a, got %q", name)
	}
}

func TestRenderSessionNameUsesConfiguredProjectNameAndTemplate(t *testing.T) {
	name, err := RenderSessionName("/home/me/code/infrastructure-live", "infra", "feature-a", "chore/test", "{project}__{worktree}__{branch}")
	if err != nil {
		t.Fatalf("RenderSessionName returned error: %v", err)
	}

	if name != "infra__feature-a__chore-test" {
		t.Fatalf("expected infra__feature-a__chore-test, got %q", name)
	}
}

func TestRenderSessionNameRejectsUnsupportedVariables(t *testing.T) {
	_, err := RenderSessionName("/repo/project", "auto", "feature-a", "feature-a", "{project}:{unknown}")
	if err == nil {
		t.Fatal("expected unsupported template variable error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected error to mention unknown, got %v", err)
	}
}

func TestRenderSessionNameSanitizesOnlyInvalidTmuxCharacters(t *testing.T) {
	name, err := RenderSessionName("/repo/project:one", "auto", "feature-a_bugfix.1", "feature/a", "{project}:{worktree}:{branch}")
	if err != nil {
		t.Fatalf("RenderSessionName returned error: %v", err)
	}

	if name != "project-one-feature-a_bugfix.1-feature-a" {
		t.Fatalf("expected minimal sanitization, got %q", name)
	}
}

func TestAdapterConstructsSessionLifecycleCommands(t *testing.T) {
	runner := &recordingRunner{output: []byte("project:feature-a\n")}
	adapter := Adapter{Run: runner.run}

	exists, err := adapter.HasSession("project:feature-a")
	if err != nil {
		t.Fatalf("HasSession returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected session to exist")
	}
	if err := adapter.SwitchClient("project:feature-a"); err != nil {
		t.Fatalf("SwitchClient returned error: %v", err)
	}
	if err := adapter.AttachSession("project:feature-a"); err != nil {
		t.Fatalf("AttachSession returned error: %v", err)
	}
	if err := adapter.KillSession("project:feature-a"); err != nil {
		t.Fatalf("KillSession returned error: %v", err)
	}
	session, err := adapter.CurrentSession()
	if err != nil {
		t.Fatalf("CurrentSession returned error: %v", err)
	}
	if session != "project:feature-a" {
		t.Fatalf("expected current session, got %q", session)
	}
	if err := adapter.CreateFallbackSession("utree", "/home/me"); err != nil {
		t.Fatalf("CreateFallbackSession returned error: %v", err)
	}

	assertCommands(t, runner.commands, []recordedCommand{
		{name: "tmux", args: []string{"has-session", "-t", "project:feature-a"}},
		{name: "tmux", args: []string{"switch-client", "-t", "project:feature-a"}},
		{name: "tmux", args: []string{"attach-session", "-t", "project:feature-a"}},
		{name: "tmux", args: []string{"kill-session", "-t", "project:feature-a"}},
		{name: "tmux", args: []string{"display-message", "-p", "#S"}},
		{name: "tmux", args: []string{"new-session", "-d", "-s", "utree", "-c", "/home/me"}},
	})
}

func TestAdapterSwitchesToAdjacentSession(t *testing.T) {
	runner := &recordingRunner{output: []byte("dev\nproject--feature-a\nother\n")}
	adapter := Adapter{Run: runner.run}

	switched, err := adapter.SwitchToAdjacentSession("project--feature-a")
	if err != nil {
		t.Fatalf("SwitchToAdjacentSession returned error: %v", err)
	}
	if !switched {
		t.Fatal("expected adjacent session switch")
	}

	assertCommands(t, runner.commands, []recordedCommand{
		{name: "tmux", args: []string{"list-sessions", "-F", "#{session_name}"}},
		{name: "tmux", args: []string{"switch-client", "-t", "dev"}},
	})
}

func TestAdapterSwitchAdjacentReportsFalseWhenNoOtherSession(t *testing.T) {
	runner := &recordingRunner{output: []byte("project--feature-a\n")}
	adapter := Adapter{Run: runner.run}

	switched, err := adapter.SwitchToAdjacentSession("project--feature-a")
	if err != nil {
		t.Fatalf("SwitchToAdjacentSession returned error: %v", err)
	}
	if switched {
		t.Fatal("expected no adjacent session switch")
	}

	assertCommands(t, runner.commands, []recordedCommand{{name: "tmux", args: []string{"list-sessions", "-F", "#{session_name}"}}})
}

func TestAdapterConstructsDefaultLayoutCommands(t *testing.T) {
	runner := &recordingRunner{}
	adapter := Adapter{Run: runner.run}
	layout := config.Default().Layout.Default

	if err := adapter.CreateDefaultLayout("project--feature-a", "/repo/feature-a", layout); err != nil {
		t.Fatalf("CreateDefaultLayout returned error: %v", err)
	}

	assertCommands(t, runner.commands, []recordedCommand{
		{name: "tmux", args: []string{"new-session", "-d", "-s", "project--feature-a", "-c", "/repo/feature-a", "nvim .; exec ${SHELL:-/bin/sh} -l"}},
		{name: "tmux", args: []string{"split-window", "-v", "-l", "33%", "-t", "project--feature-a", "-c", "/repo/feature-a", "git status; exec ${SHELL:-/bin/sh} -l"}},
		{name: "tmux", args: []string{"select-pane", "-t", "project--feature-a:0.0"}},
	})
}

func TestDefaultLayoutCommandsCanBePlannedWithoutExecutingTmux(t *testing.T) {
	layout := config.DefaultLayoutConfig{Panes: []config.PaneConfig{
		{Command: "nvim ."},
		{Command: "git status", Split: config.SplitVertical, Size: "33%"},
		{Command: "go test ./...", Split: config.SplitHorizontal, Size: "50%", Selected: true},
	}}
	commands := DefaultLayoutCommands("project--feature-a", "/repo/feature-a", layout)

	want := []Command{
		{Name: "tmux", Args: []string{"new-session", "-d", "-s", "project--feature-a", "-c", "/repo/feature-a", "nvim ."}},
		{Name: "tmux", Args: []string{"split-window", "-v", "-l", "33%", "-t", "project--feature-a", "-c", "/repo/feature-a", "git status"}},
		{Name: "tmux", Args: []string{"split-window", "-h", "-l", "50%", "-t", "project--feature-a", "-c", "/repo/feature-a", "go test ./..."}},
		{Name: "tmux", Args: []string{"select-pane", "-t", "project--feature-a:0.2"}},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("expected commands %#v, got %#v", want, commands)
	}
}

func TestAdapterOpenChoosesSwitchInsideTmuxAndAttachOutsideTmux(t *testing.T) {
	insideRunner := &recordingRunner{}
	insideAdapter := Adapter{Run: insideRunner.run}
	if err := insideAdapter.OpenSession("project:feature-a", true); err != nil {
		t.Fatalf("OpenSession inside tmux returned error: %v", err)
	}
	assertCommands(t, insideRunner.commands, []recordedCommand{{name: "tmux", args: []string{"switch-client", "-t", "project:feature-a"}}})

	outsideRunner := &recordingRunner{}
	outsideAdapter := Adapter{Run: outsideRunner.run}
	if err := outsideAdapter.OpenSession("project:feature-a", false); err != nil {
		t.Fatalf("OpenSession outside tmux returned error: %v", err)
	}
	assertCommands(t, outsideRunner.commands, []recordedCommand{{name: "tmux", args: []string{"attach-session", "-t", "project:feature-a"}}})
}

func TestAdapterAttachSessionIncludesTmuxOutputAndSuggestion(t *testing.T) {
	runner := &recordingRunner{output: []byte("open terminal failed: not a terminal\n"), err: errors.New("exit status 1")}
	adapter := Adapter{Run: runner.run}

	err := adapter.AttachSession("project--main")
	if err == nil {
		t.Fatal("expected attach error")
	}
	assertContains(t, err.Error(), "open terminal failed")
	assertContains(t, err.Error(), "tmux attach -t project--main")
}

func TestIsInsideTmuxUsesTMUXEnvironmentValue(t *testing.T) {
	if IsInsideTmux("") {
		t.Fatal("expected empty TMUX value to mean outside tmux")
	}
	if !IsInsideTmux("/tmp/tmux-1000/default,123,0") {
		t.Fatal("expected non-empty TMUX value to mean inside tmux")
	}
}

func TestHasSessionReportsFalseForMissingSession(t *testing.T) {
	runner := &recordingRunner{err: errors.New("exit status 1")}
	adapter := Adapter{Run: runner.run}

	exists, err := adapter.HasSession("project:missing")
	if err != nil {
		t.Fatalf("HasSession returned error: %v", err)
	}
	if exists {
		t.Fatal("expected missing session to report false")
	}
}

type recordedCommand struct {
	name string
	args []string
}

type recordingRunner struct {
	commands []recordedCommand
	output   []byte
	err      error
}

func (r *recordingRunner) run(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	if r.err != nil {
		return r.output, r.err
	}
	return r.output, nil
}

func assertContains(t *testing.T, value string, want string) {
	t.Helper()

	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}

func assertCommands(t *testing.T, got []recordedCommand, want []recordedCommand) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected commands %#v, got %#v", want, got)
	}
}
