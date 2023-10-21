package checks

import (
	"encoding/json"
	"log"
	"os"
	"sync"
)

var mutex sync.Mutex
var storageLocation = "data/checks.json"

func SaveChecksData(checksData Data) error {
	mutex.Lock()
	defer mutex.Unlock()

	file, err := os.Create("data/checks.json")
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)

	if err := encoder.Encode(checksData); err != nil {
		return err
	}

	return nil
}

func ReadChecksData() Data {
	mutex.Lock()
	defer mutex.Unlock()

	file, err := os.Open(storageLocation)
	if err != nil {
		log.Fatalf("[ERROR] failed open checks.json: %v", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var checksData Data

	if err := decoder.Decode(&checksData); err != nil {
		log.Fatalf("[ERROR] failed decode checks.json: %v", err)
	}

	return checksData
}

func InitStorage() {
	if _, err := os.Stat(storageLocation); os.IsNotExist(err) {
		err = os.MkdirAll("data", os.ModePerm)

		file, err := os.Create(storageLocation)
		if err != nil {
			log.Fatalf("[ERROR] failed create checks.json: %v", err)
		}

		_, err = file.WriteString("{}")
		if err != nil {
			log.Fatalf("[ERROR] failed write {} to checks.json: %v", err)
		}

		defer file.Close()
	}
}
