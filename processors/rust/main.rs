use libc::{shmat, shmget, shmdt, shmctl, MAP_FAILED, S_IRUSR, S_IWUSR, IPC_RMID, c_int, size_t};
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::ptr;
use std::slice;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::Instant;
use byteorder::{BigEndian, ByteOrder};
use sha2::Sha256;
use sha2::Digest;

const SHM_KEY: i32 = 0;
const SHM_SIZE: usize = 100 * 1024 * 1024;
const SOCKET_NAME: &str = "/tmp/eth-cl-fuzz";

fn main() {
    // Connect to the Unix domain socket
    println!("Connecting to driver...");
    let mut stream = UnixStream::connect(SOCKET_NAME).expect("Failed to connect to driver");
    stream.write_all(b"lighthouse").expect("Failed to send name to driver");

    println!("Attaching to input shared memory: {}", SHM_KEY);
    let shm_read_id = unsafe { shmget(SHM_KEY, SHM_SIZE as size_t, (S_IRUSR | S_IWUSR) as c_int) };
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

    println!("Attaching to output shared memory: {}", shm_key);
    let shm_write_id = unsafe { shmget(shm_key, SHM_SIZE as size_t, (S_IRUSR | S_IWUSR) as c_int) };
    if shm_write_id == -1 {
        panic!("Error getting shared memory segment");
    }
    let shm_write_addr = unsafe { shmat(shm_write_id, ptr::null(), 0) };
    if shm_write_addr == MAP_FAILED {
        panic!("Error attaching to shared memory");
    }

    // Create a Ctrl+C handler
    let running = Arc::new(AtomicBool::new(true));
    let running_clone = Arc::clone(&running);
    ctrlc::set_handler(move || {
        println!("\nCtrl+C detected! Cleaning up...");
        running_clone.store(false, Ordering::SeqCst);
    }).expect("Error setting Ctrl+C handler");
    println!("Running... Press Ctrl+C to exit");

    // The fuzzing loop
    while running.load(Ordering::SeqCst) {
        let mut input_size_buffer = [0u8; 4];
        match stream.read_exact(&mut input_size_buffer) {
            Ok(_) => {
                // Get the input
                let input_size = BigEndian::read_u32(&input_size_buffer) as usize;
                let input: &[u8] = unsafe { slice::from_raw_parts(shm_read_addr as *const u8, input_size) };

                // Process the input in some way...
                let start_time = Instant::now();
                let mut hasher = Sha256::new();
                hasher.update(input);
                let result = hasher.finalize();
                let output_size = result.len();
                let output: &mut [u8] = unsafe { slice::from_raw_parts_mut(shm_write_addr as *mut u8, output_size) };
                output.copy_from_slice(&result);
                let elapsed_time = start_time.elapsed();
                println!("Processing time: {:.2?}", elapsed_time);

                // Send the processed data size back to the driver as 4 bytes
                let mut response_size_buffer = [0u8; 4];
                BigEndian::write_u32(&mut response_size_buffer, output_size as u32);
                if let Err(e) = stream.write_all(&response_size_buffer) {
                    println!("Failed to send response to driver: {}", e);
                    break;
                }
            }
            Err(e) => {
                // Break the loop if the driver stops or an error occurs
                println!("Failed to read size from socket: {}", e);
                break;
            }
        }
    }

    // Code to execute when the program exits
    println!("Exiting gracefully...");

    // Detach from shared memory
    unsafe {
        shmdt(shm_read_addr);
        if shmctl(shm_read_id, IPC_RMID, ptr::null_mut()) == -1 {
            eprintln!("Failed to remove shared memory (read) with ID: {}", shm_read_id);
        }

        shmdt(shm_write_addr);
        if shmctl(shm_write_id, IPC_RMID, ptr::null_mut()) == -1 {
            eprintln!("Failed to remove shared memory (write) with ID: {}", shm_write_id);
        }
    };
}
