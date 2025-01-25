package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/snappy"
)

func PopulateBeaconStateCorpus(fork string) ([]string, error) {
	var files []string

	err := filepath.WalkDir("corpus/tests/mainnet/"+fork, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), "pre.ssz_snappy") {
			fmt.Println("hello")
			outputFilePath, err := decompressRawSnappyFile(path)
			if err != nil {
				return err
			}
			files = append(files, outputFilePath)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	return files, nil
}

func decompressRawSnappyFile(inputPath string) (string, error) {
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
	outputDir := "corpus/BeaconState/electra/"
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

	fmt.Printf("Decompressed file saved as: %s\n", outputFilePath)
	return outputFilePath, nil
}
