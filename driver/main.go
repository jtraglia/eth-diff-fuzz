package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gen2brain/shm"
)

const (
	shmDriverKey = 1000
	shmSize      = 100 * 1024 * 1024
	shmPerm      = 0666

	maxClientNameLength = 32

	socketName = "/tmp/eth-cl-fuzz"
)

type Client struct {
	Name      string
	Conn      net.Conn
	ShmId     int
	ShmBuffer []byte
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

// cleanupSharedMemory detaches and removes all shared memories.
func cleanupSharedMemory(clients map[string]*Client, shmId int, shmBuffer []byte) {
	// Cleanup driver shared memory
	fmt.Println("[cleanup] Detaching and removing driver shared memory...")
	if err := shm.Dt(shmBuffer); err != nil {
		fmt.Printf("[ERROR] Failed to detach driver shared memory: %v\n", err)
	}
	if _, err := shm.Ctl(shmId, shm.IPC_RMID, nil); err != nil {
		fmt.Printf("[ERROR] Failed to remove driver shared memory: %v\n", err)
	}

	// Cleanup client shared memory
	fmt.Println("[cleanup] Detaching and removing client shared memory segments...")
	for _, client := range clients {
		if err := shm.Dt(client.ShmBuffer); err != nil {
			fmt.Printf("[ERROR] Failed to detach shared memory for client %s: %v\n", client.Name, err)
		}
		if _, err := shm.Ctl(client.ShmId, shm.IPC_RMID, nil); err != nil {
			fmt.Printf("[ERROR] Failed to remove shared memory segment for client %s: %v\n", client.Name, err)
		}
		client.Conn.Close()
	}
}

// newSharedMemory creates a new shared memory segment.
func newSharedMemory(shmKey int) (int, []byte, error) {
	// Create the shared memory segment
	shmId, err := shm.Get(shmKey, shmSize, shmPerm|shm.IPC_CREAT|shm.IPC_EXCL)
	if err != nil {
		fmt.Printf("Error creating shared memory: %v\n", err)
		return 0, nil, err
	}
	fmt.Printf("Created shared memory segment with ID %d\n", shmId)

	// Attach to the shared memory segment
	shmBuffer, err := shm.At(shmId, 0, 0)
	if err != nil {
		fmt.Printf("Error attaching to shared memory: %v\n", err)
		return 0, nil, err
	}

	return shmId, shmBuffer, nil
}

func main() {
	mu := &sync.Mutex{}
	clients := make(map[string]*Client)

	// Handle SIGINT for cleanup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	shmId, shmBuffer, err := newSharedMemory(shmDriverKey)
	if err != nil {
		fmt.Printf("Error creating input shm: %v\n", err)
		os.Exit(1)
	}

	// A thread for cleanup
	go func() {
		<-signalChan
		fmt.Println("\n[INFO] Received SIGINT, cleaning up...")
		mu.Lock()
		cleanupSharedMemory(clients, shmId, shmBuffer)
		mu.Unlock()
		os.Exit(0)
	}()

	// A thread for status updates
	var count int
	var totalTime time.Duration
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			if count != 0 {
				average := totalTime / time.Duration(count)
				fmt.Printf("Fuzzing Time: %v, Iterations: %v, Average Iteration Time: %v\n", totalTime, count, average)
			}
		}
	}()

	//os.Remove(socketName)
	registrationListener, err := net.Listen("unix", socketName)
	if err != nil {
		fmt.Printf("Error creating Unix domain socket: %v\n", err)
		os.Exit(1)
	}
	defer registrationListener.Close()
	defer os.Remove(socketName)

	fmt.Println("IPC server started. Waiting for clients...")

	// A thread for client registrations
	go func() {
		for {
			conn, err := registrationListener.Accept()
			if err != nil {
				fmt.Printf("Error accepting connection: %v\n", err)
				os.Exit(1)
			}
			defer conn.Close()

			clientNameBytes := make([]byte, maxClientNameLength)
			n, err := conn.Read(clientNameBytes)
			if err != nil {
				fmt.Printf("Error reading client name: %v\n", err)
				os.Exit(1)
			}
			clientName := string(clientNameBytes[:n])

			clientShmKey := shmDriverKey + len(clients)
			clientShmId, clientShmBuffer, err := newSharedMemory(clientShmKey)
			if err != nil {
				fmt.Printf("Error creating client output shm: %v\n", err)
				os.Exit(1)
			}

			clientShmKeyBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(clientShmKeyBytes, uint32(clientShmKey))
			_, err = conn.Write([]byte(clientShmKeyBytes))
			if err != nil {
				fmt.Printf("Error writing to client %s: %v\n", clientName, err)
				return
			}

			mu.Lock()
			if _, exists := clients[clientName]; !exists {
				clients[clientName] = &Client{
					Name:      clientName,
					Conn:      conn,
					ShmId:     clientShmId,
					ShmBuffer: clientShmBuffer,
				}
				fmt.Printf("Registered new client: %s\n", clientName)
			}
			mu.Unlock()
		}
	}()

	for {
		mu.Lock()
		start := time.Now()

		// Wait for at least one client to connect
		if len(clients) == 0 {
			fmt.Println("No clients yet...")
			time.Sleep(1 * time.Second)
			continue
		}

		// Generate some input & throw it into the input buffer
		inputSize := 10 * 1024 * 1024
		input := generatePseudoRandomData(inputSize)
		copy(shmBuffer, input)

		wg := &sync.WaitGroup{}
		for _, client := range clients {
			wg.Add(1)
			go func(client *Client) {
				defer wg.Done()

				// Send the message to the client
				sizeBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(sizeBytes, uint32(inputSize))
				_, err := client.Conn.Write([]byte(sizeBytes))
				if err != nil {
					fmt.Printf("Error writing to client %s: %v\n", client.Name, err)
					mu.Lock()
					delete(clients, client.Name)
					mu.Unlock()
					return
				}

				// Wait for a response
				responseSizeBytes := make([]byte, 4)
				_, err = client.Conn.Read(responseSizeBytes)
				if err != nil {
					fmt.Printf("Error reading response from client %s: %v\n", client.Name, err)
					mu.Lock()
					delete(clients, client.Name)
					mu.Unlock()
					return
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
