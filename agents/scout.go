package agents

import (
	"log"
	"time"

	"github.com/natalie/tide-bits/models"
	"github.com/natalie/tide-bits/providers"
)

// Scout calls the given provider and forwards the result downstream.
func Scout(p providers.Provider, out chan<- models.RawCalibrationData) {
	defer close(out)

	log.Printf("Scout: fetching from provider %q...", p.Name())
	rawJSON, reports, err := p.Fetch()
	if err != nil {
		log.Printf("Scout: %v", err)
		return
	}
	log.Printf("Scout: received %d qubit reports from %s", len(reports), p.Name())

	out <- models.RawCalibrationData{
		ProviderName: p.Name(),
		FetchedAt:    time.Now(),
		RawJSON:      rawJSON,
		Reports:      reports,
	}
	log.Println("Scout: data sent to Scribe")
}
