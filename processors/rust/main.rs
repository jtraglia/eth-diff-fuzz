use byteorder::{BigEndian, ByteOrder};
use libc::{c_int, shmat, shmctl, shmdt, shmget, size_t, IPC_RMID, MAP_FAILED, S_IRUSR, S_IWUSR};
use ring::digest;
use std::io::ErrorKind;
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::ptr;
use std::slice;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Instant;

const SOCKET_NAME: &str = "/tmp/eth-cl-fuzz";
const SHM_INPUT_KEY: i32 = 0;
const SHM_MAX_SIZE: usize = 100 * 1024 * 1024; // 100 MiB

fn main() {
    println!("Connecting to driver...");
    let mut stream = UnixStream::connect(SOCKET_NAME).expect("Failed to connect to driver");
    stream
        .write_all(b"rust")
        .expect("Failed to send name to driver");

    // Attach to the input buffer
    let shm_input_id = unsafe {
        shmget(
            SHM_INPUT_KEY,
            SHM_MAX_SIZE as size_t,
            (S_IRUSR | S_IWUSR) as c_int,
        )
    };
    if shm_input_id == -1 {
        panic!("Error getting input shared memory segment");
    }
    let shm_input_addr = unsafe { shmat(shm_input_id, ptr::null(), 0) };
    if shm_input_addr == MAP_FAILED {
        panic!("Error attaching to input shared memory");
    }

    // Get the output key from the driver
    let mut shm_output_key_buffer = [0u8; 4];
    stream
        .read_exact(&mut shm_output_key_buffer)
        .expect("Failed to read key from socket");
    let shm_output_key = BigEndian::read_u32(&shm_output_key_buffer) as i32;

    // Attach to the output buffer
    let shm_output_id = unsafe {
        shmget(
            shm_output_key,
            SHM_MAX_SIZE as size_t,
            (S_IRUSR | S_IWUSR) as c_int,
        )
    };
    if shm_output_id == -1 {
        panic!("Error getting output shared memory segment");
    }
    let shm_output_addr = unsafe { shmat(shm_output_id, ptr::null(), 0) };
    if shm_output_addr == MAP_FAILED {
        panic!("Error attaching to output shared memory");
    }

    // Create a Ctrl+C handler
    let running = Arc::new(AtomicBool::new(true));
    let running_clone = Arc::clone(&running);
    ctrlc::set_handler(move || {
        println!("\nCtrl+C detected! Cleaning up...");
        running_clone.store(false, Ordering::SeqCst);
    })
    .expect("Error setting Ctrl+C handler");
    println!("Running... Press Ctrl+C to exit");

    // The fuzzing loop
    while running.load(Ordering::SeqCst) {
        let mut input_size_buffer = [0u8; 4];
        match stream.read_exact(&mut input_size_buffer) {
            Ok(_) => {
                // Get the input
                let input_size = BigEndian::read_u32(&input_size_buffer) as usize;
                let input: &[u8] =
                    unsafe { slice::from_raw_parts(shm_input_addr as *const u8, input_size) };

                // Process the input in some way...
                let start_time = Instant::now();
                let result = digest::digest(&digest::SHA256, input);
                let output_size = digest::SHA256.output_len;
                let output: &mut [u8] =
                    unsafe { slice::from_raw_parts_mut(shm_output_addr as *mut u8, output_size) };
                //output.copy_from_slice(&result);
                output.copy_from_slice(result.as_ref());
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
                // Print a nice message if the driver disconnects
                if e.kind() == ErrorKind::UnexpectedEof {
                    println!("Driver disconnected");
                } else {
                    println!("Failed to read size from socket: {}", e);
                }
                break;
            }
        }
    }

    unsafe {
        shmdt(shm_input_addr);
        shmctl(shm_input_id, IPC_RMID, ptr::null_mut());
        shmdt(shm_output_addr);
        shmctl(shm_output_id, IPC_RMID, ptr::null_mut());
    };

    println!("Goodbye!");
}
