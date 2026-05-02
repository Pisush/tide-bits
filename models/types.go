package models

import "time"

// QubitReport is a provider-agnostic summary of one qubit's health.
type QubitReport struct {
	Index        int     `json:"index"`
	ReadoutError float64 `json:"readout_error"`
	T1           float64 `json:"t1_us"`
	T2           float64 `json:"t2_us"`
}

// RawCalibrationData flows from Scout → Scribe.
type RawCalibrationData struct {
	ProviderName string
	FetchedAt    time.Time
	RawJSON      []byte
	Reports      []QubitReport
}

// HeatmapRequest flows from Scribe → Cartographer.
type HeatmapRequest struct {
	ProviderName string
	Date         string
	JSONPath     string
	Reports      []QubitReport
}
