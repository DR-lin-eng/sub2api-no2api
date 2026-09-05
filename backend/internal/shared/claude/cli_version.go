package claude

import (
	"log/slog"
	"os"
	"strings"

	"golang.org/x/mod/semver"
)

const CLIVersionEnv = "SUB2API_CLAUDE_CLI_VERSION"

var resolvedCLIVersion = resolveCLIVersion(os.Getenv(CLIVersionEnv))

func CLIVersion() string { return resolvedCLIVersion }
func IsSupportedCLIVersion(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	canonical := "v" + version
	if !semver.IsValid(canonical) || semver.Canonical(canonical) != canonical || semver.Prerelease(canonical) != "" || semver.Build(canonical) != "" {
		return false
	}
	return semver.Compare(canonical, "v"+CLICurrentVersion) >= 0
}
func resolveCLIVersion(raw string) string {
	version := strings.TrimSpace(raw)
	if version == "" {
		return CLICurrentVersion
	}
	if !IsSupportedCLIVersion(version) {
		slog.Warn("ignoring invalid Claude CLI version override; falling back to built-in pin", "env", CLIVersionEnv, "value", version, "builtin", CLICurrentVersion)
		return CLICurrentVersion
	}
	return version
}
