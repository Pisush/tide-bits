package main

import (
	"flag"
	"log"
	"os"
	"sync"
	"time"

	"github.com/natalie/tide-bits/agents"
	"github.com/natalie/tide-bits/models"
	"github.com/natalie/tide-bits/providers"
	ibmprovider "github.com/natalie/tide-bits/providers/ibm"
)

func main() {
	watch        := flag.Bool("watch", false, "Run continuously on a schedule")
	interval     := flag.Duration("interval", 6*time.Hour, "How often to fetch (e.g. 6h, 1h, 30m)")
	providerFlag := flag.String("provider", "ibm", "Quantum provider to use (currently: ibm)")
	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)

	p, err := buildProvider(*providerFlag)
	if err != nil {
		log.Fatalf("Provider setup: %v", err)
	}

	if !*watch {
		log.Printf("Quantum Tide — %s Weather Report", p.Name())
		log.Println("========================================")
		ok := runPipeline(p)
		if !ok {
			os.Exit(1)
		}
		return
	}

	log.Printf("Quantum Tide — watch mode (every %s, provider: %s)", *interval, p.Name())
	log.Println("========================================")

	runPipeline(p)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for t := range ticker.C {
		log.Printf("Tick at %s — starting pipeline", t.Format(time.DateTime))
		runPipeline(p)
	}
}

// buildProvider resolves the -provider flag to a Provider implementation.
// Add new providers here as they are implemented.
func buildProvider(name string) (providers.Provider, error) {
	switch name {
	case "ibm":
		apiKey := os.Getenv("IBM_QUANTUM_TOKEN")
		crn := os.Getenv("IBM_QUANTUM_CRN")
		if apiKey == "" || crn == "" {
			log.Fatal("IBM provider requires IBM_QUANTUM_TOKEN and IBM_QUANTUM_CRN environment variables")
		}
		return ibmprovider.New(apiKey, crn), nil
	default:
		log.Fatalf("Unknown provider %q — available providers: ibm", name)
		return nil, nil // unreachable; log.Fatalf exits
	}
}

func runPipeline(p providers.Provider) bool {
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
		agents.Scout(p, scoutToScribe)
	}()

	wg.Wait()

	successFile := "output/" + p.Name() + "_spatial_map.png"
	if _, err := os.Stat(successFile); err == nil {
		agents.Broadcaster(p.Name())
		log.Println("Pipeline complete!")
		return true
	}

	log.Println("Pipeline finished with errors — check logs above")
	return false
}
