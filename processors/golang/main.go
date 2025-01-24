package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gen2brain/shm"
)

func main() {
	fmt.Println("Connecting to driver...")
	stream, err := net.Dial("unix", "/tmp/eth-cl-fuzz")
	if err != nil {
		log.Fatalf("Failed to connect to driver: %v", err)
	}
	defer stream.Close()

	// Send client name
	_, err = stream.Write([]byte("golang"))
	if err != nil {
		log.Fatalf("Failed to send name to driver: %v", err)
	}

	// Attach to the input shared memory segment
	var inputShmIdBytes [4]byte
	_, err = stream.Read(inputShmIdBytes[:])
	if err != nil {
		log.Fatalf("Failed to read key from socket: %v", err)
	}
	inputShmId := int(binary.BigEndian.Uint32(inputShmIdBytes[:]))
	inputShm, err := shm.At(inputShmId, 0, 0)
	if err != nil {
		log.Fatalf("Error attaching to input shared memory: %v", err)
	}
	defer shm.Dt(inputShm)

	// Attach to the output shared memory segment
	var outputShmIdBytes [4]byte
	_, err = stream.Read(outputShmIdBytes[:])
	if err != nil {
		log.Fatalf("Failed to read key from socket: %v", err)
	}
	outputShmId := int(binary.BigEndian.Uint32(outputShmIdBytes[:]))
	outputShm, err := shm.At(outputShmId, 0, 0)
	if err != nil {
		log.Fatalf("Error attaching to output shared memory: %v", err)
	}
	defer shm.Dt(outputShm)

	// Create a channel to handle Ctrl+C
	running := int32(1)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	fmt.Println("Running... Press Ctrl+C to exit")

	// Fuzzing loop
	for atomic.LoadInt32(&running) == 1 {
		var inputSizeBytes [4]byte
		readDone := make(chan error, 1)

		// Perform the blocking read in a separate goroutine
		go func() {
			_, err := stream.Read(inputSizeBytes[:])
			readDone <- err
		}()

		select {
		case err := <-readDone:
			if err != nil {
				if err.Error() == "EOF" {
					fmt.Println("Driver disconnected")
					fmt.Println("Goodbye!")
				} else {
					fmt.Printf("Failed to read size from socket: %v\n", err)
				}
				return
			}

			// Get the input
			inputSize := binary.BigEndian.Uint32(inputSizeBytes[:])

			// Process the input
			startTime := time.Now()
			hasher := sha256.New()
			hasher.Write(inputShm[:inputSize])
			result := hasher.Sum(nil)

			// Write result to output
			copy(outputShm[:len(result)], result)
			elapsedTime := time.Since(startTime)
			fmt.Printf("Processing time: %v\n", elapsedTime)

			// Send the size of the processed data back to the driver
			var responseSizeBuffer [4]byte
			binary.BigEndian.PutUint32(responseSizeBuffer[:], uint32(len(result)))
			_, err = stream.Write(responseSizeBuffer[:])
			if err != nil {
				fmt.Printf("Failed to send response to driver: %v\n", err)
				return
			}
		case <-signalChan:
			fmt.Println("\nCtrl+C detected")
			fmt.Println("Goodbye!")
			return
		}
	}
}
