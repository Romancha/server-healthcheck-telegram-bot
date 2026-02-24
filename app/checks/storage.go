package checks

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var mutex sync.Mutex
var storageLocation = "data/checks.json"

// SetStorageLocation overrides the default storage file path (used in tests).
// Returns the previous location so callers can restore it.
func SetStorageLocation(path string) string {
	mutex.Lock()
	defer mutex.Unlock()
	prev := storageLocation
	storageLocation = path
	return prev
}

// SaveChecksData persists the checks data to the JSON storage file.
func SaveChecksData(checksData Data) error {
	mutex.Lock()
	defer mutex.Unlock()

	file, err := os.Create(storageLocation)
	if err != nil {
		return fmt.Errorf("create storage file: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(checksData); err != nil {
		return fmt.Errorf("encode checks data: %w", err)
	}

	return nil
}

// ReadChecksData reads the checks data from the JSON storage file.
func ReadChecksData() Data {
	mutex.Lock()
	defer mutex.Unlock()

	file, err := os.Open(storageLocation)
	if err != nil {
		log.Printf("[ERROR] failed open checks.json: %v", err)
		return Data{}
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var checksData Data

	if err := decoder.Decode(&checksData); err != nil {
		log.Printf("[ERROR] failed decode checks.json: %v", err)
		return Data{}
	}

	return checksData
}

// InitStorage creates the storage file and directory if they don't exist.
func InitStorage() {
	mutex.Lock()
	loc := storageLocation
	mutex.Unlock()

	if _, err := os.Stat(loc); os.IsNotExist(err) {
		if mkdirErr := os.MkdirAll(filepath.Dir(loc), 0o750); mkdirErr != nil {
			log.Fatalf("[ERROR] failed create storage directory: %v", mkdirErr)
		}

		file, createErr := os.Create(loc)
		if createErr != nil {
			log.Fatalf("[ERROR] failed create checks.json: %v", createErr)
		}

		_, writeErr := file.WriteString("{}")
		_ = file.Close()
		if writeErr != nil {
			log.Fatalf("[ERROR] failed write {} to checks.json: %v", writeErr)
		}
	}
}
