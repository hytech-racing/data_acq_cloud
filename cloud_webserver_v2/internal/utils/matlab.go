package utils

import (
	"bytes"
	"encoding/json"
	"log"
	"os/exec"
)

func CreateMatlabFile(allSignalData *map[string]map[string][]float64) {
	// Serialize to JSON
	jsonData, err := json.Marshal(*allSignalData)
	if err != nil {
		log.Fatalf("Error serializing data: %s", err)
	}

	cmd := exec.Command("python3", "internal/utils/matlab_file_creator.py")
	cmd.Stdin = bytes.NewReader([]byte(jsonData))

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to execute Python script: %s", err)
	}

	log.Print(string(output))
}
