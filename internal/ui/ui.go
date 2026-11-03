// Package ui formats CLI output, displays execution plans, and handles interactive confirmation.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/shamil-shah/folr/internal/plan"
)

// IsTerminal returns true if stdin is connected to an interactive terminal device.
func IsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// DisplayPlan prints the organized category breakdown and skipped items matching folr specification.
func DisplayPlan(out io.Writer, p *plan.Plan) {
	fmt.Fprintf(out, "Scanning: %s\n", p.TargetDir)
	fmt.Fprintf(out, "%d files found\n", p.TotalScanned)

	// Collect and sort categories alphabetically
	var categories []string
	for cat := range p.CategoryCounts {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	// Find longest category name for uniform column alignment
	maxLen := len("Skipped")
	for _, cat := range categories {
		if len(cat) > maxLen {
			maxLen = len(cat)
		}
	}

	for _, cat := range categories {
		count := p.CategoryCounts[cat]
		samples := p.CategorySamples[cat]
		sampleText := strings.Join(samples, ", ")
		if count > len(samples) {
			sampleText += ", ..."
		}
		fmt.Fprintf(out, "%-*s (%d)  %s\n", maxLen, cat, count, sampleText)
	}

	if len(p.Skipped) > 0 {
		var reasons []string
		seen := make(map[string]bool)
		for _, s := range p.Skipped {
			if !seen[s.Reason] {
				seen[s.Reason] = true
				reasons = append(reasons, s.Reason)
			}
		}
		summary := strings.Join(reasons, ", ")
		fmt.Fprintf(out, "%-*s (%d)  %s\n", maxLen, "Skipped", len(p.Skipped), summary)
	}
}

// AskConfirmation prompts the user for interactive confirmation.
func AskConfirmation(in io.Reader, out io.Writer) bool {
	fmt.Fprintf(out, "Proceed? [y/N] ")
	scanner := bufio.NewScanner(in)
	if scanner.Scan() {
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return answer == "y" || answer == "yes"
	}
	return false
}

// PrintSummary prints the final status after moving.
func PrintSummary(out io.Writer, movedCount int, warnings []string) {
	for _, msg := range warnings {
		fmt.Fprintln(out, msg)
	}
	fmt.Fprintf(out, "Moved %d files.\n", movedCount)
	fmt.Fprintln(out, "To revert: folr --undo")
}

