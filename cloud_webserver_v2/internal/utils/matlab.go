package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

func CreateMatlabFile(filePath string, allSignalData *map[string]map[string]interface{}) error {
	// Serialize to JSON
	jsonData, err := json.Marshal(*allSignalData)
	if err != nil {
		return fmt.Errorf("error serializing data: %s", err)
	}

	cmd := exec.Command("python3", "internal/utils/matlab_file_creator.py", filePath)
	cmd.Stdin = bytes.NewReader([]byte(jsonData))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute Python script: %s, %s", output, err)
	}

	log.Print(string(output))

	return nil
}
