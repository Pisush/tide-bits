package ibm

import (
	"errors"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// extractQubitReports
// ---------------------------------------------------------------------------

func TestExtractQubitReports_basic(t *testing.T) {
	props := &backendProperties{
		Qubits: [][]nduv{
			{
				{Name: "readout_error", Value: 0.012},
				{Name: "T1", Value: 150.5},
				{Name: "T2", Value: 80.3},
			},
			{
				{Name: "readout_error", Value: 0.034},
				{Name: "T1", Value: 100.0},
				{Name: "T2", Value: 60.0},
			},
		},
	}

	reports := extractQubitReports(props)

	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}

	if reports[0].Index != 0 {
		t.Errorf("report[0].Index = %d, want 0", reports[0].Index)
	}
	if reports[0].ReadoutError != 0.012 {
		t.Errorf("report[0].ReadoutError = %v, want 0.012", reports[0].ReadoutError)
	}
	if reports[0].T1 != 150.5 {
		t.Errorf("report[0].T1 = %v, want 150.5", reports[0].T1)
	}
	if reports[0].T2 != 80.3 {
		t.Errorf("report[0].T2 = %v, want 80.3", reports[0].T2)
	}

	if reports[1].Index != 1 {
		t.Errorf("report[1].Index = %d, want 1", reports[1].Index)
	}
	if reports[1].ReadoutError != 0.034 {
		t.Errorf("report[1].ReadoutError = %v, want 0.034", reports[1].ReadoutError)
	}
}

func TestExtractQubitReports_acceptsReadoutAssignmentError(t *testing.T) {
	props := &backendProperties{
		Qubits: [][]nduv{
			{
				{Name: "readout_assignment_error", Value: 0.055},
				{Name: "T1", Value: 120.0},
				{Name: "T2", Value: 70.0},
			},
		},
	}

	reports := extractQubitReports(props)
	if reports[0].ReadoutError != 0.055 {
		t.Errorf("readout_assignment_error should populate ReadoutError; got %v", reports[0].ReadoutError)
	}
}

func TestExtractQubitReports_missingParams(t *testing.T) {
	// A qubit with no matching parameters should produce zero values.
	props := &backendProperties{
		Qubits: [][]nduv{
			{
				{Name: "gate_error", Value: 0.001}, // irrelevant param
			},
		},
	}

	reports := extractQubitReports(props)
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].ReadoutError != 0 || reports[0].T1 != 0 || reports[0].T2 != 0 {
		t.Errorf("expected zero values for unknown params, got %+v", reports[0])
	}
}

func TestExtractQubitReports_empty(t *testing.T) {
	props := &backendProperties{}
	reports := extractQubitReports(props)
	if len(reports) != 0 {
		t.Errorf("expected empty reports for empty qubits, got %d", len(reports))
	}
}

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate_shortString(t *testing.T) {
	s := truncate("hello", 10)
	if s != "hello" {
		t.Errorf("expected %q, got %q", "hello", s)
	}
}

func TestTruncate_exactLength(t *testing.T) {
	s := truncate("hello", 5)
	if s != "hello" {
		t.Errorf("expected %q, got %q", "hello", s)
	}
}

func TestTruncate_longString(t *testing.T) {
	s := truncate("hello world", 5)
	if s != "hello..." {
		t.Errorf("expected %q, got %q", "hello...", s)
	}
}

func TestTruncate_empty(t *testing.T) {
	s := truncate("", 5)
	if s != "" {
		t.Errorf("expected empty string, got %q", s)
	}
}

// ---------------------------------------------------------------------------
// doWithRetry
// ---------------------------------------------------------------------------

func TestDoWithRetry_successOnFirstAttempt(t *testing.T) {
	calls := 0
	err := doWithRetry(func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestDoWithRetry_allFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: involves retry sleep delays")
	}
	sentinel := errors.New("permanent failure")
	calls := 0
	err := doWithRetry(func() error {
		calls++
		return sentinel
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error in chain, got %v", err)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestDoWithRetry_successOnSecondAttempt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: involves retry sleep delay")
	}
	calls := 0
	err := doWithRetry(func() error {
		calls++
		if calls < 2 {
			return fmt.Errorf("transient error")
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// Provider.Name
// ---------------------------------------------------------------------------

func TestName_returnsIBMTorino(t *testing.T) {
	p := New("key", "crn")
	if p.Name() != "ibm_torino" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ibm_torino")
	}
}
