package models

import "time"

// Nduv represents a single calibration parameter from IBM Quantum.
type Nduv struct {
	Name  string  `json:"name"`
	Date  string  `json:"date"`
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}

// GateProperties holds calibration data for a specific gate.
type GateProperties struct {
	Gate       string `json:"gate"`
	Qubits     []int  `json:"qubits"`
	Parameters []Nduv `json:"parameters"`
}

// BackendProperties is the top-level response from the IBM Quantum properties API.
type BackendProperties struct {
	BackendName    string           `json:"backend_name"`
	BackendVersion string           `json:"backend_version"`
	LastUpdateDate string           `json:"last_update_date"`
	Qubits         [][]Nduv         `json:"qubits"`
	Gates          []GateProperties `json:"gates"`
}

// QubitReport is a flattened, human-friendly summary of one qubit's health.
type QubitReport struct {
	Index        int     `json:"index"`
	ReadoutError float64 `json:"readout_error"`
	T1           float64 `json:"t1_us"`
	T2           float64 `json:"t2_us"`
}

// RawCalibrationData flows from Scout → Scribe.
type RawCalibrationData struct {
	FetchedAt  time.Time
	RawJSON    []byte
	Properties BackendProperties
	Reports    []QubitReport
}

// HeatmapRequest flows from Scribe → Cartographer.
type HeatmapRequest struct {
	Date     string
	JSONPath string
	Reports  []QubitReport
}
