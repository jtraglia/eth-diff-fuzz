import com.sun.jna.Library;
import com.sun.jna.Native;
import com.sun.jna.Pointer;
import java.io.IOException;
import java.net.UnixDomainSocketAddress;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.channels.SocketChannel;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.concurrent.atomic.AtomicBoolean;

public class Main {
    private static final String SOCKET_NAME = "/tmp/eth-cl-fuzz";
    private static final int SHM_INPUT_KEY = 0;
    private static final int SHM_MAX_SIZE = 100 * 1024 * 1024; // 100 MiB

    public interface CLib extends Library {
        CLib INSTANCE = Native.load("c", CLib.class);

        int shmget(int key, int size, int shmflg);
        Pointer shmat(int shmid, Pointer shmaddr, int shmflg);
        int shmdt(Pointer shmaddr);
        int shmctl(int shmid, int cmd, Pointer buf);
    }

    private static class SharedMemory {
        private final int shmId;
        private final Pointer shmAddr;

        public SharedMemory(int shmId, Pointer shmAddr) {
            this.shmId = shmId;
            this.shmAddr = shmAddr;
        }

        public int getShmId() {
            return shmId;
        }

        public Pointer getShmAddr() {
            return shmAddr;
        }
    }

    private static SharedMemory input;
    private static SharedMemory output;

    public static void main(String[] args) {
        System.out.println("Connecting to driver...");
        AtomicBoolean running = new AtomicBoolean(true);

        // Set up Ctrl+C handling
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            running.set(false);
            cleanupSharedMemory(); // Ensure shared memory is cleaned up on shutdown
        }));

        try (SocketChannel socketChannel = connectToUnixSocket(SOCKET_NAME)) {
            // Send client name
            ByteBuffer nameBuffer = ByteBuffer.wrap("java".getBytes());
            socketChannel.write(nameBuffer);

            // Attach to input shared memory
            input = attachSharedMemory(SHM_INPUT_KEY);

            // Receive the output shared memory key
            int shmOutputKey = receiveOutputKey(socketChannel);

            // Attach to output shared memory
            output = attachSharedMemory(shmOutputKey);

            System.out.println("Running... Press Ctrl+C to exit");
            while (running.get()) {
                boolean shouldBreak = processFuzzingLoop(
                    socketChannel, input.getShmAddr().getByteBuffer(0, SHM_MAX_SIZE),
                    output.getShmAddr().getByteBuffer(0, SHM_MAX_SIZE));
                if (shouldBreak) {
                    break;
                }
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private static SocketChannel connectToUnixSocket(String socketName) throws IOException {
        UnixDomainSocketAddress address = UnixDomainSocketAddress.of(socketName);
        return SocketChannel.open(address);
    }

    private static int receiveOutputKey(SocketChannel socketChannel) throws IOException {
        ByteBuffer buffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN);
        socketChannel.read(buffer);
        buffer.flip();
        return buffer.getInt();
    }

    private static boolean processFuzzingLoop(SocketChannel socketChannel,
                                           ByteBuffer shmInputBuffer, ByteBuffer shmOutputBuffer)
            throws IOException, NoSuchAlgorithmException {

        // Read the size of the input data
        ByteBuffer sizeBuffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN);
        socketChannel.read(sizeBuffer);
        sizeBuffer.flip();

        int inputSize = 0;

        try {
            inputSize = sizeBuffer.getInt();
        } catch (java.nio.BufferUnderflowException e) {
            System.out.println("Driver disconnected");
            return true;
        }

        // Check if shared memory has enough data
        if (shmInputBuffer.remaining() < inputSize) {
            throw new IOException("Shared memory buffer does not contain enough data");
        }

        // Reset buffer position and read input
        shmInputBuffer.position(0);
        byte[] input = new byte[inputSize];
        shmInputBuffer.get(input);

        // Process the input
        long startTime = System.nanoTime();
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        byte[] result = sha256.digest(input);
        shmOutputBuffer.position(0);
        shmOutputBuffer.put(result);
        long endTime = System.nanoTime();
        long duration = endTime - startTime;
        System.out.printf("Processing time: %.2fms%n", duration / 1_000_000.0);

        // Send response size back
        ByteBuffer responseBuffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN).putInt(result.length);
        responseBuffer.flip();
        socketChannel.write(responseBuffer);
        return false;
    }

    private static SharedMemory attachSharedMemory(int shmKey) throws IOException {
        int shmId = CLib.INSTANCE.shmget(shmKey, SHM_MAX_SIZE, 0666);
        if (shmId == -1) {
            throw new IOException("Failed to create shared memory segment: shmget returned -1");
        }

        Pointer shmAddr = CLib.INSTANCE.shmat(shmId, null, 0);
        if (Pointer.nativeValue(shmAddr) == -1) {
            throw new IOException("Failed to attach to shared memory segment: shmat returned -1");
        }

        return new SharedMemory(shmId, shmAddr);
    }

    private static void cleanupSharedMemory() {
        try {
            if (input.getShmAddr() != null) {
                CLib.INSTANCE.shmdt(input.getShmAddr());
                CLib.INSTANCE.shmctl(input.getShmId(), 0, null); // 0 = IPC_RMID
            }
            if (output.getShmAddr() != null) {
                CLib.INSTANCE.shmdt(output.getShmAddr());
                CLib.INSTANCE.shmctl(output.getShmId(), 0, null); // 0 = IPC_RMID
            }
        } catch (Exception e) {
            System.err.println("Failed to cleanup shared memory: " + e.getMessage());
        }
        System.out.println("Goodbye!");
    }
}