package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	safeHost = regexp.MustCompile(`^(?:[A-Za-z0-9._-]+|\[[0-9A-Fa-f:]+\])$`)
)

func normalizeHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", fmt.Errorf("host cannot be empty")
	}
	if strings.Contains(host, " ") {
		return "", fmt.Errorf("host cannot contain spaces")
	}
	if !safeHost.MatchString(host) {
		return "", fmt.Errorf("invalid host format")
	}
	return host, nil
}
