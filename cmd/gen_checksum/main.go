package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gradechecker/pkg/integrity"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current working directory: %v\n", err)
		os.Exit(1)
	}

	hash, err := integrity.CalculateProjectChecksum(cwd)
	if err != nil {
		fmt.Printf("Error calculating checksum: %v\n", err)
		os.Exit(1)
	}

	data := integrity.ChecksumData{
		Hash: hash,
	}

	file, err := os.Create(integrity.ChecksumFileName)
	if err != nil {
		fmt.Printf("Error creating checksum file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Printf("Error encoding checksum data: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s with hash: %s\n", integrity.ChecksumFileName, hash)
}
