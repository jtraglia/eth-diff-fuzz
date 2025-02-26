package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// UntarGz extracts a .tar.gz file to the specified destination directory.
func untarGz(src, dest string) error {
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open .tar.gz file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			// End of archive
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar file: %w", err)
		}

		targetPath := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), os.ModePerm); err != nil {
				return fmt.Errorf("failed to create parent directories: %w", err)
			}
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			outFile.Close()
		default:
			// Skip other file types (symlinks, etc.)
			fmt.Printf("Skipping unknown type: %s in %s\n", string(header.Typeflag), header.Name)
		}
	}

	return nil
}

// DownloadTests fetches the latest release (index 0 in the array) from
// ethereum/consensus-spec-tests, downloads the three .tar.gz assets,
// and untars them into ./downloads.
func DownloadTests() error {
	owner := "ethereum"
	repo := "consensus-spec-tests"
	outputDir := "./downloads"

	// Assets we want to download from the release
	wantedAssets := []string{"general.tar.gz", "mainnet.tar.gz", "minimal.tar.gz"}

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Fetch all releases
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return fmt.Errorf("failed to decode release JSON: %w", err)
	}
	if len(releases) == 0 {
		return fmt.Errorf("no releases found for %s/%s", owner, repo)
	}

	// Here we simply take the first release returned (typically the newest).
	// If you need to skip pre-releases or drafts, you can iterate and check
	// r.Prerelease / r.Draft.
	latest := releases[0]
	fmt.Printf("Release: %s\n", latest.TagName)

	// Download each wanted asset, then untar it
	for _, assetName := range wantedAssets {
		// Find the asset in this release
		var downloadURL string
		for _, asset := range latest.Assets {
			if asset.Name == assetName {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}
		if downloadURL == "" {
			return fmt.Errorf("could not find asset %q in release %s", assetName, latest.TagName)
		}

		// Download to ./downloads/assetName
		destPath := filepath.Join(outputDir, assetName)
		fmt.Printf("Downloading: %s\n", assetName)
		if err := downloadFile(downloadURL, destPath); err != nil {
			return fmt.Errorf("failed to download asset %q: %w", assetName, err)
		}

		// Untar the .tar.gz file into ./downloads
		if strings.HasSuffix(assetName, ".tar.gz") {
			if err := untarGz(destPath, outputDir); err != nil {
				return fmt.Errorf("failed to untar file %q: %w", assetName, err)
			}
		}
	}

	return nil
}

// downloadFile is a small helper that fetches a file and saves it to `dest`.
func downloadFile(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	outFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	return err
}
