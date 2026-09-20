package main

import (
	"regexp"
	"strings"
)

// whitelistedExactCommands is the set of command strings classified as
// whitelisted when the trimmed input matches exactly.
// This covers both bare commands (no arguments) and specific full commands.
var whitelistedExactCommands = map[string]bool{
	// Bare commands — no arguments allowed.
	"pwd":    true,
	"date":   true,
	"whoami": true,
	"env":    true,
	"id":     true,
	"ip -4 -o addr show eth1 | awk '{print $4}'": true,
	"uptime":  true,
	"df":      true,
	"free":    true,
	"ps":      true,
	"history": true,
	// Full commands with specific arguments.
	"home-manager switch --rollback":                                     true,
	"home-manager generations":                                           true,
	`nix develop --command sh -c "figlet 'Nix' | cowsay -n | lolcat -f"`: true,
}

// whitelistedPrefixCommands is the set of commands classified as whitelisted
// when invoked bare or with arguments, as long as no shell metacharacters are
// present.
var whitelistedPrefixCommands = map[string]bool{
	"cd":       true,
	"echo":     true,
	"cat":      true,
	"head":     true,
	"tail":     true,
	"grep":     true,
	"ls":       true,
	"uname":    true,
	"wc":       true,
	"file":     true,
	"du":       true,
	"stat":     true,
	"realpath": true,
	"printf":   true,
	"which":    true,
	"tree":     true,
}

// shellMetaChars matches shell operators that could be used to chain commands.
var shellMetaChars = regexp.MustCompile(`[;|&<>\t\n\r` + "`" + `]|\$\(`)

// classifyCommand returns the classification of a command for audit logging.
// It returns "whitelisted" for:
//   - exact matches in whitelistedExactCommands,
//   - prefix matches in whitelistedPrefixCommands (bare or with args, no shell metacharacters),
//   - "nix run nixpkgs#..." invocations without shell metacharacters.
//
// Otherwise it returns "unclassified".
func classifyCommand(cmd string) string {
	trimmed := strings.TrimSpace(cmd)
	if whitelistedExactCommands[trimmed] {
		return "whitelisted"
	}
	if !shellMetaChars.MatchString(trimmed) {
		for prefix := range whitelistedPrefixCommands {
			if trimmed == prefix || strings.HasPrefix(trimmed, prefix+" ") {
				return "whitelisted"
			}
		}
		if strings.HasPrefix(trimmed, "nix run nixpkgs#") {
			return "whitelisted"
		}
	}
	return "unclassified"
}
