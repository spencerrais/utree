package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spencerrais/utree/internal/config"
)

type CommandRunner func(name string, args ...string) ([]byte, error)

type Adapter struct {
	Run CommandRunner
}

type Command struct {
	Name string
	Args []string
}

func RenderSessionName(projectRoot string, configuredProjectName string, worktreeName string, branchName string, template string) (string, error) {
	if err := config.ValidateSessionTemplate(template); err != nil {
		return "", err
	}

	projectName := configuredProjectName
	if projectName == "" || projectName == "auto" {
		projectName = filepath.Base(filepath.Clean(projectRoot))
	}

	if template == config.DefaultSessionNameTemplate && branchName == worktreeName {
		template = "{project}--{worktree}"
	}
	if template == config.DefaultSessionNameTemplate && strings.TrimSpace(branchName) == "" {
		template = "{project}--{worktree}"
	}

	name := strings.ReplaceAll(template, "{project}", projectName)
	name = strings.ReplaceAll(name, "{worktree}", worktreeName)
	name = strings.ReplaceAll(name, "{branch}", branchName)
	return sanitizeNameValue(name), nil
}

func IsInsideTmux(tmuxEnv string) bool {
	return tmuxEnv != ""
}

func (a Adapter) HasSession(session string) (bool, error) {
	_, err := a.run("tmux", "has-session", "-t", session)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (a Adapter) SwitchClient(session string) error {
	if _, err := a.run("tmux", "switch-client", "-t", session); err != nil {
		return fmt.Errorf("tmux switch-client %s: %w", session, err)
	}
	return nil
}

func (a Adapter) SwitchToAdjacentSession(current string) (bool, error) {
	sessions, err := a.ListSessions()
	if err != nil {
		return false, err
	}
	if len(sessions) <= 1 {
		return false, nil
	}

	target := ""
	for i, session := range sessions {
		if session != current {
			continue
		}
		if i > 0 {
			target = sessions[i-1]
		} else if i+1 < len(sessions) {
			target = sessions[i+1]
		}
		break
	}
	if target == "" {
		for _, session := range sessions {
			if session != current {
				target = session
				break
			}
		}
	}
	if target == "" {
		return false, nil
	}
	if err := a.SwitchClient(target); err != nil {
		return false, err
	}
	return true, nil
}

func (a Adapter) AttachSession(session string) error {
	if _, err := a.runInteractive("tmux", "attach-session", "-t", session); err != nil {
		return fmt.Errorf("tmux attach-session -t %s: %w; if you are not in an interactive terminal, run: tmux attach -t %s", session, err, session)
	}
	return nil
}

func (a Adapter) CurrentSession() (string, error) {
	output, err := a.run("tmux", "display-message", "-p", "#S")
	if err != nil {
		return "", fmt.Errorf("tmux display-message -p #S: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (a Adapter) ListSessions() ([]string, error) {
	output, err := a.run("tmux", "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	sessions := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

func (a Adapter) KillSession(session string) error {
	if _, err := a.run("tmux", "kill-session", "-t", session); err != nil {
		return fmt.Errorf("tmux kill-session %s: %w", session, err)
	}
	return nil
}

func (a Adapter) OpenSession(session string, insideTmux bool) error {
	if insideTmux {
		return a.SwitchClient(session)
	}
	return a.AttachSession(session)
}

func (a Adapter) CreateDefaultLayout(session string, worktreePath string, layout config.DefaultLayoutConfig) error {
	for _, command := range DefaultLayoutCommands(session, worktreePath, layout) {
		if _, err := a.run(command.Name, command.Args...); err != nil {
			return fmt.Errorf("%s %s: %w", command.Name, strings.Join(command.Args, " "), err)
		}
	}
	return nil
}

func (a Adapter) CreateFallbackSession(session string, dir string) error {
	if _, err := a.run("tmux", "new-session", "-d", "-s", session, "-c", dir); err != nil {
		return fmt.Errorf("tmux new-session -d -s %s -c %s: %w", session, dir, err)
	}
	return nil
}

func DefaultLayoutCommands(session string, worktreePath string, layout config.DefaultLayoutConfig) []Command {
	if len(layout.Panes) == 0 {
		return nil
	}
	commands := []Command{{Name: "tmux", Args: []string{"new-session", "-d", "-s", session, "-c", worktreePath, layout.Panes[0].Command}}}
	selectedPane := 0
	if layout.Panes[0].Selected {
		selectedPane = 0
	}
	for i, pane := range layout.Panes[1:] {
		paneIndex := i + 1
		args := []string{"split-window", splitFlag(pane.Split)}
		if strings.TrimSpace(pane.Size) != "" {
			args = append(args, "-l", pane.Size)
		}
		args = append(args, "-t", session, "-c", worktreePath, pane.Command)
		commands = append(commands, Command{Name: "tmux", Args: args})
		if pane.Selected {
			selectedPane = paneIndex
		}
	}
	commands = append(commands, Command{Name: "tmux", Args: []string{"select-pane", "-t", fmt.Sprintf("%s:0.%d", session, selectedPane)}})
	return commands
}

func splitFlag(split string) string {
	if split == config.SplitHorizontal {
		return "-h"
	}
	return "-v"
}

func sanitizeNameValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, strings.TrimSpace(value))
}

func (a Adapter) run(name string, args ...string) ([]byte, error) {
	if a.Run != nil {
		output, err := a.Run(name, args...)
		return output, commandError(err, output)
	}

	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	return output, commandError(err, output)
}

func (a Adapter) runInteractive(name string, args ...string) ([]byte, error) {
	if a.Run != nil {
		output, err := a.Run(name, args...)
		return output, commandError(err, output)
	}
	if !stdinIsTerminal() {
		return a.run(name, args...)
	}

	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return nil, cmd.Run()
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func commandError(err error, output []byte) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}
