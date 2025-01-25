package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// GitHub API base URL
const githubAPIBaseURL = "https://api.github.com"

// Artifact name to download
const artifactName = "Mainnet Test Configuration"

// Function to download the latest artifact from a GitHub Actions workflow
func downloadLatestArtifact(owner, repo, workflowFileName, outputDir string) (string, error) {
	// Get GITHUB_TOKEN from the environment
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return "", fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}

	// Create an HTTP client
	client := &http.Client{}

	// Get the latest workflow runs
	workflowRunsURL := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=1&status=success", githubAPIBaseURL, owner, repo, workflowFileName)
	req, err := http.NewRequest("GET", workflowRunsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch workflow runs: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch workflow runs: %s", resp.Status)
	}

	// Parse the response
	var workflowRuns struct {
		WorkflowRuns []struct {
			ID int64 `json:"id"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&workflowRuns); err != nil {
		return "", fmt.Errorf("failed to decode workflow runs: %w", err)
	}

	if len(workflowRuns.WorkflowRuns) == 0 {
		return "", fmt.Errorf("no successful workflow runs found")
	}

	// Get the latest run ID
	latestRunID := workflowRuns.WorkflowRuns[0].ID

	// Get the artifacts for the latest run
	artifactsURL := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/artifacts", githubAPIBaseURL, owner, repo, latestRunID)
	req, err = http.NewRequest("GET", artifactsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for artifacts: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)

	// Send the request
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch artifacts: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch artifacts: %s", resp.Status)
	}

	// Parse the artifacts response
	var artifacts struct {
		Artifacts []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&artifacts); err != nil {
		return "", fmt.Errorf("failed to decode artifacts: %w", err)
	}

	// Find the artifact by name
	var artifactID int64
	for _, artifact := range artifacts.Artifacts {
		if strings.EqualFold(artifact.Name, artifactName) {
			artifactID = artifact.ID
			break
		}
	}

	if artifactID == 0 {
		return "", fmt.Errorf("artifact %q not found in the latest run", artifactName)
	}

	// Download the artifact
	artifactDownloadURL := fmt.Sprintf("%s/repos/%s/%s/actions/artifacts/%d/zip", githubAPIBaseURL, owner, repo, artifactID)
	req, err = http.NewRequest("GET", artifactDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request to download artifact: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)

	// Send the request
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download artifact: %w", err)
	}
	defer resp.Body.Close()

	// Save the artifact as a ZIP file
	artifactZipPath := filepath.Join(outputDir, artifactName+".zip")
	outputFile, err := os.Create(artifactZipPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save artifact: %w", err)
	}

	return artifactZipPath, nil
}

// Unzip the file and extract contents to a directory
func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		filePath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			// Create directories
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		} else {
			// Create the file
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return fmt.Errorf("failed to create parent directories: %w", err)
			}

			outFile, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("failed to open file in zip: %w", err)
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
		}
	}

	return nil
}

// UntarGz extracts a .tar.gz file to the specified destination directory
func untarGz(src, dest string) error {
	// Open the .tar.gz file
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open .tar.gz file: %w", err)
	}
	defer file.Close()

	// Create a gzip reader
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Create a tar reader
	tarReader := tar.NewReader(gzipReader)

	// Extract files from the tar archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			// End of archive
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar file: %w", err)
		}

		// Construct the full file path
		filePath := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directories as needed
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Create the file
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				return fmt.Errorf("failed to create parent directories: %w", err)
			}

			outFile, err := os.Create(filePath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			// Write the file's contents
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()
		default:
			// Skip other file types (e.g., symlinks)
			fmt.Printf("Skipping unknown type: %s in %s\n", string(header.Typeflag), header.Name)
		}
	}

	return nil
}

func DownloadTests() error {
	owner := "ethereum"
	repo := "consensus-specs"
	workflowFileName := "generate_vectors.yml"
	outputDir := "./downloads"

	// Make the downloads directory
	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Download the latest test vectors
	fmt.Printf("Downloading latest test vectors...\n")
	artifactZipPath, err := downloadLatestArtifact(owner, repo, workflowFileName, outputDir)
	if err != nil {
		return fmt.Errorf("error downloading artifact: %w", err)
	}
	fmt.Printf("Downloaded artifact: %v\n", artifactZipPath)

	// Unzip the artifact
	fmt.Printf("Unzipping to %v...\n", outputDir)
	unzippedDir := filepath.Join(outputDir, "unzipped")
	if err := unzip(artifactZipPath, unzippedDir); err != nil {
		return fmt.Errorf("error unzipping artifact: %w", err)
	}

	// Untar the unzipped content (assuming .tar files are inside the zip)
	files, err := os.ReadDir(unzippedDir)
	if err != nil {
		return fmt.Errorf("error reading unzipped directory: %w", err)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tar.gz") {
			tarPath := filepath.Join(unzippedDir, file.Name())
			fmt.Printf("Untarring to %v...\n", outputDir)
			if err := untarGz(tarPath, outputDir); err != nil {
				return fmt.Errorf("error untarring file: %w", err)
			}
		}
	}

	return nil
}
