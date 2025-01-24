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

    public interface CLib extends Library {
        CLib INSTANCE = Native.load("c", CLib.class);
        Pointer shmat(int shmid, Pointer shmaddr, int shmflg);
        int shmdt(Pointer shmaddr);
    }

    private static record SharedMemory(int shmId, Pointer shmAddr) {}
    private static SharedMemory input;
    private static SharedMemory output;

    private static SocketChannel connectToUnixSocket(String socketName) throws IOException {
        UnixDomainSocketAddress address = UnixDomainSocketAddress.of(socketName);
        return SocketChannel.open(address);
    }

    private static int getIntFromDriver(SocketChannel socketChannel) throws IOException {
        ByteBuffer buffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN);
        socketChannel.read(buffer);
        buffer.flip();
        return buffer.getInt();
    }

    private static void sendIntToDriver(SocketChannel socketChannel, int value) throws IOException {
        ByteBuffer responseBuffer = ByteBuffer.allocate(4).order(ByteOrder.BIG_ENDIAN).putInt(value);
        responseBuffer.flip();
        socketChannel.write(responseBuffer);
    }

    private static SharedMemory attachSharedMemory(int shmId) throws IOException {
        Pointer shmAddr = CLib.INSTANCE.shmat(shmId, null, 0);
        if (Pointer.nativeValue(shmAddr) == -1) {
            throw new IOException("Failed to attach to shared memory segment");
        }
        return new SharedMemory(shmId, shmAddr);
    }

    public static void main(String[] args) {
        System.out.println("Connecting to driver...");
        AtomicBoolean running = new AtomicBoolean(true);

        // Set up Ctrl+C handling
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            running.set(false);
            CLib.INSTANCE.shmdt(input.shmAddr());
            CLib.INSTANCE.shmdt(output.shmAddr());
            System.out.println("Goodbye!");
        }));

        try (SocketChannel socketChannel = connectToUnixSocket(SOCKET_NAME)) {
            // Send client name
            ByteBuffer nameBuffer = ByteBuffer.wrap("java".getBytes());
            socketChannel.write(nameBuffer);

            // Attach to input shared memory
            int inputShmId = getIntFromDriver(socketChannel);
            input = attachSharedMemory(inputShmId);

            // Attach to output shared memory
            int outputShmId = getIntFromDriver(socketChannel);
            output = attachSharedMemory(outputShmId);

            System.out.println("Running... Press Ctrl+C to exit");
            while (running.get()) {
                boolean shouldBreak = processFuzzingLoop(socketChannel, input, output);
                if (shouldBreak) {
                    break;
                }
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
    }

    private static boolean processFuzzingLoop(SocketChannel socketChannel,
            SharedMemory inputShm, SharedMemory outputShm)
            throws IOException, NoSuchAlgorithmException {
        // Read the the input
        int inputSize = 0;
        try {
            inputSize = getIntFromDriver(socketChannel);
        } catch (java.nio.BufferUnderflowException e) {
            System.out.println("Driver disconnected");
            return true;
        }
        byte[] input = new byte[inputSize];
        inputShm.shmAddr().getByteBuffer(0, inputSize).get(input);

        // Process the input
        long startTime = System.nanoTime();
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        byte[] result = sha256.digest(input);
        outputShm.shmAddr().getByteBuffer(0, result.length).put(result);
        long endTime = System.nanoTime();
        long duration = endTime - startTime;
        System.out.printf("Processing time: %.2fms%n", duration / 1_000_000.0);

        // Send response size back
        sendIntToDriver(socketChannel, result.length);
        return false;
    }
}