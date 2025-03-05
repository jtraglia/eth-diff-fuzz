package main

import (
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
	"processor/execution/precompiles"
)

var (
	geth = precompiles.Geth{}
)

func processInput(method string, input []byte, is_execution bool) ([]byte, error) {
	if is_execution {
		switch method {
		case "ecrecover":
			return geth.EcRecover(input)
		case "sha256":
			return geth.Sha256(input)
		case "ripemd160":
			return geth.Ripemd160(input)
		case "dataCopy":
			return geth.DataCopy(input)
		case "bigModExp":
			return []byte(fmt.Sprintf("%d", geth.BigModExp(input, false))), nil
		case "modExp":
			ret, err := geth.ModExp(input)
			if err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("%d", ret)), nil
		case "bn256Add":
			return geth.Bn256Add(input)
		case "bn256ScalarMul":
			return geth.Bn256ScalarMul(input)
		case "bn256Pairing":
			return geth.Bn256Pairing(input)
		case "blake2F":
			return geth.Blake2F(input)
		case "bls12381G1Add":
			return geth.Bls12381G1Add(input)
		case "bls12381G1MultiExp":
			return geth.Bls12381G1MultiExp(input)
		case "bls12381G2Add":
			return geth.Bls12381G2Add(input)
		case "bls12381G2MultiExp":
			return geth.Bls12381G2MultiExp(input)
		case "bls12381Pairing":
			return geth.Bls12381Pariring(input)
		case "bls12381MapG1":
			return geth.Bls12381MapG1(input)
		case "bls12381MapG2":
			return geth.Bls12381MapG2(input)
		case "kzg":
			return geth.KzgPointEvaluation(input)
		default:
			return nil, fmt.Errorf("[TODO nethoxa] impl execution function set")
		}
	} else {
		return nil, fmt.Errorf("[TODO nethoxa] impl consensus function set")
	}
}

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

	// Get the method to fuzz
	var methodBytes [64]byte
	methodLength, err := stream.Read(methodBytes[:])
	var method = string(methodBytes[:methodLength])
	fmt.Printf("Fuzzing method: %s\n", method)

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
			result, err := processInput(method, inputShm[:inputSize], true) // [TODO: nethoxa] true for testing

			// Write result to output
			if err != nil {
				copy(outputShm[:len(err.Error())], []byte(err.Error()))
			} else {
				copy(outputShm[:len(result)], result)
			}
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
