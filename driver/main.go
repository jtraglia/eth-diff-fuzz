package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gen2brain/shm"
)

const (
	shmKey     = 0x1234            // Shared memory key
	shmSize    = 100 * 1024 * 1024 // Shared memory size (100 MiB)
	shmPerm    = 0666
	socketPath = "/tmp/eth_cl_fuzz_rust_socket"
)

// generatePseudoRandomData generates pseudo-random data for testing.
func generatePseudoRandomData(shmSize int) []byte {
	data := make([]byte, shmSize)
	_, err := rand.Read(data)
	if err != nil {
		panic("Failed to generate random data")
	}
	return data
}

func main() {
	// Delete existing shared memory if it exists
	shmId, err := shm.Get(shmKey, 0, shmPerm)
	if err == nil {
		fmt.Printf("[driver] Found existing shared memory segment with ID %d, removing it...\n", shmId)
		_, err := shm.Ctl(shmId, shm.IPC_RMID, nil)
		if err != nil {
			fmt.Printf("Failed to remove existing shared memory: %v\n", err)
			os.Exit(1)
		} else {
			fmt.Println("[driver] Successfully removed existing shared memory.")
		}
	}

	// Create the shared memory segment
	shmId, err = shm.Get(shmKey, shmSize, shmPerm|shm.IPC_CREAT|shm.IPC_EXCL)
	if err != nil {
		fmt.Printf("Error creating shared memory: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[driver] Created shared memory segment with ID %d\n", shmId)

	// Attach to the shared memory segment
	shmBuffer, err := shm.At(shmId, 0, 0)
	if err != nil {
		fmt.Printf("Error attaching to shared memory: %v\n", err)
		os.Exit(1)
	}

	defer shm.Dt(shmBuffer)
	defer shm.Ctl(shmId, shm.IPC_RMID, nil)

	// Create and listen on a Unix domain socket
	os.Remove(socketPath) // Clean up any existing socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Printf("Error creating Unix domain socket: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("[driver] Waiting for processor to connect...")
	conn, err := listener.Accept()
	if err != nil {
		fmt.Printf("Error accepting connection: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	var totalTime time.Duration
	var count int

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			average := totalTime / time.Duration(count)
			fmt.Printf("[driver] Fuzzing Time: %v, Iterations: %v, Average Iteration Time: %v\n", totalTime, count, average)
		}
	}()

	for {
		// Generate some random preState
		dataSize := 50 * 1024 * 1024 // 10 MiB
		preState := generatePseudoRandomData(dataSize)

		start := time.Now()
		copy(shmBuffer, preState)

		// Send the preState size to the processor
		sizeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(sizeBytes, uint32(dataSize))
		_, err = conn.Write(sizeBytes)
		if err != nil {
			fmt.Printf("Error sending preState size to processor: %v\n", err)
			os.Exit(1)
		}

		// Wait for a response from processor
		buffer := make([]byte, 4)
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("Error reading postState size from processor: %v\n", err)
			os.Exit(1)
		}
		if n != 4 {
			fmt.Printf("Expected 4 bytes, got %d\n", n)
			os.Exit(1)
		}

		// Decode the received size
		responseSize := binary.BigEndian.Uint32(buffer)
		_ = shmBuffer[:responseSize]

		duration := time.Since(start)
		totalTime += duration
		count++
	}
}
