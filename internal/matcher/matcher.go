package matcher

import (
	"fmt"
	"path/filepath"
	"strings"

	"mgit/internal/config"
	"mgit/internal/giturl"
)

type MatchResult struct {
	Rule  config.Rule `json:"rule"`
	Score int         `json:"score"`
	Index int         `json:"index"`
}

func Match(rules []config.Rule, remote *giturl.ParsedRemote) (*MatchResult, error) {
	if remote == nil {
		return nil, fmt.Errorf("nil parsed remote")
	}
	if remote.Host == "" {
		return nil, fmt.Errorf("parsed remote host is empty")
	}
	var best *MatchResult
	for i, r := range rules {
		ok, score := matchRule(r, remote)
		if !ok {
			continue
		}
		candidate := &MatchResult{Rule: r, Score: score, Index: i}
		if best == nil || candidate.Score > best.Score {
			best = candidate
		}
	}
	if best == nil {
		return nil, fmt.Errorf(
			"no SSH key rule matched (host=%s, owner=%s)",
			remote.Host,
			remote.Owner,
		)
	}
	return best, nil
}

func matchRule(r config.Rule, remote *giturl.ParsedRemote) (bool, int) {
	hostPattern := normalizePattern(strings.ToLower(r.Host))
	ownerPattern := normalizePattern(strings.ToLower(r.Owner))
	hostValue := strings.ToLower(remote.Host)
	ownerValue := strings.ToLower(remote.Owner)

	hostOK, err := filepath.Match(hostPattern, hostValue)
	if err != nil || !hostOK {
		return false, 0
	}
	ownerOK, err := filepath.Match(ownerPattern, ownerValue)
	if err != nil || !ownerOK {
		return false, 0
	}
	score := r.Priority * 1000
	score += specificityScore(hostPattern, hostValue)
	score += specificityScore(ownerPattern, ownerValue)
	score += literalChars(hostPattern) + literalChars(ownerPattern)
	return true, score
}

func specificityScore(pattern, value string) int {
	if pattern == "*" {
		return 0
	}
	if !hasWildcard(pattern) && strings.EqualFold(pattern, value) {
		return 400
	}
	if !hasWildcard(pattern) {
		return 300
	}
	return 100
}

func literalChars(pattern string) int {
	n := 0
	for _, r := range pattern {
		switch r {
		case '*', '?', '[', ']':
			continue
		default:
			n++
		}
	}
	return n
}

func hasWildcard(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func normalizePattern(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "*"
	}
	return s
}
