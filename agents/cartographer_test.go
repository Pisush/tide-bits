package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/natalie/tide-bits/models"
)

// ---------------------------------------------------------------------------
// categorizeQubits
// ---------------------------------------------------------------------------

func TestCategorizeQubits_thresholds(t *testing.T) {
	reports := []models.QubitReport{
		{Index: 0, ReadoutError: 0.001}, // BEST
		{Index: 1, ReadoutError: 0.019}, // BEST (just below 2%)
		{Index: 2, ReadoutError: 0.02},  // UNSTABLE (exactly at lower threshold)
		{Index: 3, ReadoutError: 0.035}, // UNSTABLE
		{Index: 4, ReadoutError: 0.049}, // UNSTABLE (just below AVOID)
		{Index: 5, ReadoutError: 0.05},  // AVOID (exactly at upper threshold)
		{Index: 6, ReadoutError: 0.08},  // AVOID
	}

	avoid, unstable, best := categorizeQubits(reports)

	if len(avoid) != 2 {
		t.Errorf("avoid: expected 2, got %d", len(avoid))
	}
	if len(unstable) != 3 {
		t.Errorf("unstable: expected 3, got %d", len(unstable))
	}
	if len(best) != 2 {
		t.Errorf("best: expected 2, got %d", len(best))
	}
}

func TestCategorizeQubits_empty(t *testing.T) {
	avoid, unstable, best := categorizeQubits(nil)
	if len(avoid)+len(unstable)+len(best) != 0 {
		t.Errorf("expected all empty for nil input")
	}
}

func TestCategorizeQubits_allBest(t *testing.T) {
	reports := []models.QubitReport{
		{Index: 0, ReadoutError: 0.005},
		{Index: 1, ReadoutError: 0.01},
	}
	avoid, unstable, best := categorizeQubits(reports)
	if len(avoid) != 0 || len(unstable) != 0 || len(best) != 2 {
		t.Errorf("expected 0/0/2, got %d/%d/%d", len(avoid), len(unstable), len(best))
	}
}

func TestCategorizeQubits_allAvoid(t *testing.T) {
	reports := []models.QubitReport{
		{Index: 0, ReadoutError: 0.06},
		{Index: 1, ReadoutError: 0.09},
	}
	avoid, unstable, best := categorizeQubits(reports)
	if len(avoid) != 2 || len(unstable) != 0 || len(best) != 0 {
		t.Errorf("expected 2/0/0, got %d/%d/%d", len(avoid), len(unstable), len(best))
	}
}

func TestCategorizeQubits_usesConstants(t *testing.T) {
	// Verify the thresholds match the exported constants.
	atAvoid := models.QubitReport{Index: 0, ReadoutError: ThresholdAvoid}
	atUnstable := models.QubitReport{Index: 1, ReadoutError: ThresholdUnstable}
	justBelowUnstable := models.QubitReport{Index: 2, ReadoutError: ThresholdUnstable - 0.0001}

	avoid, unstable, best := categorizeQubits([]models.QubitReport{atAvoid, atUnstable, justBelowUnstable})

	if len(avoid) != 1 || avoid[0].Index != 0 {
		t.Errorf("qubit at ThresholdAvoid should be AVOID")
	}
	if len(unstable) != 1 || unstable[0].Index != 1 {
		t.Errorf("qubit at ThresholdUnstable should be UNSTABLE")
	}
	if len(best) != 1 || best[0].Index != 2 {
		t.Errorf("qubit just below ThresholdUnstable should be BEST")
	}
}

// ---------------------------------------------------------------------------
// qubitIndices
// ---------------------------------------------------------------------------

func TestQubitIndices_sorted(t *testing.T) {
	reports := []models.QubitReport{
		{Index: 10},
		{Index: 3},
		{Index: 7},
	}
	ids := qubitIndices(reports)
	expected := []int{3, 7, 10}
	for i, v := range expected {
		if ids[i] != v {
			t.Errorf("ids[%d] = %d, want %d", i, ids[i], v)
		}
	}
}

func TestQubitIndices_empty(t *testing.T) {
	ids := qubitIndices(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty slice, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// intsToString
// ---------------------------------------------------------------------------

func TestIntsToString_basic(t *testing.T) {
	s := intsToString([]int{1, 2, 3})
	if s != "1, 2, 3" {
		t.Errorf("expected %q, got %q", "1, 2, 3", s)
	}
}

func TestIntsToString_single(t *testing.T) {
	s := intsToString([]int{42})
	if s != "42" {
		t.Errorf("expected %q, got %q", "42", s)
	}
}

func TestIntsToString_empty(t *testing.T) {
	s := intsToString(nil)
	if s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

// ---------------------------------------------------------------------------
// errorColorBWR
// ---------------------------------------------------------------------------

func TestErrorColorBWR_zero(t *testing.T) {
	c := errorColorBWR(0)
	// At 0 error: should be blue-ish (low red, low green, high blue)
	if c.B <= c.R || c.B <= c.G {
		t.Errorf("at 0 error expected blue-dominant color, got R=%d G=%d B=%d", c.R, c.G, c.B)
	}
}

func TestErrorColorBWR_max(t *testing.T) {
	c := errorColorBWR(0.05)
	// At max error: should be red-ish (high red, low green, low blue)
	if c.R <= c.G || c.R <= c.B {
		t.Errorf("at max error expected red-dominant color, got R=%d G=%d B=%d", c.R, c.G, c.B)
	}
}

func TestErrorColorBWR_midpoint(t *testing.T) {
	c := errorColorBWR(0.025)
	// Near the midpoint (0.025 = 50%) should be whitish (all channels high)
	if c.R < 200 || c.G < 200 || c.B < 200 {
		t.Errorf("at midpoint expected near-white, got R=%d G=%d B=%d", c.R, c.G, c.B)
	}
}

func TestErrorColorBWR_clampsBelow(t *testing.T) {
	c1 := errorColorBWR(-1.0)
	c2 := errorColorBWR(0)
	if c1 != c2 {
		t.Errorf("negative values should clamp to 0: %v vs %v", c1, c2)
	}
}

func TestErrorColorBWR_clampsAbove(t *testing.T) {
	c1 := errorColorBWR(1.0)
	c2 := errorColorBWR(0.05)
	if c1 != c2 {
		t.Errorf("values above max should clamp to 0.05: %v vs %v", c1, c2)
	}
}

// ---------------------------------------------------------------------------
// hexGridPositions
// ---------------------------------------------------------------------------

func TestHexGridPositions_count(t *testing.T) {
	for _, n := range []int{1, 10, 50, 125} {
		positions := hexGridPositions(n)
		if len(positions) != n {
			t.Errorf("hexGridPositions(%d) returned %d positions", n, len(positions))
		}
	}
}

func TestHexGridPositions_withinBounds(t *testing.T) {
	positions := hexGridPositions(125)
	for i, p := range positions {
		x, y := p[0], p[1]
		if x < mapLeft || x > mapRight {
			t.Errorf("position %d: x=%.1f out of bounds [%.1f, %.1f]", i, x, mapLeft, mapRight)
		}
		if y < mapTop || y > mapBottom {
			t.Errorf("position %d: y=%.1f out of bounds [%.1f, %.1f]", i, y, mapTop, mapBottom)
		}
	}
}

func TestHexGridPositions_zero(t *testing.T) {
	positions := hexGridPositions(0)
	if len(positions) != 0 {
		t.Errorf("expected 0 positions, got %d", len(positions))
	}
}

// ---------------------------------------------------------------------------
// loadHistoricalData
// ---------------------------------------------------------------------------

func TestLoadHistoricalData_emptyDir(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	days := loadHistoricalData("test_provider")
	if len(days) != 0 {
		t.Errorf("expected 0 days for empty directory, got %d", len(days))
	}
}

func TestLoadHistoricalData_returnsSortedDays(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join("quantum_weather_data", "test_provider")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	dates := []string{"2025-01-03", "2025-01-01", "2025-01-02"}
	for _, d := range dates {
		reports := []models.QubitReport{{Index: 0, ReadoutError: 0.01}}
		raw, _ := json.Marshal(reports)
		if err := os.WriteFile(filepath.Join(dir, d+"_summary.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	days := loadHistoricalData("test_provider")
	if len(days) != 3 {
		t.Fatalf("expected 3 days, got %d", len(days))
	}
	if days[0].date != "2025-01-01" || days[1].date != "2025-01-02" || days[2].date != "2025-01-03" {
		t.Errorf("days not sorted: %v", []string{days[0].date, days[1].date, days[2].date})
	}
}

func TestLoadHistoricalData_maxSevenDays(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join("quantum_weather_data", "test_provider")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write 10 days of data.
	for i := 1; i <= 10; i++ {
		date := fmt.Sprintf("2025-01-%02d", i)
		reports := []models.QubitReport{{Index: 0, ReadoutError: 0.01}}
		raw, _ := json.Marshal(reports)
		if err := os.WriteFile(filepath.Join(dir, date+"_summary.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	days := loadHistoricalData("test_provider")
	if len(days) != 7 {
		t.Errorf("expected at most 7 days, got %d", len(days))
	}
	// Should be the most recent 7.
	if days[0].date != "2025-01-04" {
		t.Errorf("oldest kept day should be 2025-01-04, got %s", days[0].date)
	}
}

func TestLoadHistoricalData_skipsInvalidFiles(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join("quantum_weather_data", "test_provider")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// One valid file, one corrupt.
	valid := []models.QubitReport{{Index: 0, ReadoutError: 0.01}}
	raw, _ := json.Marshal(valid)
	os.WriteFile(filepath.Join(dir, "2025-01-01_summary.json"), raw, 0o644)         //nolint:errcheck
	os.WriteFile(filepath.Join(dir, "2025-01-02_summary.json"), []byte("BAD JSON"), 0o644) //nolint:errcheck

	days := loadHistoricalData("test_provider")
	if len(days) != 1 {
		t.Errorf("expected 1 valid day, got %d", len(days))
	}
}
