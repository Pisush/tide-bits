package agents

import (
	"errors"
	"testing"

	"github.com/natalie/tide-bits/models"
)

// fakeProvider is a minimal Provider implementation for testing.
type fakeProvider struct {
	name    string
	reports []models.QubitReport
	raw     []byte
	err     error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Fetch() ([]byte, []models.QubitReport, error) {
	return f.raw, f.reports, f.err
}

// ---------------------------------------------------------------------------
// Scout
// ---------------------------------------------------------------------------

func TestScout_sendsDataDownstream(t *testing.T) {
	reports := []models.QubitReport{
		{Index: 0, ReadoutError: 0.01, T1: 150, T2: 80},
		{Index: 1, ReadoutError: 0.03, T1: 120, T2: 60},
	}
	raw := []byte(`{"backend_name":"test"}`)

	p := &fakeProvider{name: "test_backend", reports: reports, raw: raw}
	out := make(chan models.RawCalibrationData, 1)

	Scout(p, out)

	data, ok := <-out
	if !ok {
		t.Fatal("channel was closed without sending data")
	}

	if data.ProviderName != "test_backend" {
		t.Errorf("ProviderName = %q, want %q", data.ProviderName, "test_backend")
	}
	if len(data.Reports) != 2 {
		t.Errorf("expected 2 reports, got %d", len(data.Reports))
	}
	if string(data.RawJSON) != string(raw) {
		t.Errorf("RawJSON mismatch")
	}
	if data.FetchedAt.IsZero() {
		t.Error("FetchedAt should be set")
	}
}

func TestScout_closesChannelAfterSend(t *testing.T) {
	p := &fakeProvider{
		name:    "test_backend",
		reports: []models.QubitReport{{Index: 0, ReadoutError: 0.01}},
		raw:     []byte(`{}`),
	}
	out := make(chan models.RawCalibrationData, 1)

	Scout(p, out)

	// Drain the value.
	<-out

	// Channel should now be closed.
	_, open := <-out
	if open {
		t.Error("channel should be closed after Scout completes")
	}
}

func TestScout_closesChannelOnFetchError(t *testing.T) {
	p := &fakeProvider{
		name: "failing_backend",
		err:  errors.New("network unreachable"),
	}
	out := make(chan models.RawCalibrationData, 1)

	Scout(p, out)

	// Channel should be closed without sending anything.
	data, open := <-out
	if open {
		t.Errorf("channel should be closed on fetch error, got data: %+v", data)
	}
}

func TestScout_reportsHaveCorrectIndices(t *testing.T) {
	reports := make([]models.QubitReport, 5)
	for i := range reports {
		reports[i] = models.QubitReport{Index: i, ReadoutError: float64(i) * 0.01}
	}

	p := &fakeProvider{name: "test", reports: reports, raw: []byte(`{}`)}
	out := make(chan models.RawCalibrationData, 1)

	Scout(p, out)

	data := <-out
	for i, r := range data.Reports {
		if r.Index != i {
			t.Errorf("report %d has Index %d, want %d", i, r.Index, i)
		}
	}
}
