package main

import (
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"github.com/natalie/tide-bits/agents"
	"github.com/natalie/tide-bits/models"
)

func main() {
	watch := flag.Bool("watch", false, "Run continuously on a schedule")
	interval := flag.Duration("interval", 6*time.Hour, "How often to fetch (e.g. 6h, 1h, 30m)")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)

	apiKey := os.Getenv("IBM_QUANTUM_TOKEN")
	crn := os.Getenv("IBM_QUANTUM_CRN")

	if apiKey == "" || crn == "" {
		log.Fatal("Set IBM_QUANTUM_TOKEN and IBM_QUANTUM_CRN environment variables")
	}

	if !*watch {
		// Single run
		log.Println("Quantum Tide — IBM Torino Weather Report")
		log.Println("========================================")
		ok := runPipeline(apiKey, crn)
		if !ok {
			os.Exit(1)
		}
		return
	}

	// Watch mode — run immediately, then on a ticker
	log.Printf("Quantum Tide — watch mode (every %s)", *interval)
	log.Println("========================================")

	runPipeline(apiKey, crn)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for t := range ticker.C {
		log.Printf("Tick at %s — starting pipeline", t.Format(time.DateTime))
		runPipeline(apiKey, crn)
	}
}

func runPipeline(apiKey, crn string) bool {
	scoutToScribe := make(chan models.RawCalibrationData, 1)
	scribeToCartographer := make(chan models.HeatmapRequest, 1)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		agents.Cartographer(scribeToCartographer)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		agents.Scribe(scoutToScribe, scribeToCartographer)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		agents.Scout(apiKey, crn, scoutToScribe)
	}()

	wg.Wait()

	if _, err := os.Stat("output/torino_spatial_map.png"); err == nil {
		// Pipeline succeeded — broadcast (HTML report + optional email)
		agents.Broadcaster()
		log.Println("Pipeline complete!")
		return true
	}

	log.Println("Pipeline finished with errors — check logs above")
	return false
}
