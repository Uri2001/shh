package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
)

var sshOptionsWithArg = map[string]bool{
	"-b": true,
	"-c": true,
	"-D": true,
	"-E": true,
	"-F": true,
	"-I": true,
	"-i": true,
	"-J": true,
	"-L": true,
	"-l": true,
	"-m": true,
	"-o": true,
	"-p": true,
	"-Q": true,
	"-R": true,
	"-S": true,
	"-W": true,
	"-w": true,
}

func (s *Store) ImportFromHistory(ctx context.Context) (int, error) {
	paths := possibleHistoryFiles()
	if len(paths) == 0 {
		return 0, nil
	}

	var errs []error
	seen := map[string]struct{}{}
	imported := 0

	for _, p := range paths {
		fileImported, err := s.importHistoryFile(ctx, p, seen)
		if err != nil {
			errs = append(errs, fmt.Errorf("history %s: %w", p, err))
			continue
		}
		imported += fileImported
	}

	return imported, errors.Join(errs...)
}

func (s *Store) importHistoryFile(ctx context.Context, path string, seen map[string]struct{}) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		// quietly skip missing files
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	added := 0
	for scanner.Scan() {
		line := scanner.Text()
		if host, ok := parseHistoryLine(line); ok {
			if _, dup := seen[host]; dup {
				continue
			}
			if err := s.ImportHost(ctx, host, "imported from history"); err != nil {
				return added, err
			}
			seen[host] = struct{}{}
			added++
		}
	}
	if err := scanner.Err(); err != nil {
		return added, err
	}
	return added, nil
}

func possibleHistoryFiles() []string {
	paths := []string{}
	seen := map[string]struct{}{}
	maybeAdd := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	if histFile := os.Getenv("HISTFILE"); histFile != "" {
		maybeAdd(histFile)
	}

	home, err := os.UserHomeDir()
	if err == nil {
		maybeAdd(filepath.Join(home, ".bash_history"))
		maybeAdd(filepath.Join(home, ".zsh_history"))
		maybeAdd(filepath.Join(home, ".local", "share", "fish", "fish_history"))
	}

	return paths
}

func parseHistoryLine(line string) (string, bool) {
	line = ansiRegexp.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}

	if strings.HasPrefix(line, ": ") {
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = line[idx+1:]
		}
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", false
	}

	sshIndex := -1
	for i, field := range fields {
		if strings.EqualFold(field, "ssh") {
			sshIndex = i
			break
		}
		if strings.HasSuffix(strings.ToLower(field), "/ssh") {
			sshIndex = i
			break
		}
	}
	if sshIndex < 0 {
		return "", false
	}

	var expectValue bool
	for i := sshIndex + 1; i < len(fields); i++ {
		token := fields[i]
		if expectValue {
			expectValue = false
			continue
		}

		if token == "--" {
			continue
		}

		lower := strings.ToLower(token)
		if strings.HasPrefix(lower, "-") {
			if eq := strings.IndexRune(token, '='); eq >= 0 {
				opt := token[:eq]
				if sshOptionsWithArg[opt] {
					continue
				}
				continue
			}
			if len(token) > 2 && !strings.ContainsRune(token, '=') {
				// e.g. -p2222
				continue
			}
			if sshOptionsWithArg[token] {
				expectValue = true
				continue
			}
			continue
		}

		host := token
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		host = strings.Trim(host, "[]")
		if host == "" {
			return "", false
		}
		norm, err := normalizeHost(host)
		if err != nil {
			return "", false
		}
		return norm, true
	}

	return "", false
}
