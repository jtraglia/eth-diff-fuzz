package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/golang/snappy"
)

func main() {
	// Get list of forks
	forks, err := listDirectories("tests/mainnet/")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Populate pre states
	for _, fork := range forks {
		err = populateCorpus(fork, "BeaconState", ".*/(pre|post).ssz_snappy")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// Populate static objects
	for _, fork := range forks {
		objects, err := listDirectories("tests/mainnet/" + fork + "/ssz_static/")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		for _, object := range objects {
			err = populateCorpus(fork, object, "/"+object+"/.*.ssz_snappy")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	}
}

func listDirectories(path string) ([]string, error) {
	// Read the directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Filter directories
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, entry.Name())
		}
	}

	return directories, nil
}

func populateCorpus(fork string, object string, regexPattern string) error {
	var files []string

	// Compile the regex
	pattern, err := regexp.Compile(regexPattern)
	if err != nil {
		return fmt.Errorf("failed to compile regex: %w", err)
	}

	// Walk through the directory tree
	err = filepath.WalkDir("tests/mainnet/"+fork, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check if the file path matches the regex
		if !d.IsDir() && pattern.MatchString(path) {
			outputFilePath, err := decompress(path, fork, object)
			if err != nil {
				return err
			}
			files = append(files, outputFilePath)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	// Print status
	if len(files) == 0 {
		fmt.Printf("No files %v.%v (pattern: %v)",
			fork, object, regexPattern)
	} else {
		fmt.Printf("Populated %v.%v (count: %v) (pattern: %v)\n",
			fork, object, len(files), regexPattern)
	}

	return nil
}

func decompress(inputPath string, fork string, object string) (string, error) {
	// Open the compressed file
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return "", fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	// Read the entire file into memory (raw Snappy files are usually small)
	compressedData, err := io.ReadAll(inputFile)
	if err != nil {
		return "", fmt.Errorf("failed to read input file: %w", err)
	}

	// Decode the Snappy-compressed data
	decompressedData, err := snappy.Decode(nil, compressedData)
	if err != nil {
		return "", fmt.Errorf("failed to decompress data: %w", err)
	}

	// Compute the SHA-256 hash of the decompressed data
	hash := sha256.Sum256(decompressedData)
	hashHex := fmt.Sprintf("%x", hash[:])

	// Define the output file path
	outputDir := fork + "/" + object + "/"
	outputFileName := fmt.Sprintf("%s.ssz", hashHex)
	outputFilePath := outputDir + outputFileName

	// Ensure the output directory exists
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create the output file
	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Write the decompressed data to the output file
	_, err = outputFile.Write(decompressedData)
	if err != nil {
		return "", fmt.Errorf("failed to write decompressed data to output file: %w", err)
	}

	return outputFilePath, nil
}
