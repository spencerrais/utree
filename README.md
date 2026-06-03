# utree

Git branches as persistent workspaces.

`utree` is a small Go CLI, invoked as `ut`, that turns git worktrees into tmux-backed branch workspaces. 
It gives you a simple way to keep track of active branches and jump between them without losing local context.

`utree` uses one project layout: a `.utree/` directory plus sibling git worktrees.

```text
project/
├── .utree/
│   └── config.toml  # optional project overrides
├── main/
├── feature-a/
└── bugfix-b/
```

## Requirements

- Go
- git
- tmux
- Linux or macOS

## Install Locally

From the repository root:

```bash
mkdir -p ~/.local/bin
go build -o ~/.local/bin/ut ./cmd/ut
```

This installs a binary named `ut` under `~/.local/bin`.

Make sure `~/.local/bin` is on your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

If `ut` is not found after building, add that line to your shell config, such as `~/.zshrc`, then restart your shell or source the config file.

Verify the install:

```bash
ut --help
ut info
```

## Commands

### `ut adopt`

Adopt an existing sibling-worktree layout without moving files.

```bash
cd ~/code/project/main
ut adopt
cd ~/code/project
ut adopt
```

The existing git repository must already be a direct child of the project directory, such as `project/main`. 
You can run `ut adopt` from a direct-child worktree or from the project root. 
If the repository has linked git worktrees, they must also be direct children of the same project directory. 
`ut adopt` refuses to run inside an existing utree project. 
The command previews the marker it will create, asks for confirmation, and creates only `.utree/`. 
It does not create config files; optional project config can live at `<project-root>/.utree/config.toml`.

### `ut convert`

Convert an existing single-worktree git repository into the sibling-worktree layout.

```bash
cd ~/code/project
ut convert --default-branch main
```

The command refuses to run inside an existing utree project. 
It previews the move, asks for confirmation, creates `.utree/`, and moves the existing checkout under the default branch directory, such as `main/`. 
It does not create config files; optional project config can live at `<project-root>/.utree/config.toml`.

Default branch detection uses `--default-branch`, project config, `origin/HEAD`, then local `main` or `master`. 
Repositories without a GitHub remote can still convert when a local default branch exists, 
and the local worktree layout remains compatible with adding a GitHub remote and pushing later.

### `ut new`

Create a new sibling git worktree and open it in tmux.

```bash
ut new feature-a
ut new firehose-role tg-123-firehose-role
ut new bugfix-b --base feature-a
ut new feature-a --detach
```

Without `--base`, new work starts from the detected default branch. 
If your current branch is not the default branch, `ut new` warns and asks before continuing.
If the selected source worktree has a `.env` file, `ut new` asks whether to copy it into the new worktree. 
When run from inside a worktree, the source is that worktree. 
When run from the project root, the source is the default-branch worktree if it is checked out.
Use `-d` or `--detach` to create the worktree and tmux session without switching or attaching to it.

### `ut open`

Open an existing project worktree in its tmux session.

```bash
ut open
ut open .
ut open feature-a
```

If the session exists, `ut open` switches or attaches to it. 
Otherwise, it creates the default two-pane layout with `nvim .` and `git status`.

### `ut list`

List git worktrees for the current project.

```bash
ut list
```

Project worktrees are shown separately from other same-repository git worktrees outside the `.utree` project root.

### `ut remove`

Safely remove a completed worktree workspace.

```bash
ut remove feature-a
```

`ut remove` refuses dirty worktrees, checks whether the local branch appears merged, 
kills the associated tmux session when present, removes the git worktree, 
and deletes the local branch when safe. It never deletes remote branches.

### `ut config info`

Show config paths, precedence, and effective config.

```bash
ut config info
```

This prints the user config path, project config path when inside a utree project, config precedence, and the effective merged TOML. 
It does not create or modify config files.

### `ut info`

Show how `utree` sees the current environment and project.

```bash
ut info
```

This reports git/tmux availability, project root, active config source, project name, default branch, current worktree, current branch, and current tmux session when applicable.

## Configuration

`utree` supports optional user config at:

```text
$XDG_CONFIG_HOME/utree/config.toml
```

If `XDG_CONFIG_HOME` is unset, the user config path is:

```text
~/.config/utree/config.toml
```

`utree` also supports optional project-local config at:

```text
<project-root>/.utree/config.toml
```

No config files are created automatically. 
Use `ut config info` to see where config can live and what effective config `ut` will use.

Config precedence is:

```text
built-in defaults
user config
project config
CLI overrides
```

If neither user config nor project config exists, `ut` uses built-in defaults. 
If user config exists, it is the active config until a project config overrides it. 
If project config exists, it is the active config for that project.

Supported values include project name, default branch, tmux session name template, and default tmux panes.

The default tmux session name is `{project}--{worktree}--{branch}`. 
If the worktree name and branch name are the same, the branch segment is omitted, for example `project--feature-a`. 
Session templates support `{project}`, `{worktree}`, and `{branch}`; rendered names are sanitized for tmux compatibility, so branch names like `chore/test` become `chore-test`.

The default layout is configured as ordered tmux panes. 
The first pane starts the session, and each later pane uses tmux split terminology: `vertical` maps to `tmux split-window -v` and `horizontal` maps to `tmux split-window -h`. 
At most one pane can set `selected = true`; if none is selected, pane 0 is selected.

```toml
[layout.default]

[[layout.default.panes]]
command = "nvim .; exec ${SHELL:-/bin/sh} -l"
selected = true

[[layout.default.panes]]
split = "vertical"
size = "33%"
command = "git status; exec ${SHELL:-/bin/sh} -l"
```

## Unsupported

The following categories are intentionally unsupported or deferred:

- global project discovery
- alternate terminal multiplexers
- multiple tmux servers or custom sockets
- non-interactive `--yes`
- remote branch deletion
