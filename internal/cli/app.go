package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"

	"mgit/internal/config"
	"mgit/internal/doctor"
	"mgit/internal/giturl"
	"mgit/internal/resolve"
	"mgit/internal/runner"
	"mgit/internal/sshkeys"
	"mgit/internal/ui"
)

const version = "0.1.0"

type App struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

type globalOptions struct {
	ConfigPath string
	JSON       bool
	Verbose    bool
	DryRun     bool
}

func New(stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{stdin: stdin, stdout: stdout, stderr: stderr}
}

func (a *App) Run(ctx context.Context, args []string) int {
	opts, rest, err := parseGlobalOptions(args)
	if err != nil {
		a.printErr(err)
		a.printUsage()
		return 2
	}
	if len(rest) == 0 {
		a.printUsage()
		return 0
	}

	switch rest[0] {
	case "help", "--help", "-h":
		a.printUsage()
		return 0
	case "version", "--version":
		fmt.Fprintln(a.stdout, version)
		return 0
	case "config":
		return a.handleConfig(ctx, opts, rest[1:])
	case "rule":
		return a.handleRule(ctx, opts, rest[1:])
	case "resolve":
		return a.handleResolve(ctx, opts, rest[1:])
	case "doctor":
		return a.handleDoctor(ctx, opts, rest[1:])
	case "ssh-test":
		return a.handleSSHTest(ctx, opts, rest[1:])
	case "exec":
		return a.handleExec(ctx, opts, rest[1:])
	default:
		return a.handleExec(ctx, opts, rest)
	}
}

func parseGlobalOptions(args []string) (globalOptions, []string, error) {
	var opts globalOptions
	rest := make([]string, 0, len(args))
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			return opts, rest, nil
		}
		if !strings.HasPrefix(a, "-") {
			rest = append(rest, args[i:]...)
			return opts, rest, nil
		}
		switch {
		case a == "--json":
			opts.JSON = true
		case a == "--verbose":
			opts.Verbose = true
		case a == "--dry-run":
			opts.DryRun = true
		case a == "--config":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--config requires a value")
			}
			i++
			opts.ConfigPath = args[i]
		case strings.HasPrefix(a, "--config="):
			opts.ConfigPath = strings.TrimPrefix(a, "--config=")
		default:
			rest = append(rest, args[i:]...)
			return opts, rest, nil
		}
		i++
	}
	return opts, rest, nil
}

func (a *App) newShell(opts globalOptions) *runner.Shell {
	return runner.NewShell(a.stdout, a.stderr, opts.Verbose)
}

func (a *App) handleConfig(ctx context.Context, opts globalOptions, args []string) int {
	if len(args) == 0 {
		a.printConfigUsage()
		return 2
	}
	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("mgit config init", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		force := fs.Bool("force", false, "")
		if err := fs.Parse(args[1:]); err != nil {
			a.printErr(err)
			return 2
		}
		path, created, err := config.Init(opts.ConfigPath, *force)
		if err != nil {
			a.printErr(err)
			return 1
		}
		if changed, err := config.EnsureGitignoreExcludesMgit(path); err == nil && changed {
			fmt.Fprintln(a.stdout, "Updated .gitignore: added .mgit")
		} else if err != nil && opts.Verbose {
			fmt.Fprintf(a.stderr, "warn: failed to update .gitignore: %v\n", err)
		}
		if created {
			fmt.Fprintf(a.stdout, "Created config: %s\n", path)
		} else {
			fmt.Fprintf(a.stdout, "Config already exists: %s\n", path)
		}
		return 0
	case "path":
		path, err := config.ResolvePath(opts.ConfigPath)
		if err != nil {
			a.printErr(err)
			return 1
		}
		fmt.Fprintln(a.stdout, path)
		return 0
	case "validate":
		cfg, path, err := a.loadConfig(opts)
		if err != nil {
			a.printErr(err)
			return 1
		}
		issues := cfg.Validate()
		if opts.JSON {
			_ = ui.PrintJSON(a.stdout, map[string]any{
				"configPath": path,
				"valid":      !config.HasErrors(issues),
				"issues":     issues,
			})
		} else {
			fmt.Fprintf(a.stdout, "Config: %s\n", path)
			if len(issues) == 0 {
				fmt.Fprintln(a.stdout, "Validation: OK")
			} else {
				for _, issue := range issues {
					field := ""
					if issue.Field != "" {
						field = " (" + issue.Field + ")"
					}
					fmt.Fprintf(a.stdout, "[%s]%s %s\n", strings.ToUpper(issue.Level), field, issue.Message)
				}
				if config.HasErrors(issues) {
					fmt.Fprintln(a.stdout, "Validation: FAILED")
				} else {
					fmt.Fprintln(a.stdout, "Validation: OK (with warnings)")
				}
			}
		}
		if config.HasErrors(issues) {
			return 1
		}
		return 0
	default:
		a.printConfigUsage()
		return 2
	}
}

func (a *App) handleRule(ctx context.Context, opts globalOptions, args []string) int {
	_ = ctx
	if len(args) == 0 {
		a.printRuleUsage()
		return 2
	}
	switch args[0] {
	case "list":
		cfg, _, err := a.loadConfig(opts)
		if err != nil {
			a.printErr(err)
			return 1
		}
		if opts.JSON {
			_ = ui.PrintJSON(a.stdout, map[string]any{"rules": cfg.Rules})
			return 0
		}
		if len(cfg.Rules) == 0 {
			fmt.Fprintln(a.stdout, "No rules configured")
			return 0
		}
		for i, r := range cfg.Rules {
			fmt.Fprintf(a.stdout, "%d. id=%s host=%s owner=%s key=%s", i+1, r.ID, r.Host, r.Owner, r.Key)
			if r.Priority != 0 {
				fmt.Fprintf(a.stdout, " priority=%d", r.Priority)
			}
			fmt.Fprintln(a.stdout)
		}
		return 0
	case "add":
		fs := flag.NewFlagSet("mgit rule add", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var host, owner, namespace, key, id, remoteURL string
		var priority int
		noPrompt := fs.Bool("no-prompt", false, "")
		force := fs.Bool("force", false, "")
		fs.StringVar(&host, "host", "", "")
		fs.StringVar(&owner, "owner", "", "")
		fs.StringVar(&namespace, "namespace", "", "")
		fs.StringVar(&key, "key", "", "")
		fs.StringVar(&remoteURL, "url", "", "")
		fs.StringVar(&id, "id", "", "")
		fs.IntVar(&priority, "priority", 0, "")
		if err := fs.Parse(args[1:]); err != nil {
			a.printErr(err)
			return 2
		}
		pos := fs.Args()
		if remoteURL == "" && len(pos) > 0 {
			remoteURL = pos[0]
		}
		if remoteURL != "" {
			parsed, err := giturl.Parse(remoteURL)
			if err != nil {
				a.printErr(fmt.Errorf("failed to parse URL %q: %w", remoteURL, err))
				return 2
			}
			if strings.TrimSpace(host) == "" {
				host = parsed.Host
			}
			if strings.TrimSpace(owner) == "" && strings.TrimSpace(namespace) == "" {
				owner = parsed.Owner
			}
			if !opts.JSON {
				fmt.Fprintf(a.stdout, "Detected from URL: host=%s owner=%s repo=%s transport=%s\n", parsed.Host, parsed.Owner, parsed.Repo, parsed.Transport)
			}
		}
		if owner == "" {
			owner = namespace
		}
		if strings.TrimSpace(host) == "" {
			host = "*"
		}
		if strings.TrimSpace(owner) == "" {
			owner = "*"
		}
		if strings.TrimSpace(key) == "" {
			if *noPrompt {
				a.printErr(errors.New("--key is required when --no-prompt is used"))
				return 2
			}
			selected, err := a.selectSSHKeyInteractively(host, owner)
			if err != nil {
				a.printErr(err)
				return 1
			}
			key = selected
		}
		cfg, path, err := a.loadOrCreateConfig(opts)
		if err != nil {
			a.printErr(err)
			return 1
		}
		if err := cfg.AddRule(config.Rule{
			ID:       id,
			Host:     host,
			Owner:    owner,
			Key:      key,
			Priority: priority,
		}, *force); err != nil {
			a.printErr(err)
			return 1
		}
		if err := config.Save(path, cfg); err != nil {
			a.printErr(err)
			return 1
		}
		fmt.Fprintf(a.stdout, "Rule added: host=%s owner=%s key=%s\n", host, owner, key)
		fmt.Fprintf(a.stdout, "Saved to %s\n", path)
		return 0
	case "remove":
		fs := flag.NewFlagSet("mgit rule remove", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		var sel config.RemoveSelector
		var namespace string
		fs.StringVar(&sel.ID, "id", "", "")
		fs.StringVar(&sel.Host, "host", "", "")
		fs.StringVar(&sel.Owner, "owner", "", "")
		fs.StringVar(&namespace, "namespace", "", "")
		fs.StringVar(&sel.Key, "key", "", "")
		fs.IntVar(&sel.Index, "index", 0, "")
		if err := fs.Parse(args[1:]); err != nil {
			a.printErr(err)
			return 2
		}
		if sel.Owner == "" {
			sel.Owner = namespace
		}
		cfg, path, err := a.loadConfig(opts)
		if err != nil {
			a.printErr(err)
			return 1
		}
		removed, ok := cfg.RemoveRule(sel)
		if !ok {
			a.printErr(errors.New("rule not found"))
			return 1
		}
		if err := config.Save(path, cfg); err != nil {
			a.printErr(err)
			return 1
		}
		fmt.Fprintf(a.stdout, "Removed rule id=%s host=%s owner=%s\n", removed.ID, removed.Host, removed.Owner)
		return 0
	default:
		a.printRuleUsage()
		return 2
	}
}

func (a *App) handleResolve(ctx context.Context, opts globalOptions, args []string) int {
	fs := flag.NewFlagSet("mgit resolve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var remoteName, rawURL string
	fs.StringVar(&remoteName, "remote", "", "")
	fs.StringVar(&rawURL, "url", "", "")
	if err := fs.Parse(args); err != nil {
		a.printErr(err)
		return 2
	}
	if remoteName == "" && rawURL == "" {
		a.printErr(errors.New("specify --remote <name> or --url <remote-url>"))
		return 2
	}
	if remoteName != "" && rawURL != "" {
		a.printErr(errors.New("use only one of --remote or --url"))
		return 2
	}

	var source string
	if remoteName != "" {
		git := runner.NewGitOps(a.newShell(opts))
		u, err := git.RemoteURL(ctx, remoteName)
		if err != nil {
			a.printErr(fmt.Errorf("failed to get URL for remote %q: %w", remoteName, err))
			return 1
		}
		rawURL = u
		source = "remote:" + remoteName
	} else {
		source = "url"
	}

	cfg, _, err := a.loadConfig(opts)
	if err != nil {
		// Resolve still works for HTTPS without config, but for simplicity parse first and branch.
		if rawURL == "" {
			a.printErr(err)
			return 1
		}
		res, parseErr := resolve.FromURL(nil, rawURL)
		if parseErr == nil && !res.SSHSelectionApplies {
			a.printResolveResult(source, remoteName, res, opts)
			return 0
		}
		a.printErr(err)
		return 1
	}
	res, err := resolve.FromURL(cfg, rawURL)
	if err != nil {
		a.printErr(err)
		return 1
	}
	a.printResolveResult(source, remoteName, res, opts)
	return 0
}

func (a *App) handleExec(ctx context.Context, opts globalOptions, gitArgs []string) int {
	if len(gitArgs) == 0 {
		a.printErr(errors.New("missing git arguments; use e.g. `mgit push origin main`"))
		return 2
	}

	git := runner.NewGitOps(a.newShell(opts))
	target, err := runner.InferGitTarget(gitArgs)
	if err != nil {
		a.printErr(err)
		return 2
	}
	notes := []string{}
	if target.Notes != "" {
		notes = append(notes, target.Notes)
	}

	var rawURL string
	var remoteName string
	switch target.Kind {
	case runner.TargetURL:
		rawURL = target.URL
	case runner.TargetRemote:
		remoteName = target.RemoteName
	case runner.TargetNone:
		if target.Command == "push" || target.Command == "fetch" || target.Command == "pull" {
			guessed, guessErr := git.GuessDefaultRemote(ctx)
			if guessErr == nil {
				remoteName = guessed
				target.Kind = runner.TargetRemote
				target.RemoteName = guessed
				notes = append(notes, "remote inferred automatically: "+guessed)
			}
		}
	}
	if remoteName != "" {
		u, err := git.RemoteURL(ctx, remoteName)
		if err != nil {
			a.printErr(fmt.Errorf("failed to get URL for remote %q: %w", remoteName, err))
			return 1
		}
		rawURL = u
	}

	extraEnv := map[string]string{}
	var res *resolve.Result
	if rawURL != "" && !target.SkipSSHSelection {
		// Load config lazily; HTTPS remotes can proceed without it.
		cfg, _, cfgErr := a.loadConfig(opts)
		if cfgErr != nil {
			if strings.Contains(rawURL, "://") && strings.HasPrefix(strings.ToLower(rawURL), "https://") {
				notes = append(notes, "config not loaded, but remote uses HTTPS so SSH rule selection is skipped")
			} else {
				a.printErr(cfgErr)
				return 1
			}
		}
		res, err = resolve.FromURL(cfg, rawURL)
		if err != nil {
			a.printErr(err)
			return 1
		}
		if res.SSHSelectionApplies {
			extraEnv["GIT_SSH_COMMAND"] = res.GITSSHCommand
		}
		notes = append(notes, res.Notes...)
	} else if rawURL != "" && target.SkipSSHSelection {
		// No SSH override needed for this command (e.g. remote set-url).
	}

	if opts.DryRun {
		payload := map[string]any{
			"gitArgs":   gitArgs,
			"target":    target,
			"remoteURL": rawURL,
			"env":       extraEnv,
			"notes":     notes,
		}
		if res != nil {
			payload["resolution"] = res
		}
		if opts.JSON {
			_ = ui.PrintJSON(a.stdout, payload)
		} else {
			fmt.Fprintf(a.stdout, "Dry run: git %s\n", strings.Join(gitArgs, " "))
			if rawURL != "" {
				fmt.Fprintf(a.stdout, "Resolved URL: %s\n", rawURL)
			}
			if target.Kind == runner.TargetRemote {
				fmt.Fprintf(a.stdout, "Remote: %s\n", target.RemoteName)
			}
			if len(extraEnv) > 0 {
				for k, v := range extraEnv {
					fmt.Fprintf(a.stdout, "%s=%s\n", k, v)
				}
			} else {
				fmt.Fprintln(a.stdout, "No SSH env override will be applied")
			}
			for _, n := range notes {
				fmt.Fprintf(a.stdout, "Note: %s\n", n)
			}
		}
		return 0
	}

	if err := git.RunGit(ctx, gitArgs, extraEnv); err != nil {
		a.printErr(err)
		return 1
	}
	return 0
}

func (a *App) handleDoctor(ctx context.Context, opts globalOptions, args []string) int {
	fs := flag.NewFlagSet("mgit doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		a.printErr(err)
		return 2
	}
	var cfg *config.Config
	cfgPath, _ := config.ResolvePath(opts.ConfigPath)
	cfgLoaded, _, cfgErr := a.tryLoadConfig(opts)
	if cfgErr == nil {
		cfg = cfgLoaded
	}

	git := runner.NewGitOps(a.newShell(opts))
	rep := doctor.Build(ctx, git, cfg, cfgPath)
	if cfgErr != nil {
		rep.Checks = append([]doctor.Check{{Name: "config-load", Status: "error", Message: cfgErr.Error()}}, rep.Checks...)
	}

	if opts.JSON {
		_ = ui.PrintJSON(a.stdout, rep)
	} else {
		fmt.Fprintf(a.stdout, "Config path: %s\n", rep.ConfigPath)
		for _, c := range rep.Checks {
			fmt.Fprintf(a.stdout, "[%s] %s: %s\n", strings.ToUpper(c.Status), c.Name, c.Message)
		}
		for _, issue := range rep.ConfigIssues {
			field := issue.Field
			if field != "" {
				field = " (" + field + ")"
			}
			fmt.Fprintf(a.stdout, "[%s] config%s: %s\n", strings.ToUpper(issue.Level), field, issue.Message)
		}
		if len(rep.Remotes) > 0 {
			fmt.Fprintln(a.stdout, "Remotes:")
			for _, r := range rep.Remotes {
				fmt.Fprintf(a.stdout, "  - %s => %s\n", r.Name, r.URL)
				if r.Error != "" {
					fmt.Fprintf(a.stdout, "    error: %s\n", r.Error)
					continue
				}
				if r.Result != nil && r.Result.Parsed != nil {
					fmt.Fprintf(a.stdout, "    parsed: host=%s owner=%s repo=%s transport=%s\n", r.Result.Parsed.Host, r.Result.Parsed.Owner, r.Result.Parsed.Repo, r.Result.Parsed.Transport)
					if r.Result.MatchedRule != nil {
						fmt.Fprintf(a.stdout, "    rule: id=%s key=%s\n", r.Result.MatchedRule.ID, r.Result.KeyPath)
					} else {
						fmt.Fprintln(a.stdout, "    rule: n/a (non-SSH remote)")
					}
				}
			}
		}
	}

	hasError := cfgErr != nil
	for _, c := range rep.Checks {
		if c.Status == "error" {
			hasError = true
		}
	}
	if len(rep.Unmatched) > 0 {
		hasError = true
	}
	if hasError {
		return 1
	}
	return 0
}

func (a *App) handleSSHTest(ctx context.Context, opts globalOptions, args []string) int {
	fs := flag.NewFlagSet("mgit ssh-test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var remoteName, rawURL string
	localDryRun := fs.Bool("dry-run", false, "")
	fs.StringVar(&remoteName, "remote", "", "")
	fs.StringVar(&rawURL, "url", "", "")
	if err := fs.Parse(args); err != nil {
		a.printErr(err)
		return 2
	}
	if remoteName == "" && rawURL == "" {
		a.printErr(errors.New("specify --remote <name> or --url <remote-url>"))
		return 2
	}
	if remoteName != "" && rawURL != "" {
		a.printErr(errors.New("use only one of --remote or --url"))
		return 2
	}

	git := runner.NewGitOps(a.newShell(opts))
	if remoteName != "" {
		u, err := git.RemoteURL(ctx, remoteName)
		if err != nil {
			a.printErr(fmt.Errorf("failed to get URL for remote %q: %w", remoteName, err))
			return 1
		}
		rawURL = u
	}

	cfg, _, err := a.loadConfig(opts)
	if err != nil {
		a.printErr(err)
		return 1
	}
	res, err := resolve.FromURL(cfg, rawURL)
	if err != nil {
		a.printErr(err)
		return 1
	}
	if !res.SSHSelectionApplies || res.Parsed == nil {
		a.printErr(errors.New("SSH test is only applicable for SSH remotes"))
		return 1
	}
	sshArgs := []string{"-i", res.KeyPath, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes", "-T", res.Parsed.TargetUserHost()}
	if opts.DryRun || *localDryRun {
		if opts.JSON {
			_ = ui.PrintJSON(a.stdout, map[string]any{
				"url":        rawURL,
				"sshCommand": append([]string{"ssh"}, sshArgs...),
				"keyPath":    res.KeyPath,
			})
		} else {
			fmt.Fprintf(a.stdout, "Dry run: ssh %s\n", strings.Join(sshArgs, " "))
		}
		return 0
	}
	if err := a.newShell(opts).Run(ctx, "ssh", sshArgs, nil); err != nil {
		a.printErr(err)
		return 1
	}
	return 0
}

func (a *App) loadConfig(opts globalOptions) (*config.Config, string, error) {
	return a.tryLoadConfig(opts)
}

func (a *App) tryLoadConfig(opts globalOptions) (*config.Config, string, error) {
	path, err := config.ResolvePath(opts.ConfigPath)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, path, fmt.Errorf("%w\nHint: initialize config with: mgit config init", err)
	}
	return cfg, path, nil
}

func (a *App) loadOrCreateConfig(opts globalOptions) (*config.Config, string, error) {
	path, err := config.ResolvePath(opts.ConfigPath)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, path, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, path, err
	}
	cfg = &config.Config{Version: config.CurrentVersion, Rules: []config.Rule{}}
	if err := config.Save(path, cfg); err != nil {
		return nil, path, fmt.Errorf("create config at %s: %w", path, err)
	}
	if changed, err := config.EnsureGitignoreExcludesMgit(path); err == nil && changed {
		fmt.Fprintln(a.stdout, "Updated .gitignore: added .mgit")
	} else if err != nil && opts.Verbose {
		fmt.Fprintf(a.stderr, "warn: failed to update .gitignore: %v\n", err)
	}
	fmt.Fprintf(a.stdout, "Created config: %s\n", path)
	return cfg, path, nil
}

func (a *App) selectSSHKeyInteractively(host, owner string) (string, error) {
	if !a.stdinIsTTY() {
		return "", errors.New("no --key provided and interactive prompt is unavailable (stdin is not a TTY). Use --key <path> or run in a terminal")
	}
	keys, err := sshkeys.DiscoverDefault()
	if err != nil {
		return "", err
	}
	fmt.Fprintln(a.stdout, "Select SSH key for the new rule:")
	fmt.Fprintf(a.stdout, "  host=%s\n", host)
	fmt.Fprintf(a.stdout, "  owner=%s\n", owner)
	if len(keys) == 0 {
		fmt.Fprintln(a.stdout, "No SSH keys found in ~/.ssh.")
		custom, err := a.promptLine("Enter key path (or leave empty to cancel): ")
		if err != nil {
			return "", err
		}
		custom = strings.TrimSpace(custom)
		if custom == "" {
			return "", errors.New("cancelled")
		}
		return custom, nil
	}
	items := make([]string, 0, len(keys))
	for _, k := range keys {
		label := k.Path
		if k.HasPublicPair {
			label += " (has .pub)"
		}
		items = append(items, label)
	}
	res, err := a.pickOptionInteractive("Select SSH key:", items)
	if err != nil {
		return "", err
	}
	switch res.Kind {
	case "index":
		return keys[res.Index].Path, nil
	case "custom":
		custom, err := a.promptLine("Enter key path: ")
		if err != nil {
			return "", err
		}
		custom = strings.TrimSpace(custom)
		if custom == "" {
			return "", errors.New("cancelled")
		}
		return custom, nil
	default:
		return "", errors.New("cancelled")
	}
}

func (a *App) promptLine(prompt string) (string, error) {
	fmt.Fprint(a.stdout, prompt)
	r := bufio.NewReader(a.stdin)
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (a *App) stdinIsTTY() bool {
	f, ok := a.stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (a *App) printResolveResult(source, remoteName string, res *resolve.Result, opts globalOptions) {
	payload := map[string]any{
		"source": source,
		"url":    res.URL,
		"result": res,
	}
	if remoteName != "" {
		payload["remote"] = remoteName
	}
	if opts.JSON {
		_ = ui.PrintJSON(a.stdout, payload)
		return
	}
	fmt.Fprintf(a.stdout, "Source: %s\n", source)
	fmt.Fprintf(a.stdout, "URL: %s\n", res.URL)
	if res.Parsed != nil {
		fmt.Fprintf(a.stdout, "Parsed: host=%s owner=%s repo=%s transport=%s\n", res.Parsed.Host, res.Parsed.Owner, res.Parsed.Repo, res.Parsed.Transport)
	}
	if res.MatchedRule != nil {
		fmt.Fprintf(a.stdout, "Matched rule: id=%s host=%s owner=%s\n", res.MatchedRule.ID, res.MatchedRule.Host, res.MatchedRule.Owner)
		fmt.Fprintf(a.stdout, "Key path: %s\n", res.KeyPath)
		fmt.Fprintf(a.stdout, "GIT_SSH_COMMAND: %s\n", res.GITSSHCommand)
	} else {
		fmt.Fprintln(a.stdout, "Matched rule: n/a")
	}
	for _, n := range res.Notes {
		fmt.Fprintf(a.stdout, "Note: %s\n", n)
	}
}

func (a *App) printUsage() {
	fmt.Fprintln(a.stdout, "mgit - smart git wrapper with SSH key auto-selection by remote URL")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Usage:")
	fmt.Fprintln(a.stdout, "  mgit [--config PATH] [--json] [--verbose] [--dry-run] <command> [args]")
	fmt.Fprintln(a.stdout, "  mgit [--config PATH] [--verbose] [--dry-run] <git-subcommand> [git args]")
	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Commands:")
	fmt.Fprintln(a.stdout, "  config init|path|validate")
	fmt.Fprintln(a.stdout, "  rule add|list|remove")
	fmt.Fprintln(a.stdout, "  resolve --remote <name> | --url <url>")
	fmt.Fprintln(a.stdout, "  doctor")
	fmt.Fprintln(a.stdout, "  ssh-test --remote <name> | --url <url>")
	fmt.Fprintln(a.stdout, "  exec <git args>")
	fmt.Fprintln(a.stdout, "  version")
}

func (a *App) printConfigUsage() {
	fmt.Fprintln(a.stdout, "Usage: mgit config init [--force] | path | validate")
}

func (a *App) printRuleUsage() {
	fmt.Fprintln(a.stdout, "Usage:")
	fmt.Fprintln(a.stdout, "  mgit rule list")
	fmt.Fprintln(a.stdout, "  mgit rule add <remote-url>              # interactive key selection from ~/.ssh")
	fmt.Fprintln(a.stdout, "  mgit rule add --host <host|*> --owner <owner|namespace|*> --key <path> [--priority N] [--id ID] [--force]")
	fmt.Fprintln(a.stdout, "  mgit rule remove [--index N | --id ID | --host H --owner O [--key K]]")
}

func (a *App) printErr(err error) {
	fmt.Fprintf(a.stderr, "Error: %v\n", err)
}

// Helper used in tests to keep deterministic ordering in textual outputs that include maps.
func stableMapLines(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+m[k])
	}
	return out
}

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
