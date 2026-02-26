package resolve

import (
	"fmt"

	"mgit/internal/config"
	"mgit/internal/giturl"
	"mgit/internal/matcher"
	"mgit/internal/runner"
)

type Result struct {
	URL                string             `json:"url"`
	Parsed             *giturl.ParsedRemote `json:"parsed,omitempty"`
	SSHSelectionApplies bool              `json:"sshSelectionApplies"`
	MatchedRule        *config.Rule       `json:"matchedRule,omitempty"`
	KeyPath            string             `json:"keyPath,omitempty"`
	GITSSHCommand      string             `json:"gitSshCommand,omitempty"`
	MatchScore         int                `json:"matchScore,omitempty"`
	Notes              []string           `json:"notes,omitempty"`
}

func FromURL(cfg *config.Config, rawURL string) (*Result, error) {
	parsed, err := giturl.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	res := &Result{
		URL:    rawURL,
		Parsed: parsed,
	}
	if !parsed.IsSSH() {
		res.SSHSelectionApplies = false
		if parsed.IsHTTPS() {
			res.Notes = append(res.Notes, "HTTPS remote detected: SSH key selection is not applied")
		} else {
			res.Notes = append(res.Notes, fmt.Sprintf("transport %q is not SSH: SSH key selection is not applied", parsed.Transport))
		}
		return res, nil
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is required for SSH remote")
	}
	match, err := matcher.Match(cfg.Rules, parsed)
	if err != nil {
		return nil, fmt.Errorf("%w. %s", err, AddRuleHint(parsed))
	}
	keyPath, err := config.ExpandPath(match.Rule.Key)
	if err != nil {
		return nil, fmt.Errorf("expand key path for rule %q: %w", match.Rule.ID, err)
	}
	res.SSHSelectionApplies = true
	res.MatchedRule = &match.Rule
	res.MatchScore = match.Score
	res.KeyPath = keyPath
	res.GITSSHCommand = runner.BuildGITSSHCommand(keyPath)
	return res, nil
}

func AddRuleHint(parsed *giturl.ParsedRemote) string {
	if parsed == nil {
		return "Add a rule with: mgit rule add --host <host> --owner <owner> --key ~/.ssh/<key>"
	}
	return fmt.Sprintf(
		"Add a rule with: mgit rule add --host %s --owner %s --key ~/.ssh/<key>",
		parsed.Host,
		parsed.Owner,
	)
}
