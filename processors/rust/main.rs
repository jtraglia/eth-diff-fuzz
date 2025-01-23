use libc::{shmat, shmget, shmdt, MAP_FAILED, S_IRUSR, S_IWUSR, c_int, size_t};
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::ptr;
use std::slice;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Instant;

const SHM_KEY: i32 = 0x1234;
const SHM_SIZE: usize = 100 * 1024 * 1024;
const SOCKET_PATH: &str = "/tmp/eth_cl_fuzz_rust_socket";

fn main() {
    // Connect to the Unix domain socket
    println!("[proc-rust] Connecting to driver...");
    let mut stream = UnixStream::connect(SOCKET_PATH).expect("Failed to connect to driver");

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
        // Read the size of the data
        let mut buffer = [0u8; 32];
        let n = stream.read(&mut buffer).expect("Failed to read from socket");
        let data_size: usize = String::from_utf8_lossy(&buffer[..n])
            .trim()
            .parse()
            .expect("Failed to parse data size");
        println!("[proc-rust] Received data size: {} bytes", data_size);

        let data: &mut [u8] = unsafe { slice::from_raw_parts_mut(shm_addr as *mut u8, data_size) };


        let start_time = Instant::now();
        println!("[proc-rust] Reversing the data...");
        data.reverse();
        let elapsed_time = start_time.elapsed();
        println!("[proc-rust] Processing time: {:.2?}", elapsed_time);

        // Send the size of the processed data back to driver
        let response = format!("{}", data_size);
        stream
            .write_all(response.as_bytes())
            .expect("Failed to send response to driver");
        println!("[proc-rust] Sent response to driver");
    }

    // Code to execute when the program exits
    println!("Exiting gracefully. Cleanup completed");

    // Detach from shared memory
    unsafe { shmdt(shm_addr) };
    println!("[proc-rust] Detached from shared memory");
}
