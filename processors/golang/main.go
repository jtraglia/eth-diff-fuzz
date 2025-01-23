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

const (
	socketName  = "/tmp/eth-cl-fuzz"
	shmInputKey = 0
	shmMaxSize  = 100 * 1024 * 1024 // 100 MiB
	shmPerm     = 0666
)

func main() {
	fmt.Println("Connecting to driver...")
	stream, err := net.Dial("unix", socketName)
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
	shmInputID, err := shm.Get(shmInputKey, shmMaxSize, shmPerm)
	if err != nil {
		log.Fatalf("Error getting input shared memory segment: %v", err)
	}
	shmInputAddr, err := shm.At(shmInputID, 0, 0)
	if err != nil {
		log.Fatalf("Error attaching to input shared memory: %v", err)
	}
	defer func() {
		shm.Dt(shmInputAddr)
		shm.Ctl(shmInputID, shm.IPC_RMID, nil)
	}()

	// Receive the output shared memory key from the driver
	var shmOutputKeyBuffer [4]byte
	_, err = stream.Read(shmOutputKeyBuffer[:])
	if err != nil {
		log.Fatalf("Failed to read key from socket: %v", err)
	}
	shmOutputKey := int(binary.BigEndian.Uint32(shmOutputKeyBuffer[:]))

	// Attach to the output shared memory segment
	shmOutputID, err := shm.Get(shmOutputKey, shmMaxSize, shmPerm)
	if err != nil {
		log.Fatalf("Error getting output shared memory segment: %v", err)
	}
	shmOutputAddr, err := shm.At(shmOutputID, 0, 0)
	if err != nil {
		log.Fatalf("Error attaching to output shared memory: %v", err)
	}
	defer func() {
		shm.Dt(shmOutputAddr)
		shm.Ctl(shmOutputID, shm.IPC_RMID, nil)
	}()

	// Create a channel to handle Ctrl+C
	running := int32(1)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		fmt.Println("\nCtrl+C detected! Cleaning up...")
		atomic.StoreInt32(&running, 0)
	}()

	fmt.Println("Running... Press Ctrl+C to exit")

	// Fuzzing loop
	for atomic.LoadInt32(&running) == 1 {
		var inputSizeBuffer [4]byte
		_, err := stream.Read(inputSizeBuffer[:])
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("Driver disconnected")
			} else {
				fmt.Printf("Failed to read size from socket: %v\n", err)
			}
			break
		}

		// Get the input
		inputSize := binary.BigEndian.Uint32(inputSizeBuffer[:])
		input := shmInputAddr[:inputSize]

		// Process the input
		startTime := time.Now()
		hasher := sha256.New()
		hasher.Write(input)
		result := hasher.Sum(nil)

		output := shmOutputAddr[:len(result)]
		copy(output, result)
		elapsedTime := time.Since(startTime)
		fmt.Printf("Processing time: %v\n", elapsedTime)

		// Send the size of the processed data back to the driver
		var responseSizeBuffer [4]byte
		binary.BigEndian.PutUint32(responseSizeBuffer[:], uint32(len(result)))
		_, err = stream.Write(responseSizeBuffer[:])
		if err != nil {
			fmt.Printf("Failed to send response to driver: %v\n", err)
			break
		}
	}

	fmt.Println("Goodbye!")
}
