use libc::{shmat, shmget, shmdt, shmctl, MAP_FAILED, S_IRUSR, S_IWUSR, IPC_RMID, c_int, size_t};
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::ptr;
use std::slice;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Instant;
use byteorder::{BigEndian, ByteOrder};

const SHM_KEY: i32 = 0;
const SHM_SIZE: usize = 100 * 1024 * 1024;
const SOCKET_NAME: &str = "/tmp/eth-cl-fuzz";

fn main() {
    // Connect to the Unix domain socket
    println!("[proc-rust] Connecting to driver...");
    let mut stream = UnixStream::connect(SOCKET_NAME).expect("Failed to connect to driver");
    stream.write_all(b"lighthouse").expect("Failed to send name to driver");

    // Attach to the shared memory segment
    let shm_read_id = unsafe { shmget(SHM_KEY, SHM_SIZE as size_t, (S_IRUSR | S_IWUSR) as c_int) };
    println!("[proc-rust] read id: {}", shm_read_id);
    if shm_read_id == -1 {
        panic!("Error getting shared memory segment");
    }

    let shm_read_addr = unsafe { shmat(shm_read_id, ptr::null(), 0) };
    if shm_read_addr == MAP_FAILED {
        panic!("Error attaching to shared memory");
    }

    let mut shm_key_buffer = [0u8; 4];
    stream.read_exact(&mut shm_key_buffer).expect("Failed to read key from socket");
    let shm_key = BigEndian::read_u32(&shm_key_buffer) as i32;
    println!("[proc-rust] Received smh key: {}", shm_key);

    let shm_write_id = unsafe { shmget(shm_key, SHM_SIZE as size_t, (S_IRUSR | S_IWUSR) as c_int) };
    println!("[proc-rust] write id: {}", shm_write_id);
    if shm_write_id == -1 {
        panic!("Error getting shared memory segment");
    }

    let shm_write_addr = unsafe { shmat(shm_write_id, ptr::null(), 0) };
    if shm_write_addr == MAP_FAILED {
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
        match stream.read_exact(&mut size_buffer) {
            Ok(_) => {
                let data_size = BigEndian::read_u32(&size_buffer) as usize;
                println!("[proc-rust] Received data size: {} bytes", data_size);

                // Get a mutable slice from the shared memory
                let input: &[u8] = unsafe { slice::from_raw_parts(shm_read_addr as *const u8, data_size) };
                let output: &mut [u8] = unsafe { slice::from_raw_parts_mut(shm_write_addr as *mut u8, data_size) };

                // Process the data
                let start_time = Instant::now();
                println!("[proc-rust] Processing data...");
                // Increment each byte in `input` and assign it to `output`
                for (src, dst) in input.iter().zip(output.iter_mut()) {
                    *dst = src.wrapping_add(1);
                }
                let elapsed_time = start_time.elapsed();
                println!("[proc-rust] Processing time: {:.2?}", elapsed_time);

                // Send the processed data size back to the driver as 4 bytes
                let mut response_size_buffer = [0u8; 4];
                BigEndian::write_u32(&mut response_size_buffer, data_size as u32);
                if let Err(e) = stream.write_all(&response_size_buffer) {
                    println!("[proc-rust] Failed to send response to driver: {}", e);
                    break;
                }
                println!("[proc-rust] Sent response size to driver");
            }
            Err(e) => {
                // Break the loop if the driver stops or an error occurs
                println!("[proc-rust] Failed to read size from socket: {}", e);
                break;
            }
        }
    }

    // Code to execute when the program exits
    println!("Exiting gracefully. Cleanup completed");

    // Detach from shared memory
    unsafe {
        shmdt(shm_read_addr);
        shmdt(shm_write_addr);

        // Mark shared memory segments for deletion
        if shmctl(shm_read_id, IPC_RMID, ptr::null_mut()) == -1 {
            eprintln!("[proc-rust] Failed to remove shared memory (read) with ID: {}", shm_read_id);
        } else {
            println!("[proc-rust] Successfully removed shared memory (read) with ID: {}", shm_read_id);
        }

        if shmctl(shm_write_id, IPC_RMID, ptr::null_mut()) == -1 {
            eprintln!("[proc-rust] Failed to remove shared memory (write) with ID: {}", shm_write_id);
        } else {
            println!("[proc-rust] Successfully removed shared memory (write) with ID: {}", shm_write_id);
        }
    };
    println!("[proc-rust] Detached from shared memory");
}
