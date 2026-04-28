package agents

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/natalie/tide-bits/models"
)

// Scribe persists calibration data to disk and forwards a heatmap request.
func Scribe(in <-chan models.RawCalibrationData, out chan<- models.HeatmapRequest) {
	defer close(out)

	data, ok := <-in
	if !ok {
		log.Println("Scribe: no data received from Scout")
		return
	}

	date := time.Now().Format("2006-01-02")
	dir := "quantum_weather_data"

	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("Scribe: failed to create data dir: %v", err)
		return
	}

	// Save raw JSON
	rawPath := filepath.Join(dir, date+".json")
	if err := os.WriteFile(rawPath, data.RawJSON, 0o644); err != nil {
		log.Printf("Scribe: failed to write raw JSON: %v", err)
		return
	}
	log.Printf("Scribe: saved raw calibration data → %s (%d bytes)", rawPath, len(data.RawJSON))

	// Save flattened summary
	summaryPath := filepath.Join(dir, date+"_summary.json")
	summaryJSON, err := json.MarshalIndent(data.Reports, "", "  ")
	if err != nil {
		log.Printf("Scribe: failed to marshal summary: %v", err)
		return
	}
	if err := os.WriteFile(summaryPath, summaryJSON, 0o644); err != nil {
		log.Printf("Scribe: failed to write summary: %v", err)
		return
	}
	log.Printf("Scribe: saved qubit summary → %s (%d qubits)", summaryPath, len(data.Reports))

	out <- models.HeatmapRequest{
		Date:     date,
		JSONPath: rawPath,
		Reports:  data.Reports,
	}
	log.Println("Scribe: heatmap request sent to Cartographer")
}
