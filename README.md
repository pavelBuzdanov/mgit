# mgit

`mgit` is a smart wrapper around `git` for macOS/Linux that automatically chooses the correct SSH key based on the remote URL.

It lets you keep normal remotes like:

- `git@github.com:CompanyOrg/project.git`
- `git@github.com:myuser/tooling.git`
- `git@gitlab.com:AnotherOrg/repo.git`

and still use the right key for each repo/account, without SSH host aliases such as `github-work` / `github-personal`.

`mgit` resolves the remote, matches a rule, and runs Git with:

- `GIT_SSH_COMMAND='ssh -i <key> -o IdentitiesOnly=yes'`

## Why This Exists

If you use multiple Git identities (work, personal, clients, GitHub + GitLab), the usual pain points are:

- too much `~/.ssh/config` alias setup
- custom hostnames in remotes (`git@github-work:...`)
- remembering which key to use for which org
- manual one-off commands like `GIT_SSH_COMMAND=... git push`

`mgit` solves this by using the **remote URL itself** as the source of truth.

## Key Benefits

- Keep standard remote URLs (`github.com`, `gitlab.com`)
- Keep standard remote names (`origin`, `mirror`)
- No alias-host architecture required
- Repo-local config support (`.mgit/config.json`)
- Interactive rule creation (choose key from `~/.ssh`)
- Transparent behavior (`resolve`, `doctor`, `--dry-run`, `--json`)
- Works as a daily wrapper: `mgit push`, `mgit pull`, `mgit fetch`, `mgit clone`

## How It Works

Example:

```bash
mgit push origin main
```

`mgit` will:

1. Detect the remote (`origin`)
2. Read the remote URL (`git remote get-url origin`)
3. Parse `host`, `owner/namespace`, `repo`
4. Match a rule from config
5. Run `git push origin main` with the correct `GIT_SSH_COMMAND`

## Installation

### Option 1: Go install (recommended)

```bash
go install ./cmd/mgit
```

Then verify:

```bash
mgit version
```

If `mgit` is not found, ensure your Go bin directory is in `PATH` (usually `~/go/bin`).

### Option 2: Build binary manually

```bash
go build -o mgit ./cmd/mgit
./mgit version
```

## Quick Start (Recommended Workflow)

### 1. Go to your repository

```bash
cd /path/to/repo
```

### 2. Add a rule interactively from a real remote URL

```bash
mgit rule add git@github.com:pavelBuzdanov/sunset-echo-4827
```

What happens:

- `mgit` parses the URL automatically (`host=github.com`, `owner=pavelBuzdanov`)
- creates repo-local config if missing: `.mgit/config.json`
- scans `~/.ssh` for likely private keys
- shows an interactive selector
- you choose a key with:
  - arrow keys `↑/↓` + `Enter`
  - or a number (`1`, `2`, `3`, ...)

### 3. Use Git through `mgit`

```bash
mgit push origin main
mgit pull origin develop
mgit fetch mirror
```

## Interactive Key Selection

When you run `mgit rule add <remote-url>` without `--key`, `mgit` opens an interactive menu:

- highlighted current selection
- arrow key navigation (`↑/↓`)
- number selection
- `j/k` navigation (optional)
- `c` for custom path
- `q` to cancel

This is designed for fast setup without typing SSH key paths manually.

## Config (Repo-local by Default)

Default config path behavior:

- If `.mgit/config.json` exists in the current directory or a parent directory, `mgit` uses it
- If you are inside a git repo and no config exists yet, `mgit` targets `<repo-root>/.mgit/config.json`
- If you are outside a git repo, `mgit` targets `./.mgit/config.json`

### Auto `.gitignore` integration

When `mgit` creates a local config and `<repo-root>/.gitignore` already exists, it automatically adds:

```gitignore
.mgit
```

No duplicates are added.

### Override config path (optional)

```bash
mgit --config /path/to/config.json ...
```

## Rule Model

Each rule maps:

- `host` (e.g. `github.com`, `gitlab.com`)
- `owner` / namespace (e.g. `CompanyOrg`, `Group/subgroup`)
- `key` (SSH private key path)

Example config:

```json
{
  "version": 1,
  "rules": [
    {
      "id": "work-github",
      "host": "github.com",
      "owner": "CompanyOrg",
      "key": "~/.ssh/work_key"
    },
    {
      "id": "personal-github",
      "host": "github.com",
      "owner": "pavelBuzdanov",
      "key": "~/.ssh/personal_key"
    },
    {
      "id": "github-fallback",
      "host": "github.com",
      "owner": "*",
      "key": "~/.ssh/default_github_key"
    },
    {
      "id": "default",
      "host": "*",
      "owner": "*",
      "key": "~/.ssh/default_key"
    }
  ]
}
```

### Matching rules

- Exact matches are preferred over wildcards
- More specific rules beat generic rules
- `priority` can be used to override normal scoring
- `owner` supports nested namespaces (GitLab groups/subgroups)

## Supported Remote URL Formats

### SCP-like SSH

- `git@github.com:CompanyOrg/project.git`
- `git@gitlab.com:Group/subgroup/repo.git`

### SSH URL

- `ssh://git@github.com/CompanyOrg/project.git`
- `ssh://git@gitlab.com/Group/subgroup/repo.git`

### HTTPS (transparent passthrough)

- `https://github.com/CompanyOrg/project.git`

For HTTPS remotes:

- `mgit` parses the URL
- skips SSH key selection
- runs Git normally (without `GIT_SSH_COMMAND`)

## Commands

### Git wrapper (preferred daily use)

```bash
mgit push origin main
mgit pull origin develop
mgit fetch origin
mgit clone git@github.com:CompanyOrg/project.git
mgit ls-remote origin
```

### Config commands

```bash
mgit config init
mgit config path
mgit config validate
```

### Rule commands

```bash
mgit rule add git@github.com:CompanyOrg/project.git
mgit rule add --host github.com --owner CompanyOrg --key ~/.ssh/work_key
mgit rule list
mgit rule remove --id work-github
mgit rule remove --host github.com --owner CompanyOrg
```

### Resolution / diagnostics

```bash
mgit resolve --remote origin
mgit resolve --url git@github.com:CompanyOrg/project.git
mgit doctor
mgit ssh-test --remote origin
mgit ssh-test --url git@github.com:CompanyOrg/project.git --dry-run
```

## Real-World Examples

### 1) One repo, two GitHub identities

Remotes:

- `origin = git@github.com:CompanyOrg/project.git`
- `mirror = git@github.com:pavelBuzdanov/project.git`

Rules:

- `github.com + CompanyOrg -> ~/.ssh/work_key`
- `github.com + pavelBuzdanov -> ~/.ssh/personal_key`

Usage:

```bash
mgit push origin main   # uses work key
mgit push mirror main   # uses personal key
```

### 2) GitHub + GitLab

Remotes:

- `origin = git@github.com:CompanyOrg/project.git`
- `gitlab = git@gitlab.com:AnotherOrg/repo.git`

Usage:

```bash
mgit fetch origin
mgit push gitlab main
```

### 3) Fallback key

Rules:

- `github.com + CompanyOrg -> ~/.ssh/work_key`
- `github.com + * -> ~/.ssh/default_github_key`
- `* + * -> ~/.ssh/default_key`

Unknown GitHub owner:

- `git@github.com:UnknownOwner/repo.git` -> uses `~/.ssh/default_github_key`

Unknown host:

- `git@custom.example.com:team/repo.git` -> uses `~/.ssh/default_key`

## Output / Automation-Friendly Modes

Global flags:

- `--json`
- `--verbose`
- `--dry-run`
- `--config PATH`

Examples:

```bash
mgit --json resolve --url git@github.com:CompanyOrg/project.git
mgit --dry-run push origin main
mgit --verbose doctor
```

## Troubleshooting

### `mgit: command not found`

- Install with `go install ./cmd/mgit`
- Ensure `~/go/bin` is in `PATH`
- In `zsh`, run:

```bash
rehash
```

### No matching rule found

Example error:

```text
No SSH key rule matched ... Add a rule with: mgit rule add --host github.com --owner CompanyOrg --key ~/.ssh/<key>
```

Fast fix (interactive):

```bash
mgit rule add git@github.com:CompanyOrg/project.git
```

### I use HTTPS remotes

That is supported. `mgit` will simply skip SSH key selection for HTTPS remotes.

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build ./cmd/mgit
```

## Security Notes

- `mgit` does not print private key contents
- `mgit` only stores paths to SSH keys in config
- SSH command injection is avoided by shell-quoting key paths when building `GIT_SSH_COMMAND`

## License

This project is licensed under the MIT License. See [`LICENSE`](./LICENSE).
