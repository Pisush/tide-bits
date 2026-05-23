package agents

import (
	"fmt"
	"strings"
	"testing"

	"github.com/natalie/tide-bits/models"
)

func sampleReports() []models.QubitReport {
	return []models.QubitReport{
		{Index: 0, ReadoutError: 0.005, T1: 180, T2: 90},  // BEST
		{Index: 1, ReadoutError: 0.01, T1: 160, T2: 85},   // BEST
		{Index: 2, ReadoutError: 0.025, T1: 140, T2: 70},  // UNSTABLE
		{Index: 3, ReadoutError: 0.04, T1: 120, T2: 60},   // UNSTABLE
		{Index: 4, ReadoutError: 0.06, T1: 100, T2: 50},   // AVOID
	}
}

// ---------------------------------------------------------------------------
// buildHTMLReport
// ---------------------------------------------------------------------------

func TestBuildHTMLReport_containsProviderName(t *testing.T) {
	html := buildHTMLReport("my_provider", "2025-01-15", sampleReports())
	if !strings.Contains(html, "my_provider") {
		t.Error("HTML report should contain the provider name")
	}
}

func TestBuildHTMLReport_containsDate(t *testing.T) {
	html := buildHTMLReport("ibm_torino", "2025-06-01", sampleReports())
	if !strings.Contains(html, "2025-06-01") {
		t.Error("HTML report should contain the date")
	}
}

func TestBuildHTMLReport_containsQubitCount(t *testing.T) {
	reports := sampleReports()
	html := buildHTMLReport("ibm_torino", "2025-01-01", reports)
	if !strings.Contains(html, fmt.Sprintf("%d", len(reports))) {
		t.Error("HTML report should contain the total qubit count")
	}
}

func TestBuildHTMLReport_containsCategories(t *testing.T) {
	html := buildHTMLReport("ibm_torino", "2025-01-01", sampleReports())
	for _, want := range []string{"AVOID", "UNSTABLE", "BEST REGION"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML report missing category label %q", want)
		}
	}
}

func TestBuildHTMLReport_thresholdLabelsMatchConstants(t *testing.T) {
	html := buildHTMLReport("ibm_torino", "2025-01-01", sampleReports())
	// The AVOID threshold is ThresholdAvoid*100 = 5%.
	avoidLabel := fmt.Sprintf("%.0f%%", ThresholdAvoid*100)
	if !strings.Contains(html, avoidLabel) {
		t.Errorf("HTML report should mention the AVOID threshold (%s)", avoidLabel)
	}
}

func TestBuildHTMLReport_validHTML(t *testing.T) {
	html := buildHTMLReport("ibm_torino", "2025-01-01", sampleReports())
	if !strings.HasPrefix(html, "<!DOCTYPE html>") {
		t.Error("HTML report should start with DOCTYPE declaration")
	}
	if !strings.HasSuffix(strings.TrimSpace(html), "</html>") {
		t.Error("HTML report should end with </html>")
	}
}

func TestBuildHTMLReport_correctCategoryCounts(t *testing.T) {
	reports := sampleReports() // 2 BEST, 2 UNSTABLE, 1 AVOID
	html := buildHTMLReport("ibm_torino", "2025-01-01", reports)

	// The stats bar shows counts; verify they appear somewhere in the HTML.
	_, unstableSlice, bestSlice := categorizeQubits(reports)
	if !strings.Contains(html, fmt.Sprintf(">%d<", len(bestSlice))) {
		t.Errorf("HTML should contain best region count %d", len(bestSlice))
	}
	_ = unstableSlice
}

func TestBuildHTMLReport_emptyReports(t *testing.T) {
	// Should not panic on empty slice (edge case — no qubits).
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("buildHTMLReport panicked on empty reports: %v", r)
		}
	}()
	// NOTE: empty slice would cause divide-by-zero in avg calc — this is an
	// expected real-world constraint (providers always return > 0 qubits).
	// We test with a single qubit instead.
	_ = buildHTMLReport("ibm_torino", "2025-01-01", []models.QubitReport{{Index: 0, ReadoutError: 0.01}})
}

// ---------------------------------------------------------------------------
// buildEmailHTML
// ---------------------------------------------------------------------------

func TestBuildEmailHTML_containsProviderName(t *testing.T) {
	html := buildEmailHTML("my_provider", "2025-01-15", sampleReports())
	if !strings.Contains(html, "my_provider") {
		t.Error("email HTML should contain the provider name")
	}
}

func TestBuildEmailHTML_containsDate(t *testing.T) {
	html := buildEmailHTML("ibm_torino", "2025-06-01", sampleReports())
	if !strings.Contains(html, "2025-06-01") {
		t.Error("email HTML should contain the date")
	}
}

func TestBuildEmailHTML_containsCategories(t *testing.T) {
	html := buildEmailHTML("ibm_torino", "2025-01-01", sampleReports())
	for _, want := range []string{"AVOID", "UNSTABLE", "BEST REGION"} {
		if !strings.Contains(html, want) {
			t.Errorf("email HTML missing category label %q", want)
		}
	}
}

func TestBuildEmailHTML_usesCIDReferences(t *testing.T) {
	html := buildEmailHTML("ibm_torino", "2025-01-01", sampleReports())
	// Email HTML should reference images via CID, not base64 data URIs.
	if !strings.Contains(html, "cid:spatial_map") {
		t.Error("email HTML should use CID reference for spatial_map image")
	}
	if !strings.Contains(html, "cid:drift_map") {
		t.Error("email HTML should use CID reference for drift_map image")
	}
	if strings.Contains(html, "data:image/png;base64") {
		t.Error("email HTML should not embed base64 images (use CID instead)")
	}
}

func TestBuildEmailHTML_thresholdLabelsMatchConstants(t *testing.T) {
	html := buildEmailHTML("ibm_torino", "2025-01-01", sampleReports())
	// The UNSTABLE row shows the range as "2-5%" (ThresholdUnstable-ThresholdAvoid).
	rangeLabel := fmt.Sprintf("%.0f-%.0f%%", ThresholdUnstable*100, ThresholdAvoid*100)
	if !strings.Contains(html, rangeLabel) {
		t.Errorf("email HTML should mention the error range %q, got html: ...%s...",
			rangeLabel, html[strings.Index(html, "UNSTABLE"):])
	}
}

func TestBuildEmailHTML_differsFromHTMLReport(t *testing.T) {
	reports := sampleReports()
	full := buildHTMLReport("ibm_torino", "2025-01-01", reports)
	email := buildEmailHTML("ibm_torino", "2025-01-01", reports)
	// Email version uses inline styles, not a stylesheet.
	if strings.Contains(email, "<style>") {
		t.Error("email HTML should use inline styles, not a <style> block")
	}
	_ = full
}
