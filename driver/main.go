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
	shmDriverKey = 0                 // Shared memory key
	shmSize      = 100 * 1024 * 1024 // Shared memory size (100 MiB)
	shmPerm      = 0666

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

/*
func deleteSharedMemory(shmKey int) error {
	// Delete existing shared memory if it exists
	shmId, err := shm.Get(shmKey, 0, shmPerm)
	if err == nil {
		fmt.Printf("[driver] Found existing shared memory segment with ID %d, removing it...\n", shmId)
		_, err := shm.Ctl(shmId, shm.IPC_RMID, nil)
		if err != nil {
			fmt.Printf("Failed to remove existing shared memory: %v\n", err)
			return err
		} else {
			fmt.Println("[driver] Successfully removed existing shared memory.")
		}
	}
	return nil
}
*/

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

func newSharedMemory(shmKey int) (int, []byte, error) {
	// Create the shared memory segment
	shmId, err := shm.Get(shmKey, shmSize, shmPerm|shm.IPC_CREAT|shm.IPC_EXCL)
	if err != nil {
		fmt.Printf("Error creating shared memory: %v\n", err)
		return 0, nil, err
	}
	fmt.Printf("[driver] Created shared memory segment with ID %d\n", shmId)

	// Attach to the shared memory segment
	shmBuffer, err := shm.At(shmId, 0, 0)
	if err != nil {
		fmt.Printf("Error attaching to shared memory: %v\n", err)
		return 0, nil, err
	}

	return shmId, shmBuffer, nil
}

func main() {
	clients := make(map[string]*Client)
	mu := &sync.Mutex{}

	// Handle SIGINT for cleanup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	shmId, shmBuffer, err := newSharedMemory(shmDriverKey)
	if err != nil {
		fmt.Printf("Error creating input shm: %v\n", err)
		os.Exit(1)
	}
	//defer shm.Dt(shmBuffer)
	//defer shm.Ctl(shmId, shm.IPC_RMID, nil)

	// Signal cleanup on interrupt
	go func() {
		<-signalChan
		fmt.Println("\n[INFO] Received SIGINT, cleaning up...")
		mu.Lock()
		cleanupSharedMemory(clients, shmId, shmBuffer)
		mu.Unlock()
		os.Exit(0)
	}()

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

	clientShmKey := 1
	go func() {
		for {
			conn, err := registrationListener.Accept()
			if err != nil {
				fmt.Printf("Error accepting connection: %v\n", err)
				os.Exit(1)
			}
			defer conn.Close()

			clientNameBytes := make([]byte, 32)
			n, err := conn.Read(clientNameBytes)
			if err != nil {
				fmt.Printf("Error reading client name: %v\n", err)
				os.Exit(1)
			}
			clientName := string(clientNameBytes[:n])

			clientShmId, clientShmBuffer, err := newSharedMemory(clientShmKey)
			if err != nil {
				fmt.Printf("Error creating client output shm: %v\n", err)
				os.Exit(1)
			}
			//defer shm.Dt(clientShmBuffer)
			//defer shm.Ctl(clientShmId, shm.IPC_RMID, nil)
			defer func() {
				fmt.Println("[DEBUG] Detaching shared memory buffer.")
				if err := shm.Dt(clientShmBuffer); err != nil {
					fmt.Printf("[ERROR] Failed to detach shared memory buffer: %v\n", err)
				} else {
					fmt.Println("[DEBUG] Successfully detached shared memory buffer.")
				}
			}()

			defer func() {
				fmt.Println("[DEBUG] Removing shared memory segment.")
				if _, err := shm.Ctl(clientShmId, shm.IPC_RMID, nil); err != nil {
					fmt.Printf("[ERROR] Failed to remove shared memory segment: %v\n", err)
				} else {
					fmt.Println("[DEBUG] Successfully removed shared memory segment.")
				}
			}()

			clientShmKeyBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(clientShmKeyBytes, uint32(clientShmKey))
			_, err = conn.Write([]byte(clientShmKeyBytes))
			if err != nil {
				fmt.Printf("[Server] Error writing to client %s: %v\n", clientName, err)
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
				fmt.Printf("[Server] Registered new client: %s\n", clientName)
			}
			mu.Unlock()

			clientShmKey += 1
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
		dataSize := 10 * 1024 * 1024
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
