package agents

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/natalie/tide-bits/models"
)

// Broadcaster generates the HTML daily report and optionally sends it via email.
// Reads output files from disk — call after the Cartographer pipeline completes.
func Broadcaster(providerName string) {
	date := time.Now().Format("2006-01-02")

	summaryPath := filepath.Join("quantum_weather_data", providerName, date+"_summary.json")
	raw, err := os.ReadFile(summaryPath)
	if err != nil {
		log.Printf("Broadcaster: no summary data for %s/%s, skipping", providerName, date)
		return
	}
	var reports []models.QubitReport
	if err := json.Unmarshal(raw, &reports); err != nil {
		log.Printf("Broadcaster: failed to parse summary: %v", err)
		return
	}

	html := buildHTMLReport(providerName, date, reports)
	htmlPath := filepath.Join("output", "daily_report.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		log.Printf("Broadcaster: failed to write HTML report: %v", err)
	} else {
		log.Printf("Broadcaster: HTML report saved → %s", htmlPath)
	}

	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	emailTo := os.Getenv("EMAIL_TO")

	if smtpUser == "" || smtpPass == "" || emailTo == "" {
		log.Println("Broadcaster: no SMTP credentials configured, skipping email")
		log.Println("Broadcaster: set SMTP_USER, SMTP_PASS, EMAIL_TO to enable")
		return
	}

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "smtp.gmail.com"
	}
	smtpPort := os.Getenv("SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "587"
	}

	subject := fmt.Sprintf("Daily Tide Report: %s (%s)", providerName, date)
	if err := sendEmail(smtpHost, smtpPort, smtpUser, smtpPass, emailTo, subject, providerName, date, reports); err != nil {
		log.Printf("Broadcaster: email send failed: %v", err)
	} else {
		log.Printf("Broadcaster: email sent to %s", emailTo)
	}
}

func buildHTMLReport(providerName, date string, reports []models.QubitReport) string {
	avoid, unstable, best := categorizeQubits(reports)

	avoidIdx := qubitIndices(avoid)
	unstableIdx := qubitIndices(unstable)
	bestIdx := qubitIndices(best)

	sort.Ints(avoidIdx)
	sort.Ints(unstableIdx)
	sort.Ints(bestIdx)

	var sum float64
	for _, r := range reports {
		sum += r.ReadoutError
	}
	avg := sum / float64(len(reports))

	spatialB64 := fileToBase64(filepath.Join("output", providerName+"_spatial_map.png"))
	driftB64 := fileToBase64(filepath.Join("output", providerName+"_drift_map.png"))
	actionB64 := fileToBase64(filepath.Join("output", providerName+"_action_list.png"))

	formatList := func(ids []int) string {
		strs := make([]string, len(ids))
		for i, v := range ids {
			strs[i] = fmt.Sprintf("Q%d", v)
		}
		return strings.Join(strs, ", ")
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body { font-family: Arial, Helvetica, sans-serif; background: #f4f4f6; margin: 0; padding: 20px; }
.container { max-width: 900px; margin: 0 auto; background: white; border-radius: 12px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); overflow: hidden; }
.header { background: #1a1a2e; color: white; padding: 24px 30px; }
.header h1 { margin: 0; font-size: 22px; }
.header p { margin: 4px 0 0; color: #a0a0b0; font-size: 14px; }
.body { padding: 24px 30px; }
.stats { display: flex; gap: 20px; margin-bottom: 24px; }
.stat { background: #f8f8fa; border-radius: 8px; padding: 16px; flex: 1; text-align: center; }
.stat .val { font-size: 28px; font-weight: bold; color: #1a1a2e; }
.stat .label { font-size: 12px; color: #888; margin-top: 4px; }
h2 { color: #1a1a2e; border-bottom: 2px solid #eee; padding-bottom: 8px; margin-top: 30px; }
table { border-collapse: collapse; width: 100%; margin: 16px 0; }
td, th { border: 1px solid #ddd; padding: 10px 12px; text-align: left; font-size: 14px; }
th { background: #f0f0f2; font-weight: 600; }
.avoid { color: #cc0000; font-weight: bold; }
.unstable { color: #cc8800; font-weight: bold; }
.best { color: #008800; font-weight: bold; }
img { max-width: 100%; height: auto; border-radius: 6px; margin: 12px 0; }
.footer { background: #f8f8fa; padding: 16px 30px; text-align: center; color: #999; font-size: 12px; }
</style></head><body>
<div class="container">
`)

	fmt.Fprintf(&b, `<div class="header">
<h1>Daily Tide Report: %s</h1>
<p>%s</p>
</div>
<div class="body">
`, providerName, date)

	fmt.Fprintf(&b, `<div class="stats">
<div class="stat"><div class="val">%d</div><div class="label">Total Qubits</div></div>
<div class="stat"><div class="val">%.2f%%</div><div class="label">Mean Error</div></div>
<div class="stat"><div class="val avoid">%d</div><div class="label">Avoid</div></div>
<div class="stat"><div class="val best">%d</div><div class="label">Best Region</div></div>
</div>
`, len(reports), avg*100, len(avoid), len(best))

	if spatialB64 != "" {
		fmt.Fprintf(&b, `<h2>Spatial Heatmap</h2>
<img src="data:image/png;base64,%s" alt="Spatial Heatmap">
`, spatialB64)
	}

	b.WriteString(`<h2>Daily Action List (Do Not Fly)</h2>
<table>
<tr><th>Status</th><th>Qubits</th><th>Reason</th></tr>
`)
	fmt.Fprintf(&b, `<tr><td class="avoid">AVOID</td><td>%s</td><td>High Persistent Noise (&gt;%.0f%% error)</td></tr>
`, formatList(avoidIdx), ThresholdAvoid*100)
	fmt.Fprintf(&b, `<tr><td class="unstable">UNSTABLE</td><td>%s</td><td>Elevated Error (%.0f-%.0f%%)</td></tr>
`, formatList(unstableIdx), ThresholdUnstable*100, ThresholdAvoid*100)
	fmt.Fprintf(&b, `<tr><td class="best">BEST REGION</td><td>%s</td><td>Stable, Low Error (&lt;%.0f%%)</td></tr>
`, formatList(bestIdx), ThresholdUnstable*100)
	b.WriteString("</table>\n")

	if actionB64 != "" {
		fmt.Fprintf(&b, `<img src="data:image/png;base64,%s" alt="Action List">
`, actionB64)
	}

	if driftB64 != "" {
		fmt.Fprintf(&b, `<h2>Temporal Drift Heatmap</h2>
<img src="data:image/png;base64,%s" alt="Temporal Drift">
`, driftB64)
	}

	b.WriteString(`</div>
<div class="footer">Generated by Quantum Tide</div>
</div></body></html>`)

	return b.String()
}

func sendEmail(host, port, user, pass, to, subject, providerName, date string, reports []models.QubitReport) error {
	addr := host + ":" + port
	auth := smtp.PlainAuth("", user, pass, host)

	boundary := fmt.Sprintf("quantum-tide-%d", time.Now().UnixNano())

	images := map[string]string{
		"spatial_map": filepath.Join("output", providerName+"_spatial_map.png"),
		"drift_map":   filepath.Join("output", providerName+"_drift_map.png"),
		"action_list": filepath.Join("output", providerName+"_action_list.png"),
	}

	htmlBody := buildEmailHTML(providerName, date, reports)

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: Quantum Tide <%s>\r\n", user)
	fmt.Fprintf(&msg, "To: %s\r\n", to)
	fmt.Fprintf(&msg, "Subject: %s\r\n", subject)
	fmt.Fprintf(&msg, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/related; boundary=\"%s\"\r\n", boundary)
	fmt.Fprintf(&msg, "\r\n")

	fmt.Fprintf(&msg, "--%s\r\n", boundary)
	fmt.Fprintf(&msg, "Content-Type: text/html; charset=\"utf-8\"\r\n")
	fmt.Fprintf(&msg, "\r\n")
	fmt.Fprintf(&msg, "%s\r\n", htmlBody)

	for cid, path := range images {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fmt.Fprintf(&msg, "--%s\r\n", boundary)
		fmt.Fprintf(&msg, "Content-Type: image/png\r\n")
		fmt.Fprintf(&msg, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&msg, "Content-ID: <%s>\r\n", cid)
		fmt.Fprintf(&msg, "Content-Disposition: inline; filename=\"%s.png\"\r\n", cid)
		fmt.Fprintf(&msg, "\r\n")

		encoded := base64.StdEncoding.EncodeToString(data)
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			fmt.Fprintf(&msg, "%s\r\n", encoded[i:end])
		}
	}

	fmt.Fprintf(&msg, "--%s--\r\n", boundary)

	return smtp.SendMail(addr, auth, user, []string{to}, msg.Bytes())
}

func buildEmailHTML(providerName, date string, reports []models.QubitReport) string {
	avoid, unstable, best := categorizeQubits(reports)

	avoidIdx := qubitIndices(avoid)
	unstableIdx := qubitIndices(unstable)
	bestIdx := qubitIndices(best)

	formatList := func(ids []int) string {
		strs := make([]string, len(ids))
		for i, v := range ids {
			strs[i] = fmt.Sprintf("Q%d", v)
		}
		return strings.Join(strs, ", ")
	}

	var b strings.Builder
	b.WriteString(`<html><body style="font-family:Arial,sans-serif;background:#f4f4f6;padding:20px;">
<div style="max-width:900px;margin:0 auto;background:white;border-radius:12px;overflow:hidden;">
<div style="background:#1a1a2e;color:white;padding:24px 30px;">
`)
	fmt.Fprintf(&b, `<h1 style="margin:0;font-size:22px;">Daily Tide Report: %s (%s)</h1>`, providerName, date)
	b.WriteString(`</div><div style="padding:24px 30px;">`)

	b.WriteString(`<h2>Spatial Heatmap</h2><img src="cid:spatial_map" style="max-width:100%;">`)

	b.WriteString(`<h2>Daily Action List (Do Not Fly)</h2>
<table style="border-collapse:collapse;width:100%;margin:16px 0;">
<tr><th style="border:1px solid #ddd;padding:10px;background:#f0f0f2;">Status</th>
<th style="border:1px solid #ddd;padding:10px;background:#f0f0f2;">Qubits</th>
<th style="border:1px solid #ddd;padding:10px;background:#f0f0f2;">Reason</th></tr>
`)
	fmt.Fprintf(&b, `<tr><td style="border:1px solid #ddd;padding:10px;color:#cc0000;font-weight:bold;">AVOID</td>
<td style="border:1px solid #ddd;padding:10px;font-size:13px;">%s</td>
<td style="border:1px solid #ddd;padding:10px;">High Persistent Noise</td></tr>
`, formatList(avoidIdx))
	fmt.Fprintf(&b, `<tr><td style="border:1px solid #ddd;padding:10px;color:#cc8800;font-weight:bold;">UNSTABLE</td>
<td style="border:1px solid #ddd;padding:10px;font-size:13px;">%s</td>
<td style="border:1px solid #ddd;padding:10px;">Elevated Error (%.0f-%.0f%%)</td></tr>
`, formatList(unstableIdx), ThresholdUnstable*100, ThresholdAvoid*100)
	fmt.Fprintf(&b, `<tr><td style="border:1px solid #ddd;padding:10px;color:#008800;font-weight:bold;">BEST REGION</td>
<td style="border:1px solid #ddd;padding:10px;font-size:13px;">%s</td>
<td style="border:1px solid #ddd;padding:10px;">Stable, Low Error</td></tr>
`, formatList(bestIdx))
	b.WriteString(`</table>`)

	b.WriteString(`<h2>Temporal Drift</h2><img src="cid:drift_map" style="max-width:100%;">`)

	b.WriteString(`</div>
<div style="background:#f8f8fa;padding:16px 30px;text-align:center;color:#999;font-size:12px;">Generated by Quantum Tide</div>
</div></body></html>`)

	return b.String()
}

func fileToBase64(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}
