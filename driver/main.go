package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gen2brain/shm"
)

const (
	shmKey  = 0x1234            // Shared memory key
	shmSize = 100 * 1024 * 1024 // Shared memory size (100 MiB)
	shmPerm = 0666

	socketName = "/tmp/eth-cl-fuzz"
)

type Client struct {
	Name string
	Conn net.Conn
}

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

	var totalTime time.Duration
	var count int

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			if count != 0 {
				average := totalTime / time.Duration(count)
				fmt.Printf("[driver] Fuzzing Time: %v, Iterations: %v, Average Iteration Time: %v\n", totalTime, count, average)
			}
		}
	}()

	os.Remove(socketName)
	registrationListener, err := net.Listen("unix", socketName)
	if err != nil {
		fmt.Printf("Error creating Unix domain socket: %v\n", err)
		os.Exit(1)
	}
	defer registrationListener.Close()

	fmt.Println("[Server] IPC server started. Waiting for clients...")

	clients := make(map[string]*Client)
	mu := &sync.Mutex{}

	go func() {
		for {
			conn, err := registrationListener.Accept()
			if err != nil {
				fmt.Printf("Error accepting connection: %v\n", err)
				os.Exit(1)
			}
			//defer conn.Close()

			clientNameBytes := make([]byte, 32)
			n, err := conn.Read(clientNameBytes)
			if err != nil {
				fmt.Printf("Error reading client name: %v\n", err)
				os.Exit(1)
			}
			clientName := string(clientNameBytes[:n])

			mu.Lock()
			if _, exists := clients[clientName]; !exists {
				clients[clientName] = &Client{Name: clientName, Conn: conn}
				fmt.Printf("[Server] Registered new client: %s\n", clientName)
			}
			mu.Unlock()
		}
	}()

	messageID := 0
	for {
		if len(clients) == 0 {
			fmt.Println("[driver] No clients yet...")
			time.Sleep(1 * time.Second)
			continue
		}

		start := time.Now()
		dataSize := 50 * 1024 * 1024
		preState := generatePseudoRandomData(dataSize)
		copy(shmBuffer, preState)

		messageID++

		mu.Lock()
		wg := &sync.WaitGroup{}
		for _, client := range clients {
			wg.Add(1)
			go func(client *Client) {
				defer wg.Done()

				// Send the message to the client
				sizeBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(sizeBytes, uint32(dataSize))
				_, err := client.Conn.Write([]byte(sizeBytes))
				if err != nil {
					fmt.Printf("[Server] Error writing to client %s: %v\n", client.Name, err)
					mu.Lock()
					delete(clients, client.Name)
					mu.Unlock()
					return
				}

				// Wait for a response
				responseSizeBytes := make([]byte, 4)
				n, err := client.Conn.Read(responseSizeBytes)
				if err != nil {
					fmt.Printf("[Server] Error reading response from client %s: %v\n", client.Name, err)
					mu.Lock()
					delete(clients, client.Name)
					mu.Unlock()
					return
				}
				if n != 4 {
					fmt.Printf("Expected 4 bytes, got %d\n", n)
					os.Exit(1)
				}

				// Decode the received size
				responseSize := binary.BigEndian.Uint32(responseSizeBytes)
				_ = shmBuffer[:responseSize]
			}(client)
		}
		mu.Unlock()
		wg.Wait()

		duration := time.Since(start)
		totalTime += duration
		count++
	}
}
