package ibm

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/natalie/tide-bits/models"
)

type iamTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// nduv is the raw IBM calibration parameter shape.
type nduv struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// backendProperties is the top-level IBM Quantum properties API response.
type backendProperties struct {
	BackendName    string    `json:"backend_name"`
	BackendVersion string    `json:"backend_version"`
	Qubits         [][]nduv  `json:"qubits"`
}

// Provider fetches calibration data from IBM Quantum Cloud.
type Provider struct {
	apiKey string
	crn    string
}

func New(apiKey, crn string) *Provider {
	return &Provider{apiKey: apiKey, crn: crn}
}

func (p *Provider) Name() string {
	return "ibm_torino"
}

func (p *Provider) Fetch() ([]byte, []models.QubitReport, error) {
	log.Println("IBM: authenticating with IBM Cloud IAM...")
	token, err := p.authenticate()
	if err != nil {
		return nil, nil, fmt.Errorf("authentication failed: %w", err)
	}
	log.Println("IBM: authenticated successfully")

	log.Println("IBM: fetching ibm_torino calibration data...")
	rawJSON, props, err := p.fetchProperties(token)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch failed: %w", err)
	}
	log.Printf("IBM: received data for %s (version %s), %d qubits",
		props.BackendName, props.BackendVersion, len(props.Qubits))

	if len(props.Qubits) == 0 {
		return nil, nil, fmt.Errorf("received 0 qubits from IBM API")
	}

	reports := extractQubitReports(props)
	log.Printf("IBM: extracted %d qubit reports", len(reports))
	return rawJSON, reports, nil
}

func (p *Provider) authenticate() (string, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ibm:params:oauth:grant-type:apikey")
	form.Set("apikey", p.apiKey)

	var tokenResp iamTokenResponse
	err := doWithRetry(func() error {
		resp, err := http.Post(
			"https://iam.cloud.ibm.com/identity/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		if err != nil {
			return fmt.Errorf("IAM request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("IAM returned %d: %s", resp.StatusCode, string(body))
		}

		return json.NewDecoder(resp.Body).Decode(&tokenResp)
	})
	if err != nil {
		return "", err
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("IAM returned empty access token")
	}
	return tokenResp.AccessToken, nil
}

func (p *Provider) fetchProperties(token string) ([]byte, *backendProperties, error) {
	const endpoint = "https://quantum.cloud.ibm.com/api/v1/backends/ibm_torino/properties"

	var rawJSON []byte
	var props backendProperties

	err := doWithRetry(func() error {
		req, err := http.NewRequest("GET", endpoint, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Service-CRN", p.crn)
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("properties request failed: %w", err)
		}
		defer resp.Body.Close()

		rawJSON, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("properties API returned %d: %s", resp.StatusCode, truncate(string(rawJSON), 200))
		}

		return json.Unmarshal(rawJSON, &props)
	})

	return rawJSON, &props, err
}

func extractQubitReports(props *backendProperties) []models.QubitReport {
	reports := make([]models.QubitReport, len(props.Qubits))
	for i, params := range props.Qubits {
		r := models.QubitReport{Index: i}
		for _, p := range params {
			switch p.Name {
			case "readout_error", "readout_assignment_error":
				r.ReadoutError = p.Value
			case "T1":
				r.T1 = p.Value
			case "T2":
				r.T2 = p.Value
			}
		}
		reports[i] = r
	}
	return reports
}

func doWithRetry(fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 2 * time.Second
			log.Printf("IBM: retrying in %v (attempt %d/3)...", delay, attempt+1)
			time.Sleep(delay)
		}
		if err := fn(); err != nil {
			lastErr = err
			log.Printf("IBM: attempt %d failed: %v", attempt+1, err)
			continue
		}
		return nil
	}
	return fmt.Errorf("all 3 attempts failed, last error: %w", lastErr)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
