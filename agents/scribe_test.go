package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/natalie/tide-bits/models"
)

func makeScribeInput(providerName string, reports []models.QubitReport) models.RawCalibrationData {
	return models.RawCalibrationData{
		ProviderName: providerName,
		FetchedAt:    time.Now(),
		RawJSON:      []byte(`{"backend_name":"` + providerName + `"}`),
		Reports:      reports,
	}
}

// ---------------------------------------------------------------------------
// Scribe
// ---------------------------------------------------------------------------

func TestScribe_createsProviderScopedDirectory(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	reports := []models.QubitReport{{Index: 0, ReadoutError: 0.01}}
	in := make(chan models.RawCalibrationData, 1)
	out := make(chan models.HeatmapRequest, 1)

	in <- makeScribeInput("fake_provider", reports)
	close(in)
	Scribe(in, out)
	<-out

	expectedDir := filepath.Join("quantum_weather_data", "fake_provider")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("expected directory %s to be created", expectedDir)
	}
}

func TestScribe_writesRawJSON(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	raw := []byte(`{"backend_name":"test_provider","qubits":[]}`)
	data := models.RawCalibrationData{
		ProviderName: "test_provider",
		FetchedAt:    time.Now(),
		RawJSON:      raw,
		Reports:      []models.QubitReport{{Index: 0}},
	}

	in := make(chan models.RawCalibrationData, 1)
	out := make(chan models.HeatmapRequest, 1)
	in <- data
	close(in)
	Scribe(in, out)
	req := <-out

	savedRaw, err := os.ReadFile(req.JSONPath)
	if err != nil {
		t.Fatalf("failed to read raw JSON file: %v", err)
	}
	if string(savedRaw) != string(raw) {
		t.Errorf("raw JSON mismatch:\n got: %s\nwant: %s", savedRaw, raw)
	}
}

func TestScribe_writesSummaryJSON(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	reports := []models.QubitReport{
		{Index: 0, ReadoutError: 0.012, T1: 150, T2: 80},
		{Index: 1, ReadoutError: 0.034, T1: 100, T2: 60},
	}
	in := make(chan models.RawCalibrationData, 1)
	out := make(chan models.HeatmapRequest, 1)
	in <- makeScribeInput("test_provider", reports)
	close(in)
	Scribe(in, out)
	<-out

	date := time.Now().Format("2006-01-02")
	summaryPath := filepath.Join("quantum_weather_data", "test_provider", date+"_summary.json")
	raw, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("summary file not found: %v", err)
	}

	var saved []models.QubitReport
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatalf("summary JSON invalid: %v", err)
	}
	if len(saved) != len(reports) {
		t.Errorf("expected %d reports in summary, got %d", len(reports), len(saved))
	}
	if saved[0].ReadoutError != reports[0].ReadoutError {
		t.Errorf("ReadoutError mismatch: got %v, want %v", saved[0].ReadoutError, reports[0].ReadoutError)
	}
}

func TestScribe_forwardsHeatmapRequest(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	reports := []models.QubitReport{{Index: 0, ReadoutError: 0.01}}
	in := make(chan models.RawCalibrationData, 1)
	out := make(chan models.HeatmapRequest, 1)
	in <- makeScribeInput("test_provider", reports)
	close(in)
	Scribe(in, out)

	req, ok := <-out
	if !ok {
		t.Fatal("Scribe closed output channel without sending HeatmapRequest")
	}

	if req.ProviderName != "test_provider" {
		t.Errorf("ProviderName = %q, want %q", req.ProviderName, "test_provider")
	}
	if req.Date == "" {
		t.Error("Date should be set in HeatmapRequest")
	}
	if req.JSONPath == "" {
		t.Error("JSONPath should be set in HeatmapRequest")
	}
	if len(req.Reports) != len(reports) {
		t.Errorf("expected %d reports in request, got %d", len(reports), len(req.Reports))
	}
}

func TestScribe_closesOutputChannelOnEmptyInput(t *testing.T) {
	in := make(chan models.RawCalibrationData)
	out := make(chan models.HeatmapRequest, 1)
	close(in)

	Scribe(in, out)

	_, open := <-out
	if open {
		t.Error("output channel should be closed when input is empty")
	}
}

func TestScribe_differentProvidersDontShareDirs(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	for _, providerName := range []string{"provider_a", "provider_b"} {
		in := make(chan models.RawCalibrationData, 1)
		out := make(chan models.HeatmapRequest, 1)
		in <- makeScribeInput(providerName, []models.QubitReport{{Index: 0, ReadoutError: 0.01}})
		close(in)
		Scribe(in, out)
		<-out
	}

	for _, providerName := range []string{"provider_a", "provider_b"} {
		dir := filepath.Join("quantum_weather_data", providerName)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("expected separate directory for %s", providerName)
		}
	}
}
