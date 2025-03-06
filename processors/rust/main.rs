use byteorder::{BigEndian, ByteOrder};
use libc::{shmat, shmdt, MAP_FAILED};
use std::io::ErrorKind;
use std::io::{Read, Write};
use std::os::unix::net::UnixStream;
use std::ptr;
use std::slice;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Instant;

use execution::types::reth::Reth;

mod execution;

const SOCKET_NAME: &str = "/tmp/eth-cl-fuzz";

fn process_input(method: &str, input: &[u8], is_execution: bool, reth: &Reth) -> Result<Vec<u8>, String> {
    if is_execution {
        // Handle precompile call
        let result = reth.handle_precompile_call(method, input);
        match result {
            Ok(output) => Ok(output),
            Err(e) => Err(e),
        }
    } else {
        Ok(vec![])
    }
}

fn main() {
    println!("Connecting to driver...");
    let mut stream = UnixStream::connect(SOCKET_NAME).expect("Failed to connect to driver");
    stream
        .write_all(b"rust")
        .expect("Failed to send name to driver");

    // Attach to the input buffer
    let mut shm_input_id_buffer = [0u8; 4];
    stream
        .read_exact(&mut shm_input_id_buffer)
        .expect("Failed to read input id from socket");
    let shm_input_id = BigEndian::read_u32(&shm_input_id_buffer) as i32;
    let shm_input_addr = unsafe { shmat(shm_input_id, ptr::null(), 0) };
    if shm_input_addr == MAP_FAILED {
        panic!("Error attaching to input shared memory");
    }

    // Attach to the output buffer
    let mut shm_output_id_buffer = [0u8; 4];
    stream
        .read_exact(&mut shm_output_id_buffer)
        .expect("Failed to read output id from socket");
    let shm_output_id = BigEndian::read_u32(&shm_output_id_buffer) as i32;
    let shm_output_addr = unsafe { shmat(shm_output_id, ptr::null(), 0) };
    if shm_output_addr == MAP_FAILED {
        panic!("Error attaching to output shared memory");
    }

    // Get the method to fuzz
    let mut method_buffer = [0u8; 64];
    stream
        .read(&mut method_buffer)
        .expect("Failed to read method from socket");
    let method_string = String::from_utf8_lossy(&method_buffer);
    let method = method_string.trim_matches('\0').trim();
    println!("Fuzzing method: {}", method);

    // Create a Ctrl+C handler
    let running = Arc::new(AtomicBool::new(true));
    let running_clone = Arc::clone(&running);
    ctrlc::set_handler(move || {
        println!("\nCtrl+C detected");
        running_clone.store(false, Ordering::SeqCst);
    })
    .expect("Error setting Ctrl+C handler");
    println!("Running... Press Ctrl+C to exit");

    let reth = Reth::new();

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
                match process_input(method, input, true, &reth) {
                    Ok(output) => {
                        // Copy the output to the shared memory
                        let output_shm: &mut [u8] =
                            unsafe { slice::from_raw_parts_mut(shm_output_addr as *mut u8, output.len()) };
                        output_shm.copy_from_slice(output.as_ref());

                        // Send the processed data size back to the driver as 4 bytes
                        let mut response_size_buffer = [0u8; 4];
                        BigEndian::write_u32(&mut response_size_buffer, output.len() as u32);
                        if let Err(e) = stream.write_all(&response_size_buffer) {
                            println!("Failed to send response to driver: {}", e);
                            break;
                        }
                    }
                    Err(e) => {
                        eprintln!("Error: {}", e);
                        break;
                    }
                }
                let elapsed_time = start_time.elapsed();
                println!("Processing time: {:.2?}", elapsed_time);
            }
            Err(e) => {
                // Print a nice message if the driver disconnects
                if e.kind() == ErrorKind::UnexpectedEof {
                    println!("Driver disconnected");
                } else {
                    println!("Failed to read input size from socket: {}", e);
                }
                break;
            }
        }
    }

    unsafe {
        shmdt(shm_input_addr);
        shmdt(shm_output_addr);
    };

    println!("Goodbye!");
}
