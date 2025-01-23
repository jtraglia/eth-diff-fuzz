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

public class App {
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

    private static Pointer shmInputAddr;
    private static int shmInputId;
    private static Pointer shmOutputAddr;
    private static int shmOutputId;

    public static void main(String[] args) {
        System.out.println("Connecting to driver...");
        AtomicBoolean running = new AtomicBoolean(true);

        // Set up Ctrl+C handling
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            System.out.println("\nCtrl+C detected! Cleaning up...");
            running.set(false);
            cleanupSharedMemory(); // Ensure shared memory is cleaned up on shutdown
        }));

        try (SocketChannel socketChannel = connectToUnixSocket(SOCKET_NAME)) {
            // Send client name
            ByteBuffer nameBuffer = ByteBuffer.wrap("java".getBytes());
            socketChannel.write(nameBuffer);

            // Attach to input shared memory
            shmInputAddr = attachSharedMemory(SHM_INPUT_KEY, SHM_MAX_SIZE, false);
            shmInputId = getSharedMemoryId(SHM_INPUT_KEY, SHM_MAX_SIZE, false);

            // Receive the output shared memory key
            int shmOutputKey = receiveOutputKey(socketChannel);

            // Attach to output shared memory
            shmOutputAddr = attachSharedMemory(shmOutputKey, SHM_MAX_SIZE, true);
            shmOutputId = getSharedMemoryId(shmOutputKey, SHM_MAX_SIZE, true);

            System.out.println("Running... Press Ctrl+C to exit");

            while (running.get()) {
                processFuzzingLoop(socketChannel, shmInputAddr.getByteBuffer(0, SHM_MAX_SIZE),
                                   shmOutputAddr.getByteBuffer(0, SHM_MAX_SIZE));
            }

        } catch (Exception e) {
            e.printStackTrace();
        } finally {
            cleanupSharedMemory(); // Ensure cleanup happens in case of exceptions
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

    private static void processFuzzingLoop(SocketChannel socketChannel,
                                           ByteBuffer shmInputBuffer, ByteBuffer shmOutputBuffer)
            throws IOException, NoSuchAlgorithmException {

        // Read the size of the input data
        ByteBuffer sizeBuffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN);
        socketChannel.read(sizeBuffer);
        sizeBuffer.flip();
        int inputSize = sizeBuffer.getInt();

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
    }

    private static Pointer attachSharedMemory(int shmKey, int size, boolean readOnly) throws IOException {
        int flags = readOnly ? 0666 : (0666 | 01000); // 01000 = IPC_CREAT
        int shmId = CLib.INSTANCE.shmget(shmKey, size, flags);
        if (shmId == -1) {
            throw new IOException("Failed to create shared memory segment: shmget returned -1");
        }

        Pointer shmAddr = CLib.INSTANCE.shmat(shmId, null, 0);
        if (Pointer.nativeValue(shmAddr) == -1) {
            throw new IOException("Failed to attach to shared memory segment: shmat returned -1");
        }

        return shmAddr;
    }

    private static int getSharedMemoryId(int shmKey, int size, boolean readOnly) throws IOException {
        int flags = readOnly ? 0666 : (0666 | 01000); // 01000 = IPC_CREAT
        int shmId = CLib.INSTANCE.shmget(shmKey, size, flags);
        if (shmId == -1) {
            throw new IOException("Failed to create shared memory segment: shmget returned -1");
        }
        return shmId;
    }

    private static void cleanupSharedMemory() {
        try {
            if (shmInputAddr != null) {
                CLib.INSTANCE.shmdt(shmInputAddr);
                CLib.INSTANCE.shmctl(shmInputId, 0, null); // 0 = IPC_RMID
                System.out.println("Cleaned up input shared memory");
            }
            if (shmOutputAddr != null) {
                CLib.INSTANCE.shmdt(shmOutputAddr);
                CLib.INSTANCE.shmctl(shmOutputId, 0, null); // 0 = IPC_RMID
                System.out.println("Cleaned up output shared memory");
            }
        } catch (Exception e) {
            System.err.println("Failed to cleanup shared memory: " + e.getMessage());
        }
    }
}