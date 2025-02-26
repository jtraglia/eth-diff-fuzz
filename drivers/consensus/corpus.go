package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/golang/snappy"
)

// FileCache caches file contents in memory.
type FileCache struct {
	mu    sync.Mutex        // Protects the cache map
	cache map[string][]byte // Map to store file contents
}

// NewFileCache creates a new FileCache.
func NewFileCache() *FileCache {
	return &FileCache{
		cache: make(map[string][]byte),
	}
}

// ReadFile reads a file, caching its contents in memory.
func (fc *FileCache) ReadFile(path string) ([]byte, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Check if the file is already in the cache
	if data, found := fc.cache[path]; found {
		return data, nil
	}

	// Read the file from disk
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Cache the file contents
	fc.cache[path] = data
	return data, nil
}

// Global cache instance
var fileCache = NewFileCache()

// Get function with caching
func Get(fork string, object string, seed int64) ([]byte, error) {
	dir := filepath.Join("corpus", fork, object)

	// Get files to choose from
	files, err := listFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in directory: %s", dir)
	}

	// Pick a random file
	r := rand.New(rand.NewSource(seed))
	randomIndex := r.Intn(len(files))
	file := files[randomIndex]

	// Construct the full file path
	filePath := filepath.Join(dir, file)

	// Use the cache to read the file
	data, err := fileCache.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// InitializeCorpus downloads test vectors & decompresses data
func InitializeCorpus() error {
	err := DownloadTests()
	if err != nil {
		return err
	}

	// Get list of forks
	forks, err := listDirectories("downloads/tests/mainnet/")
	if err != nil {
		return err
	}

	// Populate pre states
	for _, fork := range forks {
		err = populateCorpus(fork, "BeaconState", ".*/(pre|post).ssz_snappy")
		if err != nil {
			return err
		}
	}

	// Populate static objects
	for _, fork := range forks {
		objects, err := listDirectories("downloads/tests/mainnet/" + fork + "/ssz_static/")
		if err != nil {
			return err
		}

		for _, object := range objects {
			err = populateCorpus(fork, object, "/"+object+"/.*.ssz_snappy")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// listFiles returns a list of files (not including directories)
func listFiles(path string) ([]string, error) {
	// Read the directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Filter files
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// listDirectories returns a list of directories (not including regular files)
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
	err = filepath.WalkDir("downloads/tests/mainnet/"+fork, func(path string, d os.DirEntry, err error) error {
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
		fmt.Printf("No files %v.%v (pattern: %v)\n",
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
	outputDir := "corpus/" + fork + "/" + object + "/"
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
