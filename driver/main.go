package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gen2brain/shm"
)

const (
	socketName = "/tmp/eth-cl-fuzz"

	maxClientNameLength = 32

	inputShmKey = 1000
	shmMaxSize  = 100 * 1024 * 1024 // 100 MiB
	shmPerm     = 0666
)

type Client struct {
	Name      string
	Conn      net.Conn
	ShmId     int
	ShmBuffer []byte
	Method    string
}

// detachAndDelete detaches and deletes a shared memory region.
func detachAndDelete(shmId int, shmBuffer []byte) {
	// Cleanup driver shared memory
	if err := shm.Dt(shmBuffer); err != nil {
		fmt.Printf("Failed to detach driver shared memory: %v\n", err)
	}
	if _, err := shm.Ctl(shmId, shm.IPC_RMID, nil); err != nil {
		fmt.Printf("Failed to remove driver shared memory: %v\n", err)
	}
}

// newSharedMemory creates a new shared memory segment.
func newSharedMemory(shmKey int) (int, []byte, error) {
	// Create the shared memory segment
	shmId, err := shm.Get(shmKey, shmMaxSize, shmPerm|shm.IPC_CREAT|shm.IPC_EXCL)
	if err != nil {
		fmt.Printf("Error creating shared memory: %v\n", err)
		return 0, nil, err
	}

	// Attach to the shared memory segment
	shmBuffer, err := shm.At(shmId, 0, 0)
	if err != nil {
		fmt.Printf("Error attaching to shared memory: %v\n", err)
		return 0, nil, err
	}

	return shmId, shmBuffer, nil
}

func directoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func main() {
	mu := &sync.Mutex{}
	clients := make(map[string]*Client)

	// Initialize the corpus
	corpusExists, err := directoryExists("corpus")
	if err != nil {
		fmt.Printf("Error checking if directory exists: %v\n", err)
		os.Exit(1)
	}
	if !corpusExists {
		err := InitializeCorpus()
		if err != nil {
			fmt.Printf("Error initializing corpus: %v\n", err)
			os.Exit(1)
		}
	}

	// Handle SIGINT for cleanup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	inputShmId, inputShmBuffer, err := newSharedMemory(inputShmKey)
	if err != nil {
		fmt.Printf("Error creating input shm: %v\n", err)
		os.Exit(1)
	}

	// Create unix domain socket for small communications
	os.Remove(socketName)
	registrationListener, err := net.Listen("unix", socketName)
	if err != nil {
		fmt.Printf("Error creating Unix domain socket: %v\n", err)
		os.Exit(1)
	}

	// A thread for cleanup
	go func() {
		<-signalChan
		fmt.Println("\nReceived interrupt")
		mu.Lock()
		detachAndDelete(inputShmId, inputShmBuffer)
		for _, client := range clients {
			client.Conn.Close()
			detachAndDelete(client.ShmId, client.ShmBuffer)
		}
		mu.Unlock()
		registrationListener.Close()
		os.Remove(socketName)
		fmt.Println("Goodbye!")
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
				var clientNames []string
				for clientName := range clients {
					clientNames = append(clientNames, clientName)
				}
				sort.Strings(clientNames)
				joinedNames := strings.Join(clientNames, ",")
				fmt.Printf("Fuzzing Time: %s, Iterations: %v, Average Iteration: %s, Clients: %v\n",
					totalTime.Round(time.Second), count, average.Round(time.Millisecond), joinedNames)
			}
		}
	}()

	// A thread for client registrations
	go func() {
		for {
			conn, err := registrationListener.Accept()
			if err != nil {
				// Don't print error if we close the registration listener
				if !strings.Contains(err.Error(), "use of closed network connection") {
					fmt.Printf("Error accepting connection: %v\n", err)
				}
				return
			}
			defer conn.Close()

			clientNameBytes := make([]byte, maxClientNameLength)
			n, err := conn.Read(clientNameBytes)
			if err != nil {
				fmt.Printf("Error reading client name: %v\n", err)
				return
			}
			clientName := string(clientNameBytes[:n])

			inputShmIdBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(inputShmIdBytes, uint32(inputShmId))
			_, err = conn.Write([]byte(inputShmIdBytes))
			if err != nil {
				fmt.Printf("Error writing to client %s: %v\n", clientName, err)
				return
			}

			outputShmKey := inputShmKey + len(clients) + 1
			outputShmId, clientShmBuffer, err := newSharedMemory(outputShmKey)
			if err != nil {
				fmt.Printf("Error creating client output shm: %v\n", err)
				return
			}

			outputShmIdBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(outputShmIdBytes, uint32(outputShmId))
			_, err = conn.Write([]byte(outputShmIdBytes))
			if err != nil {
				fmt.Printf("Error writing to client %s: %v\n", clientName, err)
				return
			}

			_, err = conn.Write([]byte("sha"))
			if err != nil {
				fmt.Printf("Error writing to client %s: %v\n", clientName, err)
				return
			}

			mu.Lock()
			if _, exists := clients[clientName]; !exists {
				clients[clientName] = &Client{
					Name:      clientName,
					Conn:      conn,
					ShmId:     outputShmId,
					ShmBuffer: clientShmBuffer,
					Method:    "sha",
				}
				fmt.Printf("Registered new client: %s\n", clientName)
			}
			mu.Unlock()
		}
	}()

	var seed int64 = 0

	for {
		start := time.Now()

		// Wait for at least one client to connect
		mu.Lock()
		numClients := len(clients)
		mu.Unlock()
		if numClients == 0 {
			fmt.Println("Waiting for a client...")
			time.Sleep(1 * time.Second)
			count = 0
			totalTime = 0
			continue
		}

		// Generate a random state
		state, err := Get("electra", "BeaconState", seed)
		if err != nil {
			fmt.Println(err)
			continue
		}

		// Mutate the state
		mutatedState := Mutate(state, seed)

		// Copy the mutated state into the input buffer
		copy(inputShmBuffer, mutatedState)

		mu.Lock()
		wg := &sync.WaitGroup{}
		muResult := &sync.Mutex{}
		results := make(map[string][]byte)
		for _, client := range clients {
			wg.Add(1)
			go func(client *Client) {
				defer wg.Done()

				// Send the input size to the client
				sizeBytes := make([]byte, 4)
				binary.BigEndian.PutUint32(sizeBytes, uint32(len(mutatedState)))
				_, err := client.Conn.Write([]byte(sizeBytes))
				if err != nil {
					if strings.Contains(err.Error(), "broken pipe") {
						fmt.Printf("Client disconnected: %v\n", client.Name)
					} else {
						fmt.Printf("Error writing to client %s: %v\n", client.Name, err)
					}
					detachAndDelete(client.ShmId, client.ShmBuffer)
					delete(clients, client.Name)
					return
				}

				// Wait for a response size
				responseSizeBytes := make([]byte, 4)
				_, err = client.Conn.Read(responseSizeBytes)
				if err != nil {
					if !strings.Contains(err.Error(), "EOF") {
						fmt.Printf("Error reading response from client %s: %v\n", client.Name, err)
					}
					detachAndDelete(client.ShmId, client.ShmBuffer)
					delete(clients, client.Name)
					return
				}
				responseSize := binary.BigEndian.Uint32(responseSizeBytes)

				// Write the response to the results map
				muResult.Lock()
				results[client.Name] = client.ShmBuffer[:responseSize]
				muResult.Unlock()
			}(client)
		}
		wg.Wait()
		mu.Unlock()

		same := true
		var first []byte
		firstResultSet := false
		for _, result := range results {
			if !firstResultSet {
				first = result
				firstResultSet = true
			} else if !bytes.Equal(result, first) {
				same = false
				break
			}
		}
		if !same {
			fmt.Println("Values are different:")
			for client, result := range results {
				fmt.Printf("Key: %v, Value: %x\n", client, result)
			}
		}

		duration := time.Since(start)
		totalTime += duration
		count++
		seed++
	}
}
