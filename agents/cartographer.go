package agents

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/natalie/tide-bits/models"
)

const (
	canvasW = 1400
	canvasH = 950
	hexSize = 36.0

	mapLeft   = 40.0
	mapTop    = 75.0
	mapRight  = 1100.0
	mapBottom = 910.0

	legLeft   = 1160.0
	legRight  = 1205.0
	legTop    = 100.0
	legBottom = 880.0
)

var (
	mapCX = (mapLeft + mapRight) / 2
	mapCY = (mapTop + mapBottom) / 2
)

// Cartographer renders spatial heatmap + temporal drift map.
func Cartographer(in <-chan models.HeatmapRequest) {
	req, ok := <-in
	if !ok {
		log.Println("Cartographer: no heatmap request received")
		return
	}

	log.Printf("Cartographer: rendering heatmap for %s (%d qubits)", req.Date, len(req.Reports))

	if err := os.MkdirAll("output", 0o755); err != nil {
		log.Printf("Cartographer: failed to create output dir: %v", err)
		return
	}

	// Spatial heatmap
	spatialPath := filepath.Join("output", "torino_spatial_map.png")
	if err := renderHeatmap(req, spatialPath); err != nil {
		log.Printf("Cartographer: spatial render failed: %v", err)
	} else {
		log.Printf("Cartographer: spatial heatmap saved → %s", spatialPath)
	}

	// Temporal drift map
	driftPath := filepath.Join("output", "torino_drift_map.png")
	if err := renderDriftMap(driftPath); err != nil {
		log.Printf("Cartographer: drift render failed: %v", err)
	} else {
		log.Printf("Cartographer: drift map saved → %s", driftPath)
	}

	// Machine-readable summaries for agents
	summaryPath := filepath.Join("output", "torino_summary.md")
	if err := writeSummary(req, summaryPath); err != nil {
		log.Printf("Cartographer: summary write failed: %v", err)
	} else {
		log.Printf("Cartographer: summary saved → %s", summaryPath)
	}

	driftSummaryPath := filepath.Join("output", "torino_drift_summary.md")
	if err := writeDriftSummary(driftSummaryPath); err != nil {
		log.Printf("Cartographer: drift summary write failed: %v", err)
	} else {
		log.Printf("Cartographer: drift summary saved → %s", driftSummaryPath)
	}

	// Action list PNG (Do Not Fly)
	actionPath := filepath.Join("output", "torino_action_list.png")
	if err := renderActionList(req.Reports, actionPath); err != nil {
		log.Printf("Cartographer: action list render failed: %v", err)
	} else {
		log.Printf("Cartographer: action list saved → %s", actionPath)
	}

	// Agent daily briefing (single comprehensive file)
	briefingPath := filepath.Join("output", "torino_daily_briefing.md")
	if err := writeAgentBriefing(req.Reports, briefingPath); err != nil {
		log.Printf("Cartographer: agent briefing failed: %v", err)
	} else {
		log.Printf("Cartographer: agent briefing saved → %s", briefingPath)
	}
}

// ---------------------------------------------------------------------------
// Spatial Heatmap
// ---------------------------------------------------------------------------

func renderHeatmap(req models.HeatmapRequest, outPath string) error {
	dc := gg.NewContext(canvasW, canvasH)

	dc.SetColor(color.RGBA{R: 20, G: 22, B: 30, A: 255})
	dc.Clear()

	// Frame
	dc.SetColor(color.RGBA{R: 155, G: 160, B: 175, A: 255})
	dc.SetLineWidth(2)
	dc.DrawRoundedRectangle(mapLeft-15, mapTop-15, mapRight-mapLeft+30, mapBottom-mapTop+30, 10)
	dc.Stroke()
	dc.SetColor(color.RGBA{R: 26, G: 28, B: 38, A: 255})
	dc.DrawRoundedRectangle(mapLeft-14, mapTop-14, mapRight-mapLeft+28, mapBottom-mapTop+28, 9)
	dc.Fill()

	// Title
	loadFont(dc, 20)
	dc.SetColor(color.RGBA{R: 220, G: 225, B: 235, A: 255})
	dc.DrawStringAnchored("DAILY QUANTUM TIDE REPORT - SPATIAL HEATMAP (IBM_TORINO)", canvasW/2, 35, 0.5, 0.5)

	// Timestamp subtitle
	loadFont(dc, 13)
	dc.SetColor(color.RGBA{R: 140, G: 145, B: 165, A: 255})
	ts := time.Now().Format("2006-01-02 15:04 MST")
	dc.DrawStringAnchored(ts, canvasW/2, 56, 0.5, 0.5)

	positions := hexGridPositions(len(req.Reports))

	errorMap := make(map[int]float64)
	for _, r := range req.Reports {
		errorMap[r.Index] = r.ReadoutError
	}

	for i, pos := range positions {
		drawHexFlat(dc, pos[0], pos[1], hexSize, errorColorBWR(errorMap[i]))
	}

	// Qubit index labels inside each hexagon
	loadFont(dc, 9)
	for i, pos := range positions {
		c := errorColorBWR(errorMap[i])
		// Pick text color based on background luminance
		lum := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
		if lum > 140 {
			dc.SetColor(color.RGBA{R: 20, G: 20, B: 30, A: 255})
		} else {
			dc.SetColor(color.RGBA{R: 230, G: 235, B: 245, A: 255})
		}
		dc.DrawStringAnchored(fmt.Sprintf("%d", i), pos[0], pos[1], 0.5, 0.5)
	}

	drawVerticalLegend(dc, legLeft, legRight, legTop, legBottom)

	return dc.SavePNG(outPath)
}

// ---------------------------------------------------------------------------
// Temporal Drift Map
// ---------------------------------------------------------------------------

type daySnapshot struct {
	date    string
	reports []models.QubitReport
}

func loadHistoricalData() []daySnapshot {
	files, err := filepath.Glob("quantum_weather_data/*_summary.json")
	if err != nil || len(files) == 0 {
		return nil
	}

	var days []daySnapshot
	for _, f := range files {
		base := filepath.Base(f)
		date := strings.TrimSuffix(base, "_summary.json")
		if !strings.Contains(base, "_summary") {
			continue
		}

		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var reports []models.QubitReport
		if err := json.Unmarshal(raw, &reports); err != nil {
			continue
		}
		days = append(days, daySnapshot{date, reports})
	}

	sort.Slice(days, func(i, j int) bool {
		return days[i].date < days[j].date
	})

	// Keep last 7 days
	if len(days) > 7 {
		days = days[len(days)-7:]
	}
	return days
}

func renderDriftMap(outPath string) error {
	days := loadHistoricalData()
	if len(days) == 0 {
		return fmt.Errorf("no historical data found")
	}

	// Find max qubit count across all days
	maxQubits := 0
	for _, d := range days {
		if len(d.reports) > maxQubits {
			maxQubits = len(d.reports)
		}
	}

	dc := gg.NewContext(canvasW, canvasH)

	// Dark background
	dc.SetColor(color.RGBA{R: 20, G: 22, B: 30, A: 255})
	dc.Clear()

	// Grid area
	gridLeft := 100.0
	gridTop := 70.0
	gridRight := 1100.0
	gridBottom := 890.0
	gridW := gridRight - gridLeft
	gridH := gridBottom - gridTop

	// White frame
	dc.SetColor(color.RGBA{R: 240, G: 240, B: 245, A: 255})
	dc.DrawRoundedRectangle(gridLeft-4, gridTop-4, gridW+8, gridH+8, 6)
	dc.Fill()

	// Title
	loadFont(dc, 20)
	dc.SetColor(color.RGBA{R: 220, G: 225, B: 235, A: 255})
	title := fmt.Sprintf("TEMPORAL DRIFT HEATMAP (LAST %d DAYS)", len(days))
	dc.DrawStringAnchored(title, canvasW/2, 35, 0.5, 0.5)

	// Draw grid cells
	colW := gridW / float64(len(days))
	rowH := gridH / float64(maxQubits)

	for col, day := range days {
		errMap := make(map[int]float64)
		for _, r := range day.reports {
			errMap[r.Index] = r.ReadoutError
		}
		x := gridLeft + float64(col)*colW
		for q := 0; q < maxQubits; q++ {
			// Y: qubit 0 at bottom, max at top
			y := gridBottom - float64(q+1)*rowH
			c := errorColorBWR(errMap[q])
			dc.SetColor(c)
			dc.DrawRectangle(x, y, colW+1, rowH+1)
			dc.Fill()
		}
	}

	// Y-axis labels (Qubit Index)
	loadFont(dc, 11)
	dc.SetColor(color.RGBA{R: 195, G: 200, B: 215, A: 255})
	step := 10
	if maxQubits > 100 {
		step = maxQubits / 10
		// Round to nearest even number
		if step%2 != 0 {
			step++
		}
	}
	for q := 0; q <= maxQubits; q += step {
		y := gridBottom - float64(q)*rowH
		dc.DrawStringAnchored(fmt.Sprintf("%d", q), gridLeft-10, y, 1, 0.5)
		// Tick
		dc.DrawLine(gridLeft-4, y, gridLeft, y)
		dc.Stroke()
	}

	// Y-axis title
	loadFont(dc, 14)
	dc.Push()
	dc.RotateAbout(math.Pi/2, 30, (gridTop+gridBottom)/2)
	dc.DrawStringAnchored("Qubit Index", 30, (gridTop+gridBottom)/2, 0.5, 0.5)
	dc.Pop()

	// X-axis labels (dates / days ago)
	loadFont(dc, 11)
	dc.SetColor(color.RGBA{R: 195, G: 200, B: 215, A: 255})
	today := time.Now().Truncate(24 * time.Hour)
	for col, day := range days {
		x := gridLeft + float64(col)*colW + colW/2
		t, err := time.Parse("2006-01-02", day.date)
		var label string
		if err == nil {
			daysAgo := int(today.Sub(t).Hours() / 24)
			if daysAgo == 0 {
				label = "Today"
			} else if daysAgo == 1 {
				label = "1"
			} else {
				label = fmt.Sprintf("%d", daysAgo)
			}
		} else {
			label = day.date
		}
		dc.DrawStringAnchored(label, x, gridBottom+18, 0.5, 0.5)
	}

	// X-axis title
	loadFont(dc, 14)
	dc.SetColor(color.RGBA{R: 195, G: 200, B: 215, A: 255})
	dc.DrawStringAnchored("Time (Days Ago)", (gridLeft+gridRight)/2, gridBottom+40, 0.5, 0.5)

	// Legend (reuse same style, positioned to the right)
	dLegLeft := 1160.0
	dLegRight := 1205.0
	dLegTop := 100.0
	dLegBottom := 880.0
	drawVerticalLegend(dc, dLegLeft, dLegRight, dLegTop, dLegBottom)

	return dc.SavePNG(outPath)
}

// ---------------------------------------------------------------------------
// Agent-readable summary
// ---------------------------------------------------------------------------

func writeSummary(req models.HeatmapRequest, outPath string) error {
	ts := time.Now().Format("2006-01-02 15:04 MST")

	// Sort reports by error descending
	sorted := make([]models.QubitReport, len(req.Reports))
	copy(sorted, req.Reports)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ReadoutError > sorted[j].ReadoutError
	})

	// Compute stats
	var sum float64
	worst := sorted[0]
	best := sorted[len(sorted)-1]
	for _, r := range sorted {
		sum += r.ReadoutError
	}
	avg := sum / float64(len(sorted))

	// Collect qubits above thresholds
	var above4, above3, above2 []int
	for _, r := range sorted {
		pct := r.ReadoutError * 100
		if pct >= 4.0 {
			above4 = append(above4, r.Index)
		}
		if pct >= 3.0 {
			above3 = append(above3, r.Index)
		}
		if pct >= 2.0 {
			above2 = append(above2, r.Index)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# IBM Torino — Qubit Health Summary\n\n")
	fmt.Fprintf(&b, "**Date:** %s\n\n", ts)
	fmt.Fprintf(&b, "**Backend:** ibm_torino (%d qubits)\n\n", len(req.Reports))
	fmt.Fprintf(&b, "## Overview\n\n")
	fmt.Fprintf(&b, "| Metric | Value |\n")
	fmt.Fprintf(&b, "|--------|-------|\n")
	fmt.Fprintf(&b, "| Mean readout error | %.4f (%.2f%%) |\n", avg, avg*100)
	fmt.Fprintf(&b, "| Best qubit | Q%d — %.4f (%.2f%%) |\n", best.Index, best.ReadoutError, best.ReadoutError*100)
	fmt.Fprintf(&b, "| Worst qubit | Q%d — %.4f (%.2f%%) |\n", worst.Index, worst.ReadoutError, worst.ReadoutError*100)
	fmt.Fprintf(&b, "| Qubits > 4%% error | %d |\n", len(above4))
	fmt.Fprintf(&b, "| Qubits > 3%% error | %d |\n", len(above3))
	fmt.Fprintf(&b, "| Qubits > 2%% error | %d |\n", len(above2))

	if len(above4) > 0 {
		fmt.Fprintf(&b, "\n## Critical Qubits (>4%% error)\n\n")
		for _, idx := range above4 {
			for _, r := range req.Reports {
				if r.Index == idx {
					fmt.Fprintf(&b, "- **Q%d** — %.2f%% readout error (T1=%.1fµs, T2=%.1fµs)\n",
						r.Index, r.ReadoutError*100, r.T1, r.T2)
				}
			}
		}
	}

	fmt.Fprintf(&b, "\n## Top 10 Worst Qubits\n\n")
	fmt.Fprintf(&b, "| Qubit | Readout Error | T1 (µs) | T2 (µs) |\n")
	fmt.Fprintf(&b, "|-------|--------------|---------|--------|\n")
	limit := 10
	if len(sorted) < limit {
		limit = len(sorted)
	}
	for _, r := range sorted[:limit] {
		fmt.Fprintf(&b, "| Q%d | %.2f%% | %.1f | %.1f |\n",
			r.Index, r.ReadoutError*100, r.T1, r.T2)
	}

	fmt.Fprintf(&b, "\n## Top 10 Best Qubits\n\n")
	fmt.Fprintf(&b, "| Qubit | Readout Error | T1 (µs) | T2 (µs) |\n")
	fmt.Fprintf(&b, "|-------|--------------|---------|--------|\n")
	for i := len(sorted) - 1; i >= 0 && i >= len(sorted)-10; i-- {
		r := sorted[i]
		fmt.Fprintf(&b, "| Q%d | %.2f%% | %.1f | %.1f |\n",
			r.Index, r.ReadoutError*100, r.T1, r.T2)
	}

	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

func writeDriftSummary(outPath string) error {
	days := loadHistoricalData()
	if len(days) == 0 {
		return fmt.Errorf("no historical data")
	}

	ts := time.Now().Format("2006-01-02 15:04 MST")

	var b strings.Builder
	fmt.Fprintf(&b, "# IBM Torino — Temporal Drift Summary\n\n")
	fmt.Fprintf(&b, "**Generated:** %s\n\n", ts)
	fmt.Fprintf(&b, "**Window:** %d day(s) (%s → %s)\n\n", len(days), days[0].date, days[len(days)-1].date)

	// Per-day mean error
	fmt.Fprintf(&b, "## Daily Mean Readout Error\n\n")
	fmt.Fprintf(&b, "| Date | Mean Error | Worst Qubit | Worst Error |\n")
	fmt.Fprintf(&b, "|------|-----------|-------------|-------------|\n")
	for _, d := range days {
		var sum float64
		var worst models.QubitReport
		for _, r := range d.reports {
			sum += r.ReadoutError
			if r.ReadoutError > worst.ReadoutError {
				worst = r
			}
		}
		mean := sum / float64(len(d.reports))
		fmt.Fprintf(&b, "| %s | %.2f%% | Q%d | %.2f%% |\n",
			d.date, mean*100, worst.Index, worst.ReadoutError*100)
	}

	if len(days) >= 2 {
		first := days[0]
		last := days[len(days)-1]

		// Build per-qubit error maps for first and last day
		firstErr := make(map[int]float64)
		for _, r := range first.reports {
			firstErr[r.Index] = r.ReadoutError
		}
		lastErr := make(map[int]float64)
		for _, r := range last.reports {
			lastErr[r.Index] = r.ReadoutError
		}

		// Compute deltas
		type delta struct {
			index      int
			firstVal   float64
			lastVal    float64
			changeAbs  float64
			changePct  float64
		}
		var deltas []delta
		for idx, lv := range lastErr {
			fv := firstErr[idx]
			d := delta{
				index:     idx,
				firstVal:  fv,
				lastVal:   lv,
				changeAbs: lv - fv,
			}
			if fv > 0 {
				d.changePct = (lv - fv) / fv * 100
			}
			deltas = append(deltas, d)
		}

		// Sort by change (most degraded first)
		sort.Slice(deltas, func(i, j int) bool {
			return deltas[i].changeAbs > deltas[j].changeAbs
		})

		// Degraded qubits (error increased significantly)
		var degraded, improved []delta
		for _, d := range deltas {
			if d.changeAbs > 0.005 { // >0.5% increase
				degraded = append(degraded, d)
			} else if d.changeAbs < -0.005 { // >0.5% decrease
				improved = append(improved, d)
			}
		}

		fmt.Fprintf(&b, "\n## Trend Analysis (%s → %s)\n\n", first.date, last.date)

		// Overall
		var firstSum, lastSum float64
		for _, r := range first.reports {
			firstSum += r.ReadoutError
		}
		for _, r := range last.reports {
			lastSum += r.ReadoutError
		}
		firstMean := firstSum / float64(len(first.reports))
		lastMean := lastSum / float64(len(last.reports))
		direction := "stable"
		if lastMean-firstMean > 0.002 {
			direction = "DEGRADING"
		} else if firstMean-lastMean > 0.002 {
			direction = "IMPROVING"
		}
		fmt.Fprintf(&b, "**Overall trend:** %s (mean %.2f%% → %.2f%%)\n\n", direction, firstMean*100, lastMean*100)
		fmt.Fprintf(&b, "- Qubits degraded (>0.5%% increase): %d\n", len(degraded))
		fmt.Fprintf(&b, "- Qubits improved (>0.5%% decrease): %d\n\n", len(improved))

		if len(degraded) > 0 {
			fmt.Fprintf(&b, "### Most Degraded Qubits\n\n")
			fmt.Fprintf(&b, "| Qubit | %s | %s | Change |\n", first.date, last.date)
			fmt.Fprintf(&b, "|-------|--------|--------|--------|\n")
			limit := 10
			if len(degraded) < limit {
				limit = len(degraded)
			}
			for _, d := range degraded[:limit] {
				fmt.Fprintf(&b, "| Q%d | %.2f%% | %.2f%% | +%.2f%% |\n",
					d.index, d.firstVal*100, d.lastVal*100, d.changeAbs*100)
			}
		}

		if len(improved) > 0 {
			fmt.Fprintf(&b, "\n### Most Improved Qubits\n\n")
			fmt.Fprintf(&b, "| Qubit | %s | %s | Change |\n", first.date, last.date)
			fmt.Fprintf(&b, "|-------|--------|--------|--------|\n")
			// Sort improved by most improvement (most negative change)
			sort.Slice(improved, func(i, j int) bool {
				return improved[i].changeAbs < improved[j].changeAbs
			})
			limit := 10
			if len(improved) < limit {
				limit = len(improved)
			}
			for _, d := range improved[:limit] {
				fmt.Fprintf(&b, "| Q%d | %.2f%% | %.2f%% | %.2f%% |\n",
					d.index, d.firstVal*100, d.lastVal*100, d.changeAbs*100)
			}
		}

		// Most volatile (biggest absolute swings across ALL days)
		if len(days) >= 3 {
			type volatility struct {
				index    int
				minErr   float64
				maxErr   float64
				swing    float64
			}
			volMap := make(map[int]*volatility)
			for _, day := range days {
				for _, r := range day.reports {
					v, ok := volMap[r.Index]
					if !ok {
						v = &volatility{index: r.Index, minErr: r.ReadoutError, maxErr: r.ReadoutError}
						volMap[r.Index] = v
					}
					if r.ReadoutError < v.minErr {
						v.minErr = r.ReadoutError
					}
					if r.ReadoutError > v.maxErr {
						v.maxErr = r.ReadoutError
					}
					v.swing = v.maxErr - v.minErr
				}
			}
			var vols []volatility
			for _, v := range volMap {
				vols = append(vols, *v)
			}
			sort.Slice(vols, func(i, j int) bool {
				return vols[i].swing > vols[j].swing
			})
			fmt.Fprintf(&b, "\n### Most Volatile Qubits (largest swing over %d days)\n\n", len(days))
			fmt.Fprintf(&b, "| Qubit | Min Error | Max Error | Swing |\n")
			fmt.Fprintf(&b, "|-------|----------|----------|-------|\n")
			limit := 10
			if len(vols) < limit {
				limit = len(vols)
			}
			for _, v := range vols[:limit] {
				fmt.Fprintf(&b, "| Q%d | %.2f%% | %.2f%% | %.2f%% |\n",
					v.index, v.minErr*100, v.maxErr*100, v.swing*100)
			}
		}
	} else {
		fmt.Fprintf(&b, "\n*Trend analysis requires 2+ days of data. Check back tomorrow.*\n")
	}

	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Action List (Do Not Fly) PNG
// ---------------------------------------------------------------------------

// categorizeQubits splits qubits into AVOID / UNSTABLE / BEST REGION.
func categorizeQubits(reports []models.QubitReport) (avoid, unstable, best []models.QubitReport) {
	for _, r := range reports {
		switch {
		case r.ReadoutError >= 0.05:
			avoid = append(avoid, r)
		case r.ReadoutError >= 0.02:
			unstable = append(unstable, r)
		default:
			best = append(best, r)
		}
	}
	return
}

func qubitIndices(reports []models.QubitReport) []int {
	ids := make([]int, len(reports))
	for i, r := range reports {
		ids[i] = r.Index
	}
	sort.Ints(ids)
	return ids
}

func intsToString(ids []int) string {
	strs := make([]string, len(ids))
	for i, v := range ids {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(strs, ", ")
}

func renderActionList(reports []models.QubitReport, outPath string) error {
	avoid, unstable, best := categorizeQubits(reports)

	avoidStr := intsToString(qubitIndices(avoid))
	unstableStr := intsToString(qubitIndices(unstable))
	bestStr := intsToString(qubitIndices(best))

	// Measure text to determine row heights
	tmpDc := gg.NewContext(1, 1)
	loadFont(tmpDc, 14)

	colW := [3]float64{180, 680, 280}
	tableW := colW[0] + colW[1] + colW[2]
	pad := 14.0
	lineH := 22.0

	wrap := func(s string) []string {
		return tmpDc.WordWrap(s, colW[1]-2*pad)
	}

	avoidLines := wrap(avoidStr)
	unstableLines := wrap(unstableStr)
	bestLines := wrap(bestStr)

	rowH := func(lines []string) float64 {
		h := float64(len(lines))*lineH + 2*pad
		if h < 60 {
			return 60
		}
		return h
	}

	avoidH := rowH(avoidLines)
	unstableH := rowH(unstableLines)
	bestH := rowH(bestLines)

	titleH := 55.0
	headerH := 48.0
	margin := 20.0
	totalH := titleH + headerH + avoidH + unstableH + bestH + margin
	cW := int(tableW + 2*margin)
	cH := int(totalH)

	dc := gg.NewContext(cW, cH)
	dc.SetColor(color.White)
	dc.Clear()

	// Title
	loadFont(dc, 20)
	dc.SetColor(color.Black)
	dc.DrawStringAnchored("DAILY ACTION LIST (DO NOT FLY)", float64(cW)/2, titleH/2, 0.5, 0.5)

	y := titleH
	x0 := margin

	// Header
	dc.SetColor(color.RGBA{R: 235, G: 235, B: 235, A: 255})
	dc.DrawRectangle(x0, y, tableW, headerH)
	dc.Fill()

	dc.SetColor(color.Black)
	dc.SetLineWidth(2)
	dc.DrawRectangle(x0, y, tableW, headerH+avoidH+unstableH+bestH)
	dc.Stroke()

	loadFont(dc, 16)
	dc.SetColor(color.Black)
	headers := []string{"Status", "Qubits", "Reason"}
	cx := x0
	for i, h := range headers {
		dc.DrawStringAnchored(h, cx+colW[i]/2, y+headerH/2, 0.5, 0.5)
		cx += colW[i]
	}

	// Column dividers
	dc.SetLineWidth(1)
	cx = x0
	for i := 0; i <= 3; i++ {
		dc.DrawLine(cx, y, cx, y+headerH+avoidH+unstableH+bestH)
		dc.Stroke()
		if i < 3 {
			cx += colW[i]
		}
	}

	// Row dividers
	for _, ry := range []float64{y + headerH, y + headerH + avoidH, y + headerH + avoidH + unstableH} {
		dc.DrawLine(x0, ry, x0+tableW, ry)
		dc.Stroke()
	}

	// Data rows
	type rowDef struct {
		label  string
		c      color.RGBA
		lines  []string
		reason string
		h      float64
	}
	rows := []rowDef{
		{"AVOID", color.RGBA{R: 200, G: 0, B: 0, A: 255}, avoidLines, "High Persistent Noise", avoidH},
		{"UNSTABLE", color.RGBA{R: 200, G: 140, B: 0, A: 255}, unstableLines, "Significant Drift", unstableH},
		{"BEST REGION", color.RGBA{R: 0, G: 140, B: 0, A: 255}, bestLines, "Stable, Low Error", bestH},
	}

	ry := y + headerH
	for _, row := range rows {
		// Status label
		loadFont(dc, 16)
		dc.SetColor(row.c)
		dc.DrawStringAnchored(row.label, x0+colW[0]/2, ry+row.h/2, 0.5, 0.5)

		// Qubits (wrapped text)
		loadFont(dc, 13)
		dc.SetColor(color.Black)
		ty := ry + pad
		for _, line := range row.lines {
			dc.DrawString(line, x0+colW[0]+pad, ty+lineH*0.75)
			ty += lineH
		}

		// Reason
		loadFont(dc, 14)
		dc.SetColor(color.Black)
		dc.DrawStringAnchored(row.reason, x0+colW[0]+colW[1]+colW[2]/2, ry+row.h/2, 0.5, 0.5)

		ry += row.h
	}

	return dc.SavePNG(outPath)
}

// ---------------------------------------------------------------------------
// Agent Daily Briefing (one file to rule them all)
// ---------------------------------------------------------------------------

func writeAgentBriefing(reports []models.QubitReport, outPath string) error {
	ts := time.Now().Format("2006-01-02 15:04 MST")
	avoid, unstable, best := categorizeQubits(reports)

	// Stats
	var sum float64
	for _, r := range reports {
		sum += r.ReadoutError
	}
	avg := sum / float64(len(reports))

	var b strings.Builder
	fmt.Fprintf(&b, "# Quantum Tide Daily Briefing — IBM Torino\n\n")
	fmt.Fprintf(&b, "**Generated:** %s\n\n", ts)
	fmt.Fprintf(&b, "**Backend:** ibm_torino | **Qubits:** %d | **Mean Error:** %.2f%%\n\n", len(reports), avg*100)
	fmt.Fprintf(&b, "---\n\n")

	// Action list
	fmt.Fprintf(&b, "## Action List (Do Not Fly)\n\n")
	fmt.Fprintf(&b, "### AVOID (%d qubits) — High Persistent Noise (>5%% error)\n\n", len(avoid))
	if len(avoid) > 0 {
		fmt.Fprintf(&b, "Qubits: %s\n\n", intsToString(qubitIndices(avoid)))
		fmt.Fprintf(&b, "| Qubit | Error | T1 (µs) | T2 (µs) |\n|-------|-------|---------|--------|\n")
		sorted := make([]models.QubitReport, len(avoid))
		copy(sorted, avoid)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].ReadoutError > sorted[j].ReadoutError })
		for _, r := range sorted {
			fmt.Fprintf(&b, "| Q%d | %.2f%% | %.1f | %.1f |\n", r.Index, r.ReadoutError*100, r.T1, r.T2)
		}
	} else {
		fmt.Fprintf(&b, "None — all qubits below 5%% error.\n")
	}

	fmt.Fprintf(&b, "\n### UNSTABLE (%d qubits) — Elevated Error (2-5%%)\n\n", len(unstable))
	if len(unstable) > 0 {
		fmt.Fprintf(&b, "Qubits: %s\n\n", intsToString(qubitIndices(unstable)))
	} else {
		fmt.Fprintf(&b, "None.\n")
	}

	fmt.Fprintf(&b, "\n### BEST REGION (%d qubits) — Stable, Low Error (<2%%)\n\n", len(best))
	if len(best) > 0 {
		fmt.Fprintf(&b, "Qubits: %s\n\n", intsToString(qubitIndices(best)))
	} else {
		fmt.Fprintf(&b, "None.\n")
	}

	// Drift section
	fmt.Fprintf(&b, "\n---\n\n## Temporal Drift\n\n")
	days := loadHistoricalData()
	if len(days) < 2 {
		fmt.Fprintf(&b, "*Trend analysis requires 2+ days of data.*\n")
	} else {
		first := days[0]
		last := days[len(days)-1]
		firstErr := make(map[int]float64)
		for _, r := range first.reports {
			firstErr[r.Index] = r.ReadoutError
		}
		var degraded, improved int
		for _, r := range last.reports {
			delta := r.ReadoutError - firstErr[r.Index]
			if delta > 0.005 {
				degraded++
			} else if delta < -0.005 {
				improved++
			}
		}
		fmt.Fprintf(&b, "**Window:** %s → %s (%d days)\n\n", first.date, last.date, len(days))
		fmt.Fprintf(&b, "- Qubits degraded (>0.5pp increase): %d\n", degraded)
		fmt.Fprintf(&b, "- Qubits improved (>0.5pp decrease): %d\n", improved)
	}

	fmt.Fprintf(&b, "\n---\n\n*Files: torino_spatial_map.png, torino_drift_map.png, torino_action_list.png*\n")

	return os.WriteFile(outPath, []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Shared rendering helpers
// ---------------------------------------------------------------------------

func hexGridPositions(n int) [][2]float64 {
	colSp := math.Sqrt(3) * hexSize
	rowSp := 1.5 * hexSize

	type pos struct {
		x, y, dist float64
	}
	var cands []pos

	for row := -15; row <= 15; row++ {
		for col := -15; col <= 15; col++ {
			x := mapCX + float64(col)*colSp
			y := mapCY + float64(row)*rowSp
			if row%2 != 0 {
				x += colSp / 2
			}
			if x < mapLeft+hexSize || x > mapRight-hexSize ||
				y < mapTop+hexSize || y > mapBottom-hexSize {
				continue
			}
			dx := (x - mapCX) * 0.62
			dy := y - mapCY
			cands = append(cands, pos{x, y, math.Hypot(dx, dy)})
		}
	}

	sort.Slice(cands, func(i, j int) bool {
		return cands[i].dist < cands[j].dist
	})
	if len(cands) > n {
		cands = cands[:n]
	}

	sort.Slice(cands, func(i, j int) bool {
		if math.Abs(cands[i].y-cands[j].y) > rowSp*0.4 {
			return cands[i].y < cands[j].y
		}
		return cands[i].x < cands[j].x
	})

	out := make([][2]float64, len(cands))
	for i, c := range cands {
		out[i] = [2]float64{c.x, c.y}
	}
	return out
}

func drawHexFlat(dc *gg.Context, cx, cy, size float64, fill color.RGBA) {
	for i := 0; i < 6; i++ {
		a := math.Pi / 180 * float64(i*60)
		x := cx + size*math.Cos(a)
		y := cy + size*math.Sin(a)
		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
	dc.SetColor(fill)
	dc.FillPreserve()
	dc.SetColor(color.RGBA{R: 15, G: 17, B: 26, A: 255})
	dc.SetLineWidth(2.5)
	dc.Stroke()
}

func errorColorBWR(errVal float64) color.RGBA {
	t := errVal / 0.05
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	var r, g, b float64
	if t < 0.5 {
		s := t / 0.5
		r = 30 + s*225
		g = 70 + s*185
		b = 170 + s*85
	} else {
		s := (t - 0.5) / 0.5
		r = 255 - s*70
		g = 255 - s*230
		b = 255 - s*230
	}

	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

func drawVerticalLegend(dc *gg.Context, lLeft, lRight, lTop, lBottom float64) {
	steps := 300
	stepH := (lBottom - lTop) / float64(steps)
	for i := 0; i < steps; i++ {
		t := 1.0 - float64(i)/float64(steps-1)
		c := errorColorBWR(t * 0.05)
		dc.SetColor(c)
		dc.DrawRectangle(lLeft, lTop+float64(i)*stepH, lRight-lLeft, stepH+1)
		dc.Fill()
	}

	dc.SetColor(color.RGBA{R: 155, G: 160, B: 175, A: 255})
	dc.SetLineWidth(1)
	dc.DrawRectangle(lLeft, lTop, lRight-lLeft, lBottom-lTop)
	dc.Stroke()

	loadFont(dc, 12)
	dc.SetColor(color.RGBA{R: 195, G: 200, B: 215, A: 255})
	for v := 0.5; v <= 5.01; v += 0.5 {
		frac := v / 5.0
		y := lBottom - frac*(lBottom-lTop)
		dc.DrawLine(lRight, y, lRight+5, y)
		dc.Stroke()
		dc.DrawStringAnchored(fmt.Sprintf("%.1f", v), lRight+9, y, 0, 0.5)
	}

	loadFont(dc, 14)
	dc.Push()
	lx := lRight + 55
	ly := (lTop + lBottom) / 2
	dc.RotateAbout(-math.Pi/2, lx, ly)
	dc.DrawStringAnchored("Readout Error (%)", lx, ly, 0.5, 0.5)
	dc.Pop()
}

func loadFont(dc *gg.Context, size float64) {
	for _, p := range []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Helvetica.ttc",
		"/Library/Fonts/Arial.ttf",
	} {
		if err := dc.LoadFontFace(p, size); err == nil {
			return
		}
	}
}
