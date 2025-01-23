use libc::{shmat, shmget, shmdt, MAP_FAILED, S_IRUSR, S_IWUSR, c_int, size_t};
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::ptr;
use std::slice;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Instant;
use byteorder::{BigEndian, ByteOrder};

const SHM_KEY: i32 = 0x1234;
const SHM_SIZE: usize = 100 * 1024 * 1024;
const SOCKET_NAME: &str = "/tmp/eth-cl-fuzz";

fn main() {
    // Connect to the Unix domain socket
    println!("[proc-rust] Connecting to driver...");
    let mut stream = UnixStream::connect(SOCKET_NAME).expect("Failed to connect to driver");
    stream.write_all(b"lighthouse").expect("Failed to send name to driver");

    // Attach to the shared memory segment
    let shm_id = unsafe { shmget(SHM_KEY, SHM_SIZE as size_t, (S_IRUSR | S_IWUSR) as c_int) };
    if shm_id == -1 {
        panic!("Error getting shared memory segment");
    }

    let shm_addr = unsafe { shmat(shm_id, ptr::null(), 0) };
    if shm_addr == MAP_FAILED {
        panic!("Error attaching to shared memory");
    }

    // Create a shared atomic flag to track if Ctrl+C has been pressed
    let running = Arc::new(AtomicBool::new(true));

    // Clone the Arc for the signal handler
    let running_clone = Arc::clone(&running);

    // Register a Ctrl+C handler
    ctrlc::set_handler(move || {
        println!("\n[proc-rust] Ctrl+C detected! Cleaning up...");
        running_clone.store(false, Ordering::SeqCst);
    }).expect("Error setting Ctrl+C handler");

    println!("[proc-rust] Running... Press Ctrl+C to exit");

    // Forever loop
    while running.load(Ordering::SeqCst) {
        // Read the 4-byte size of the data
        let mut size_buffer = [0u8; 4];
        stream.read_exact(&mut size_buffer).expect("Failed to read size from socket");
        let data_size = BigEndian::read_u32(&size_buffer) as usize;
        println!("[proc-rust] Received data size: {} bytes", data_size);

        // Get a mutable slice from the shared memory
        let data: &mut [u8] = unsafe { slice::from_raw_parts_mut(shm_addr as *mut u8, data_size) };

        // Process the data (reverse it)
        let start_time = Instant::now();
        println!("[proc-rust] Reversing the data...");
        data.reverse();
        let elapsed_time = start_time.elapsed();
        println!("[proc-rust] Processing time: {:.2?}", elapsed_time);

        // Send the processed data size back to the driver as 4 bytes
        let mut response_size_buffer = [0u8; 4];
        BigEndian::write_u32(&mut response_size_buffer, data_size as u32);
        stream
            .write_all(&response_size_buffer)
            .expect("Failed to send response to driver");
        println!("[proc-rust] Sent response size to driver");
    }

    // Code to execute when the program exits
    println!("Exiting gracefully. Cleanup completed");

    // Detach from shared memory
    unsafe { shmdt(shm_addr) };
    println!("[proc-rust] Detached from shared memory");
}
