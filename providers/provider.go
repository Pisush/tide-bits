package providers

import "github.com/natalie/tide-bits/models"

// Provider is the interface any quantum backend must implement.
type Provider interface {
	// Name returns the unique identifier for this backend (e.g. "ibm_torino").
	Name() string
	// Fetch retrieves current qubit calibration data.
	Fetch() (rawJSON []byte, reports []models.QubitReport, err error)
}
