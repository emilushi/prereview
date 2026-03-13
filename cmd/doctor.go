package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/emilushi/prereview/internal/copilot"
	"github.com/emilushi/prereview/internal/git"
	"github.com/emilushi/prereview/internal/ui"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system dependencies and configuration",
	Long: `Check if all required dependencies are installed and properly configured.

This command verifies:
  - Copilot CLI is installed
  - Copilot CLI is authenticated
  - Git is available
  - Current directory is a git repository (optional)`,
	Run: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type checkResult struct {
	name    string
	ok      bool
	message string
	help    string
}

func runDoctor(cmd *cobra.Command, args []string) {
	fmt.Println()
	ui.Title("🩺 PreReview Doctor")
	fmt.Println()
	ui.Muted("Checking dependencies and configuration...")
	fmt.Println()

	results := []checkResult{}
	allOk := true

	// Check 1: Git
	results = append(results, checkGit())

	// Check 2: Git repository (informational)
	results = append(results, checkGitRepo())

	// Check 3: Copilot CLI installed
	results = append(results, checkCopilotInstalled())

	// Check 4: Copilot CLI authenticated
	results = append(results, checkCopilotAuth())

	// Check 5: Copilot SDK/CLI protocol compatibility
	results = append(results, checkCopilotProtocol())

	// Print results
	for _, r := range results {
		if r.ok {
			fmt.Printf("  %s %s\n", ui.SuccessIcon(), r.name)
			if r.message != "" {
				ui.Muted(fmt.Sprintf("     %s", r.message))
			}
		} else {
			fmt.Printf("  %s %s\n", ui.ErrorIcon(), r.name)
			if r.message != "" {
				ui.Muted(fmt.Sprintf("     %s", r.message))
			}
			allOk = false
		}
		fmt.Println()
	}

	// Print help for failed checks
	hasHelp := false
	for _, r := range results {
		if !r.ok && r.help != "" {
			if !hasHelp {
				ui.Divider()
				fmt.Println()
				ui.Title("📋 How to fix")
				fmt.Println()
				hasHelp = true
			}
			fmt.Println(r.help)
			fmt.Println()
		}
	}

	// Summary
	ui.Divider()
	fmt.Println()
	if allOk {
		ui.Success("✓ All checks passed! PreReview is ready to use.")
	} else {
		ui.Warning("⚠ Some checks failed. Please fix the issues above.")
	}
	fmt.Println()
}

func checkGit() checkResult {
	path, err := exec.LookPath("git")
	if err != nil {
		return checkResult{
			name:    "Git",
			ok:      false,
			message: "git command not found",
			help:    getGitInstallHelp(),
		}
	}

	// Get git version
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return checkResult{
			name:    "Git",
			ok:      false,
			message: "failed to get git version",
		}
	}

	version := strings.TrimSpace(string(out))
	return checkResult{
		name:    "Git",
		ok:      true,
		message: fmt.Sprintf("%s (%s)", version, path),
	}
}

func checkGitRepo() checkResult {
	if git.IsGitRepo() {
		return checkResult{
			name:    "Git repository",
			ok:      true,
			message: "current directory is a git repository",
		}
	}

	return checkResult{
		name:    "Git repository",
		ok:      true, // Not a failure, just informational
		message: "not in a git repository (optional for doctor)",
	}
}

func checkCopilotInstalled() checkResult {
	// Try different possible command names
	commands := []string{"copilot", "github-copilot-cli"}
	
	for _, cmd := range commands {
		path, err := exec.LookPath(cmd)
		if err == nil {
			// Try to get version
			out, err := exec.Command(cmd, "--version").Output()
			version := "unknown version"
			if err == nil {
				version = strings.TrimSpace(string(out))
			}
			return checkResult{
				name:    "Copilot CLI",
				ok:      true,
				message: fmt.Sprintf("%s (%s)", version, path),
			}
		}
	}

	return checkResult{
		name:    "Copilot CLI",
		ok:      false,
		message: "copilot command not found in PATH",
		help:    getCopilotInstallHelp(),
	}
}

func checkCopilotAuth() checkResult {
	// First check if copilot is installed
	copilotCmd := findCopilotCommand()
	if copilotCmd == "" {
		return checkResult{
			name:    "Copilot authentication",
			ok:      false,
			message: "cannot check - Copilot CLI not installed",
		}
	}

	// Run copilot auth status
	cmd := exec.Command(copilotCmd, "auth", "status")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		// Check if it's an auth issue vs command issue
		if strings.Contains(strings.ToLower(output), "not logged in") ||
			strings.Contains(strings.ToLower(output), "not authenticated") ||
			strings.Contains(strings.ToLower(output), "no token") {
			return checkResult{
				name:    "Copilot authentication",
				ok:      false,
				message: "not authenticated",
				help:    getCopilotAuthHelp(copilotCmd),
			}
		}
		
		// Unknown error
		return checkResult{
			name:    "Copilot authentication",
			ok:      false,
			message: fmt.Sprintf("failed to check: %s", strings.TrimSpace(output)),
			help:    getCopilotAuthHelp(copilotCmd),
		}
	}

	// Check for success indicators in output
	if strings.Contains(strings.ToLower(output), "logged in") ||
		strings.Contains(strings.ToLower(output), "authenticated") ||
		strings.Contains(output, "✓") {
		// Extract user if possible
		return checkResult{
			name:    "Copilot authentication",
			ok:      true,
			message: "authenticated with GitHub",
		}
	}

	// Command succeeded but unclear status
	return checkResult{
		name:    "Copilot authentication",
		ok:      true,
		message: "authentication status checked",
	}
}

func checkCopilotProtocol() checkResult {
	// First check if copilot is installed
	copilotCmd := findCopilotCommand()
	if copilotCmd == "" {
		return checkResult{
			name:    "SDK/CLI compatibility",
			ok:      false,
			message: "cannot check - Copilot CLI not installed",
		}
	}

	// Try to create a client - this will fail if there's a protocol mismatch
	client, err := copilot.NewClient()
	if err != nil {
		errStr := err.Error()
		
		// Check for protocol version mismatch
		if strings.Contains(errStr, "protocol version mismatch") ||
			strings.Contains(errStr, "SDK expects version") {
			return checkResult{
				name:    "SDK/CLI compatibility",
				ok:      false,
				message: "protocol version mismatch between SDK and Copilot CLI",
				help:    getCopilotProtocolHelp(),
			}
		}
		
		// Other startup errors (might be auth related, etc.)
		return checkResult{
			name:    "SDK/CLI compatibility",
			ok:      false,
			message: fmt.Sprintf("failed to connect: %s", truncateError(errStr, 60)),
			help:    getCopilotProtocolHelp(),
		}
	}
	
	// Success - clean up the client
	client.Close()
	
	return checkResult{
		name:    "SDK/CLI compatibility",
		ok:      true,
		message: "SDK and Copilot CLI are compatible",
	}
}

func truncateError(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func findCopilotCommand() string {
	commands := []string{"copilot", "github-copilot-cli"}
	for _, cmd := range commands {
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd
		}
	}
	return ""
}

func getCopilotInstallHelp() string {
	os := runtime.GOOS
	
	var sb strings.Builder
	sb.WriteString("  Install Copilot CLI:\n\n")

	switch os {
	case "darwin":
		sb.WriteString("    macOS (Homebrew):\n")
		sb.WriteString("    $ brew install copilot-cli\n")
	case "linux":
		sb.WriteString("    Linux (Homebrew):\n")
		sb.WriteString("    $ brew install copilot-cli\n\n")
		sb.WriteString("    Or download from:\n")
		sb.WriteString("    https://github.com/github/copilot-cli/releases\n")
	case "windows":
		sb.WriteString("    Windows (winget):\n")
		sb.WriteString("    > winget install GitHub.CopilotCLI\n\n")
		sb.WriteString("    Windows (Scoop):\n")
		sb.WriteString("    > scoop install copilot-cli\n")
	default:
		sb.WriteString("    Download from:\n")
		sb.WriteString("    https://github.com/github/copilot-cli/releases\n")
	}

	sb.WriteString("\n  After installation, authenticate:\n")
	sb.WriteString("    $ copilot auth login\n")

	return sb.String()
}

func getCopilotAuthHelp(copilotCmd string) string {
	var sb strings.Builder
	sb.WriteString("  Authenticate with GitHub Copilot:\n\n")
	sb.WriteString(fmt.Sprintf("    $ %s auth login\n\n", copilotCmd))
	sb.WriteString("  This will open a browser to authenticate with your GitHub account.\n")
	sb.WriteString("  You need an active GitHub Copilot subscription.\n\n")
	sb.WriteString("  After login, verify with:\n")
	sb.WriteString(fmt.Sprintf("    $ %s auth status\n", copilotCmd))
	return sb.String()
}

func getCopilotProtocolHelp() string {
	os := runtime.GOOS

	var sb strings.Builder
	sb.WriteString("  SDK/CLI protocol version mismatch detected.\n\n")
	sb.WriteString("  This happens when the Copilot CLI version is incompatible with\n")
	sb.WriteString("  the SDK version used by prereview.\n\n")
	sb.WriteString("  To fix, update your Copilot CLI:\n\n")

	switch os {
	case "darwin":
		sb.WriteString("    macOS (Homebrew):\n")
		sb.WriteString("    $ brew upgrade copilot-cli\n")
	case "linux":
		sb.WriteString("    Linux (Homebrew):\n")
		sb.WriteString("    $ brew upgrade copilot-cli\n\n")
		sb.WriteString("    Or download the latest from:\n")
		sb.WriteString("    https://github.com/github/copilot-cli/releases\n")
	case "windows":
		sb.WriteString("    Windows (winget):\n")
		sb.WriteString("    > winget upgrade GitHub.CopilotCLI\n\n")
		sb.WriteString("    Windows (Scoop):\n")
		sb.WriteString("    > scoop update copilot-cli\n")
	default:
		sb.WriteString("    Download the latest from:\n")
		sb.WriteString("    https://github.com/github/copilot-cli/releases\n")
	}

	sb.WriteString("\n  If the issue persists after updating, you may need to update\n")
	sb.WriteString("  prereview itself to get a compatible SDK version:\n")
	sb.WriteString("    $ go install github.com/emilushi/prereview@latest\n")

	return sb.String()
}

func getGitInstallHelp() string {
	os := runtime.GOOS
	
	var sb strings.Builder
	sb.WriteString("  Install Git:\n\n")

	switch os {
	case "darwin":
		sb.WriteString("    macOS (Homebrew):\n")
		sb.WriteString("    $ brew install git\n\n")
		sb.WriteString("    Or install Xcode Command Line Tools:\n")
		sb.WriteString("    $ xcode-select --install\n")
	case "linux":
		sb.WriteString("    Debian/Ubuntu:\n")
		sb.WriteString("    $ sudo apt install git\n\n")
		sb.WriteString("    Fedora:\n")
		sb.WriteString("    $ sudo dnf install git\n\n")
		sb.WriteString("    Arch:\n")
		sb.WriteString("    $ sudo pacman -S git\n")
	case "windows":
		sb.WriteString("    Windows (winget):\n")
		sb.WriteString("    > winget install Git.Git\n\n")
		sb.WriteString("    Or download from:\n")
		sb.WriteString("    https://git-scm.com/download/win\n")
	default:
		sb.WriteString("    Download from:\n")
		sb.WriteString("    https://git-scm.com/downloads\n")
	}

	return sb.String()
}
