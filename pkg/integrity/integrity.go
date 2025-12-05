package integrity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	ChecksumFileName  = "checksum.json"
	GitHubChecksumURL = "https://raw.githubusercontent.com/Raindancer118/NAK-Grade-Checker/refs/heads/main/checksum.json"
)

type ChecksumData struct {
	Hash string `json:"hash"`
}

// CalculateProjectChecksum calculates the SHA256 hash of the project files.
// It includes specific directories and files while excluding others.
func CalculateProjectChecksum(rootPath string) (string, error) {
	var files []string

	// Define directories and files to include
	includeDirs := []string{"cmd", "internal", "pkg", "src"}
	includeFiles := []string{"go.mod", "go.sum", "package.json", "astro.config.mjs"}

	// Walk through directories
	for _, dir := range includeDirs {
		err := filepath.Walk(filepath.Join(rootPath, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			// It's okay if a directory doesn't exist (e.g. pkg might be new)
			if !os.IsNotExist(err) {
				return "", err
			}
		}
	}

	// Add individual files
	for _, file := range includeFiles {
		path := filepath.Join(rootPath, file)
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}

	// Sort files to ensure consistent order
	sort.Strings(files)

	hasher := sha256.New()

	for _, file := range files {
		// Open file
		f, err := os.Open(file)
		if err != nil {
			return "", err
		}
		defer f.Close()

		// Hash file path (relative to root) to detect file moves/renames
		relPath, err := filepath.Rel(rootPath, file)
		if err != nil {
			return "", err
		}
		// Normalize path separators for cross-platform consistency
		relPath = filepath.ToSlash(relPath)
		hasher.Write([]byte(relPath))

		// Hash file content
		if _, err := io.Copy(hasher, f); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// CheckIntegrity compares the local checksum with the remote one.
func CheckIntegrity(rootPath string) (bool, string, error) {
	localHash, err := CalculateProjectChecksum(rootPath)
	if err != nil {
		return false, "", fmt.Errorf("failed to calculate local checksum: %w", err)
	}

	url := fmt.Sprintf("%s?t=%d", GitHubChecksumURL, time.Now().Unix())
	resp, err := http.Get(url)
	if err != nil {
		return false, localHash, fmt.Errorf("failed to fetch remote checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, localHash, fmt.Errorf("failed to fetch remote checksum: status %d", resp.StatusCode)
	}

	var remoteData ChecksumData
	if err := json.NewDecoder(resp.Body).Decode(&remoteData); err != nil {
		return false, localHash, fmt.Errorf("failed to decode remote checksum: %w", err)
	}

	return localHash == remoteData.Hash, localHash, nil
}

// UpdateChecksumFile calculates the project checksum and writes it to checksum.json
func UpdateChecksumFile(rootPath string) (string, error) {
	hash, err := CalculateProjectChecksum(rootPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	data := ChecksumData{
		Hash: hash,
	}

	file, err := os.Create(filepath.Join(rootPath, ChecksumFileName))
	if err != nil {
		return "", fmt.Errorf("failed to create checksum file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", fmt.Errorf("failed to encode checksum data: %w", err)
	}

	return hash, nil
}
