package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shamil-shah/folr/internal/plan"
	"github.com/shamil-shah/folr/internal/scan"
)

func TestDisplayPlan(t *testing.T) {
	p := &plan.Plan{
		TargetDir:    "/test/downloads",
		TotalScanned: 10,
		CategoryCounts: map[string]int{
			"Images":    5,
			"Documents": 3,
		},
		CategorySamples: map[string][]string{
			"Images":    {"photo.jpg", "pic.png"},
			"Documents": {"report.pdf"},
		},
		Skipped: []scan.SkippedFile{
			{Name: "data.crdownload", Reason: "incomplete download"},
		},
	}

	var buf bytes.Buffer
	DisplayPlan(&buf, p)
	output := buf.String()

	if !strings.Contains(output, "Scanning: /test/downloads") {
		t.Errorf("missing target dir in output: %s", output)
	}
	if !strings.Contains(output, "Images") || !strings.Contains(output, "(5)") {
		t.Errorf("missing Images count in output: %s", output)
	}
	if !strings.Contains(output, "Skipped") || !strings.Contains(output, "incomplete download") {
		t.Errorf("missing Skipped section in output: %s", output)
	}
}

func TestAskConfirmation(t *testing.T) {
	// Test positive responses
	for _, yesInput := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		var out bytes.Buffer
		if !AskConfirmation(strings.NewReader(yesInput), &out) {
			t.Errorf("expected true for input %q", yesInput)
		}
	}

	// Test negative responses
	for _, noInput := range []string{"n\n", "N\n", "no\n", "\n", "cancel\n"} {
		var out bytes.Buffer
		if AskConfirmation(strings.NewReader(noInput), &out) {
			t.Errorf("expected false for input %q", noInput)
		}
	}
}

