package checks

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// setupTestStorage redirects storageLocation to a temp dir and initializes it.
func setupTestStorage(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	original := SetStorageLocation(filepath.Join(tmpDir, "checks.json"))
	t.Cleanup(func() { SetStorageLocation(original) })
	InitStorage()
}

func TestInitStorage(t *testing.T) {
	// Use manual setup here because we need to test InitStorage itself
	tmpDir := t.TempDir()
	original := SetStorageLocation(filepath.Join(tmpDir, "checks.json"))
	t.Cleanup(func() { SetStorageLocation(original) })

	// File should not exist yet
	if _, err := os.Stat(storageLocation); !os.IsNotExist(err) {
		t.Fatal("expected storage file to not exist before InitStorage")
	}

	InitStorage()

	// File should exist now
	info, err := os.Stat(storageLocation)
	if err != nil {
		t.Fatalf("expected storage file to exist after InitStorage: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected storage file to have content after InitStorage")
	}
}

func TestInitStorageIdempotent(t *testing.T) {
	setupTestStorage(t)

	// Write some data
	data := Data{
		HealthChecks: map[string]ServerCheck{
			"test": {Name: "test", URL: "https://example.com"},
		},
	}
	err := SaveChecksData(data)
	if err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	// Call InitStorage again — should NOT overwrite existing file
	InitStorage()

	got := ReadChecksData()
	if _, ok := got.HealthChecks["test"]; !ok {
		t.Fatal("InitStorage overwrote existing data")
	}
}

func TestSaveAndReadChecksData(t *testing.T) {
	setupTestStorage(t)

	now := time.Now().Truncate(time.Second)

	data := Data{
		HealthChecks: map[string]ServerCheck{
			"server1": {
				Name:             "server1",
				URL:              "https://example.com",
				IsOk:             true,
				LastSuccess:      now,
				Availability:     99.5,
				TotalChecks:      200,
				SuccessfulChecks: 199,
				LastResponseTime: 42,
			},
			"server2": {
				Name:            "server2",
				URL:             "https://test.com",
				IsOk:            false,
				LastFailure:     now,
				ExpectedContent: "OK",
			},
		},
	}

	err := SaveChecksData(data)
	if err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	got := ReadChecksData()

	if len(got.HealthChecks) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(got.HealthChecks))
	}

	s1 := got.HealthChecks["server1"]
	if s1.URL != "https://example.com" {
		t.Errorf("server1.Url = %q, want %q", s1.URL, "https://example.com")
	}
	if !s1.IsOk {
		t.Error("server1.IsOk = false, want true")
	}
	if s1.Availability != 99.5 {
		t.Errorf("server1.Availability = %f, want 99.5", s1.Availability)
	}
	if s1.TotalChecks != 200 {
		t.Errorf("server1.TotalChecks = %d, want 200", s1.TotalChecks)
	}
	if s1.LastResponseTime != 42 {
		t.Errorf("server1.LastResponseTime = %d, want 42", s1.LastResponseTime)
	}

	s2 := got.HealthChecks["server2"]
	if s2.ExpectedContent != "OK" {
		t.Errorf("server2.ExpectedContent = %q, want %q", s2.ExpectedContent, "OK")
	}
}

func TestSaveAndReadEmptyData(t *testing.T) {
	setupTestStorage(t)

	data := Data{
		HealthChecks: make(map[string]ServerCheck),
	}

	err := SaveChecksData(data)
	if err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	got := ReadChecksData()
	if len(got.HealthChecks) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(got.HealthChecks))
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	setupTestStorage(t)

	// Seed initial data
	data := Data{
		HealthChecks: map[string]ServerCheck{
			"s1": {Name: "s1", URL: "https://example.com", IsOk: true},
		},
	}
	if err := SaveChecksData(data); err != nil {
		t.Fatalf("SaveChecksData: %v", err)
	}

	var wg sync.WaitGroup
	const goroutines = 10

	// Use a barrier so all goroutines start at the same time
	start := make(chan struct{})

	// Mix reads and writes in a single loop for true concurrency
	for range goroutines {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			got := ReadChecksData()
			if got.HealthChecks == nil {
				t.Error("ReadChecksData returned nil HealthChecks")
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			err := SaveChecksData(data)
			if err != nil {
				t.Errorf("SaveChecksData: %v", err)
			}
		}()
	}

	close(start) // release all goroutines at once
	wg.Wait()
}
