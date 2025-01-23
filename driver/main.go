package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"unsafe"
)

const (
	shmKey     = 0x1234            // Shared memory key
	shmSize    = 100 * 1024 * 1024 // Shared memory size (100 MiB)
	socketPath = "/tmp/eth_cl_fuzz_rust_socket"

	// System V IPC flags for macOS
	IPC_CREAT = 01000
	IPC_EXCL  = 02000
	IPC_RMID  = 0
)

// generatePseudoRandomData generates pseudo-random data for testing.
func generatePseudoRandomData(shmSize int) []byte {
	data := make([]byte, shmSize)
	for i := 0; i < shmSize; i++ {
		data[i] = byte(i % 256)
	}
	return data
}

// shmget wraps the System V shmget syscall.
func shmget(key int, shmSize int, shmflg int) (int, error) {
	id, _, errno := syscall.Syscall(syscall.SYS_SHMGET, uintptr(key), uintptr(shmSize), uintptr(shmflg))
	if errno != 0 {
		return 0, errno
	}
	return int(id), nil
}

// shmat wraps the System V shmat syscall.
func shmat(shmid int, shmaddr uintptr, shmflg int) (uintptr, error) {
	addr, _, errno := syscall.Syscall(syscall.SYS_SHMAT, uintptr(shmid), shmaddr, uintptr(shmflg))
	if errno != 0 {
		return 0, errno
	}
	return addr, nil
}

// shmdt wraps the System V shmdt syscall.
func shmdt(shmaddr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_SHMDT, shmaddr, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// shmctl wraps the System V shmctl syscall.
func shmctl(shmid int, cmd int, buf unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_SHMCTL, uintptr(shmid), uintptr(cmd), uintptr(buf))
	if errno != 0 {
		return errno
	}
	return nil
}

// deleteExistingSharedMemory checks if the shared memory already exists and deletes it.
func deleteExistingSharedMemory(key int) {
	shmID, err := shmget(key, 0, 0666)
	if err == nil {
		fmt.Printf("[driver] Found existing shared memory segment with ID %d, removing it...\n", shmID)
		err := shmctl(shmID, IPC_RMID, nil)
		if err != nil {
			fmt.Printf("[driver] Failed to remove existing shared memory: %v\n", err)
		} else {
			fmt.Println("[driver] Successfully removed existing shared memory.")
		}
	} else {
		fmt.Println("[driver] No existing shared memory segment found.")
	}
}

func main() {
	deleteExistingSharedMemory(shmKey)

	// Create the shared memory segment
	shmID, err := shmget(int(shmKey), shmSize, IPC_CREAT|IPC_EXCL|0666)
	if err != nil {
		fmt.Printf("Error creating shared memory: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[driver] Created shared memory segment with ID %d\n", shmID)

	// Attach to the shared memory segment
	shmAddr, err := shmat(shmID, 0, 0)
	if err != nil {
		fmt.Printf("Error attaching to shared memory: %v\n", err)
		os.Exit(1)
	}

	// Cast the address to a byte slice
	data := (*[shmSize]byte)(unsafe.Pointer(shmAddr))[:shmSize:shmSize]
	defer func() {
		shmdt(shmAddr)               // Detach from shared memory
		shmctl(shmID, IPC_RMID, nil) // Remove shared memory
	}()

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

	for {
		// Generate pseudo-random data
		dataSize := 10 * 1024 * 1024 // 10 MiB
		randomData := generatePseudoRandomData(dataSize)
		copy(data, randomData)
		fmt.Printf("[driver] Wrote %d MiB of pseudo-random data\n", dataSize/(1024*1024))

		// Send the data size to the processor
		fmt.Println("[driver] Sending data size to processor...")
		_, err = conn.Write([]byte(fmt.Sprintf("%d", dataSize)))
		if err != nil {
			fmt.Printf("Error sending message to processor: %v\n", err)
			os.Exit(1)
		}

		// Wait for a response from processor
		buffer := make([]byte, 32)
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("Error reading message from processor: %v\n", err)
			os.Exit(1)
		}
		responseSize := string(buffer[:n])
		fmt.Printf("[driver] Received response from processor: %s bytes processed\n", responseSize)
	}
}
