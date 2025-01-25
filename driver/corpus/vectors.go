package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// GitHub API base URL
const githubAPIBaseURL = "https://api.github.com"

// Artifact name to download
const artifactName = "Mainnet Test Configuration"

// Function to download the latest artifact from a GitHub Actions workflow
func downloadLatestArtifact(owner, repo, workflowFileName, outputDir string) error {
	// Get GITHUB_TOKEN from the environment
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}

	// Create an HTTP client
	client := &http.Client{}

	// Get the latest workflow runs
	workflowRunsURL := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=1&status=success", githubAPIBaseURL, owner, repo, workflowFileName)
	req, err := http.NewRequest("GET", workflowRunsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch workflow runs: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch workflow runs: %s", resp.Status)
	}

	// Parse the response
	var workflowRuns struct {
		WorkflowRuns []struct {
			ID int64 `json:"id"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&workflowRuns); err != nil {
		return fmt.Errorf("failed to decode workflow runs: %w", err)
	}

	if len(workflowRuns.WorkflowRuns) == 0 {
		return fmt.Errorf("no successful workflow runs found")
	}

	// Get the latest run ID
	latestRunID := workflowRuns.WorkflowRuns[0].ID

	// Get the artifacts for the latest run
	artifactsURL := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%d/artifacts", githubAPIBaseURL, owner, repo, latestRunID)
	req, err = http.NewRequest("GET", artifactsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for artifacts: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)

	// Send the request
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch artifacts: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch artifacts: %s", resp.Status)
	}

	// Parse the artifacts response
	var artifacts struct {
		Artifacts []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"artifacts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&artifacts); err != nil {
		return fmt.Errorf("failed to decode artifacts: %w", err)
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
		return fmt.Errorf("artifact %q not found in the latest run", artifactName)
	}

	// Download the artifact
	artifactDownloadURL := fmt.Sprintf("%s/repos/%s/%s/actions/artifacts/%d/zip", githubAPIBaseURL, owner, repo, artifactID)
	req, err = http.NewRequest("GET", artifactDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request to download artifact: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+githubToken)

	// Send the request
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download artifact: %s", resp.Status)
	}

	// Save the artifact to a file
	outputFilePath := fmt.Sprintf("%s/%s.zip", outputDir, artifactName)
	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	_, err = io.Copy(outputFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save artifact: %w", err)
	}

	fmt.Printf("Artifact %q downloaded successfully to %s\n", artifactName, outputFilePath)
	return nil
}

func DownloadTestVectors() error {
	owner := "ethereum"
	repo := "consensus-specs"
	workflowFileName := "generate_vectors.yml"
	outputDir := "./artifacts"

	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := downloadLatestArtifact(owner, repo, workflowFileName, outputDir); err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}

	return nil
}
