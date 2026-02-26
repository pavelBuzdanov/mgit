package sshkeys

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Candidate struct {
	Path          string `json:"path"`
	Name          string `json:"name"`
	HasPublicPair bool   `json:"hasPublicPair"`
}

func DiscoverDefault() ([]Candidate, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determine home dir: %w", err)
	}
	return Discover(filepath.Join(home, ".ssh"))
}

func Discover(dir string) ([]Candidate, error) {
	resolved, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve ssh dir: %w", err)
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("read ssh dir %s: %w", resolved, err)
	}
	var out []Candidate
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		path := filepath.Join(resolved, name)
		hasPub := fileExists(path + ".pub")
		if !isLikelyPrivateKeyName(name) && !hasPub {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		out = append(out, Candidate{
			Path:          path,
			Name:          name,
			HasPublicPair: hasPub,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func isLikelyPrivateKeyName(name string) bool {
	l := strings.ToLower(strings.TrimSpace(name))
	if l == "" {
		return false
	}
	if strings.HasSuffix(l, ".pub") || strings.HasSuffix(l, ".crt") || strings.HasSuffix(l, ".pem.pub") {
		return false
	}
	switch {
	case l == "config",
		l == "known_hosts",
		l == "known_hosts.old",
		l == "authorized_keys",
		l == "authorized_keys2":
		return false
	}
	if strings.HasSuffix(l, ".sock") {
		return false
	}
	if strings.Contains(l, "known_hosts") {
		return false
	}
	if strings.HasPrefix(l, "id_") {
		return true
	}
	if strings.Contains(l, "key") || strings.HasSuffix(l, ".pem") || strings.HasSuffix(l, ".ppk") {
		return true
	}
	// Fallback heuristic: accept files with a public pair next to them.
	return false
}
