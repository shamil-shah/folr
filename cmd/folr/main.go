// Package main provides the command-line interface entrypoint for folr.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shamil-shah/folr/internal/history"
	"github.com/shamil-shah/folr/internal/mover"
	"github.com/shamil-shah/folr/internal/plan"
	"github.com/shamil-shah/folr/internal/rules"
	"github.com/shamil-shah/folr/internal/safety"
	"github.com/shamil-shah/folr/internal/scan"
	"github.com/shamil-shah/folr/internal/ui"
)

const Version = "0.1.0"

func main() {
	exitCode := run(os.Args[1:])
	os.Exit(exitCode)
}

func run(args []string) int {
	args = normalizeArgs(args)
	fs := flag.NewFlagSet("folr", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		dryRun      bool
		yes         bool
		undo        bool
		configPath  string
		allowRoot   bool
		showVersion bool
	)

	fs.BoolVar(&dryRun, "dry-run", false, "Show organization plan without moving any files")
	fs.BoolVar(&yes, "y", false, "Do not prompt for confirmation")
	fs.BoolVar(&yes, "yes", false, "Do not prompt for confirmation")
	fs.BoolVar(&undo, "undo", false, "Revert the previous organization operation")
	fs.StringVar(&configPath, "config", "", "Path to custom rules.toml configuration file")
	fs.BoolVar(&allowRoot, "allow-root", false, "Permit running with root or administrator privileges")
	fs.BoolVar(&showVersion, "version", false, "Show version information")
	fs.BoolVar(&showVersion, "v", false, "Show version information")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "folr - Fast, safe, and atomic folder organizer (v%s)\n\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  folr [flags] [path]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fmt.Fprintf(os.Stderr, "  -y, --yes          Skip interactive confirmation\n")
		fmt.Fprintf(os.Stderr, "      --dry-run      Show organization plan without making any changes\n")
		fmt.Fprintf(os.Stderr, "      --undo         Revert the last organization run\n")
		fmt.Fprintf(os.Stderr, "      --config FILE  Path to custom rules.toml file\n")
		fmt.Fprintf(os.Stderr, "      --allow-root   Permit execution as root/administrator\n")
		fmt.Fprintf(os.Stderr, "  -v, --version      Display version\n")
		fmt.Fprintf(os.Stderr, "  -h, --help         Display this help menu\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  folr                     Organize current directory\n")
		fmt.Fprintf(os.Stderr, "  folr ~/Downloads         Organize ~/Downloads folder\n")
		fmt.Fprintf(os.Stderr, "  folr --dry-run           Inspect what would be moved\n")
		fmt.Fprintf(os.Stderr, "  folr --undo              Undo previous operation\n")
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return safety.ExitSuccess
		}
		return safety.ExitMisuse
	}

	if showVersion {
		fmt.Printf("folr version %s\n", Version)
		return safety.ExitSuccess
	}

	// Determine target path
	targetDir := "."
	if fs.NArg() == 1 {
		targetDir = fs.Arg(0)
	} else if fs.NArg() > 1 {
		// Handle unquoted folder names containing spaces (e.g. `folr new folder`)
		joined := strings.Join(fs.Args(), " ")
		if fi, err := os.Stat(joined); err == nil && fi.IsDir() {
			targetDir = joined
		} else {
			fmt.Fprintf(os.Stderr, "Error: multiple folder arguments provided. If your path contains spaces, please quote it: folr %q\n", joined)
			return safety.ExitMisuse
		}
	}

	// Clean target directory path
	targetDir = filepath.Clean(targetDir)

	// Validate target directory boundaries
	validatedDir, err := safety.ValidateTargetDirectory(targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		var secErr *safety.SecurityError
		if errors.As(err, &secErr) {
			return safety.ExitSecurityRejected
		}
		return safety.ExitGeneralError
	}

	// 5.9 Privilege check (root / Administrator)
	isPriv, privErr := safety.IsPrivilegedUser()
	if privErr == nil && isPriv {
		if !allowRoot {
			if !ui.IsTerminal() {
				fmt.Fprintln(os.Stderr, "Security error: running as root/administrator is prohibited without --allow-root")
				return safety.ExitSecurityRejected
			}
			fmt.Fprintln(os.Stderr, "WARNING: Running folr with root or administrator privileges is discouraged.")
			fmt.Fprint(os.Stderr, "Are you sure you want to proceed as privileged user? [y/N] ")
			if !ui.AskConfirmation(os.Stdin, os.Stderr) {
				fmt.Fprintln(os.Stderr, "Aborted by user.")
				return safety.ExitSecurityRejected
			}
		}
	}

	// Handle --undo
	if undo {
		return handleUndo(validatedDir)
	}

	// Non-interactive check (stdin is not a terminal and -y was not passed)
	if !dryRun && !yes && !ui.IsTerminal() {
		fmt.Fprintln(os.Stderr, "Error: standard input is not a terminal and -y was not provided. Exiting without modifying files.")
		return safety.ExitMisuse
	}

	// Load configuration rules
	ruleSet, err := rules.LoadRules(configPath, validatedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		var secErr *safety.SecurityError
		if errors.As(err, &secErr) {
			return safety.ExitSecurityRejected
		}
		return safety.ExitGeneralError
	}

	// Scan direct files
	scanResult, err := scan.ScanDirectory(validatedDir, ruleSet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scan error: %v\n", err)
		var secErr *safety.SecurityError
		if errors.As(err, &secErr) {
			return safety.ExitSecurityRejected
		}
		return safety.ExitGeneralError
	}

	// Build execution plan
	organizationPlan, err := plan.BuildPlan(scanResult, ruleSet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Plan building error: %v\n", err)
		var secErr *safety.SecurityError
		if errors.As(err, &secErr) {
			return safety.ExitSecurityRejected
		}
		return safety.ExitGeneralError
	}

	// Display planned operations
	ui.DisplayPlan(os.Stdout, organizationPlan)

	if len(organizationPlan.Actions) == 0 {
		fmt.Println("No files to organize.")
		return safety.ExitSuccess
	}

	// If dry-run, stop safely
	if dryRun {
		fmt.Println("(Dry-run mode: no files were moved)")
		return safety.ExitSuccess
	}

	// Prompt for confirmation if not -y
	if !yes {
		if !ui.AskConfirmation(os.Stdin, os.Stdout) {
			fmt.Println("Operation aborted.")
			return safety.ExitSuccess
		}
	}

	// Acquire lock file to prevent concurrent executions
	dirLock, err := safety.AcquireLock(validatedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lock error: %v\n", err)
		var secErr *safety.SecurityError
		if errors.As(err, &secErr) {
			return safety.ExitSecurityRejected
		}
		return safety.ExitGeneralError
	}
	defer dirLock.Release()

	// Initialize atomic history session
	histSession, err := history.StartSession(validatedDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "History initialization warning: %v\n", err)
	}

	// Execute move operations
	moveResult, err := mover.ExecutePlan(organizationPlan, histSession)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		return safety.ExitGeneralError
	}

	// Print summary
	ui.PrintSummary(os.Stdout, moveResult.MovedCount, moveResult.Messages)

	return safety.ExitSuccess
}

func handleUndo(targetDir string) int {
	opRecord, logPath, err := history.FindLatestOperation(targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Undo error: %v\n", err)
		return safety.ExitGeneralError
	}

	fmt.Printf("Undoing operation %s (run on %s)\n", opRecord.ID, opRecord.StartedAt.Format("2006-01-02 15:04:05"))

	// Acquire lock during undo
	dirLock, err := safety.AcquireLock(targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lock error: %v\n", err)
		return safety.ExitSecurityRejected
	}
	defer dirLock.Release()

	undoResult, err := history.ExecuteUndo(opRecord)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to execute undo: %v\n", err)
		return safety.ExitGeneralError
	}

	for _, msg := range undoResult.Messages {
		fmt.Fprintln(os.Stderr, msg)
	}

	fmt.Printf("Restored %d files.\n", undoResult.RestoredCount)

	// Remove or mark the completed history log
	_ = os.Remove(logPath)

	return safety.ExitSuccess
}

// normalizeArgs reorders arguments so that flags always precede positional arguments,
// enabling users to place flags anywhere on the command line.
func normalizeArgs(args []string) []string {
	var flags []string
	var nonFlags []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// String flag that takes a parameter (e.g. --config file.toml)
			if (arg == "--config" || arg == "-config") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			nonFlags = append(nonFlags, arg)
		}
	}
	return append(flags, nonFlags...)
}

