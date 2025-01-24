import com.sun.jna.Library;
import com.sun.jna.Native;
import com.sun.jna.Pointer;
import java.io.IOException;
import java.net.UnixDomainSocketAddress;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.channels.SocketChannel;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.concurrent.atomic.AtomicBoolean;

public class Main {
    private static byte[] processInput(String method, byte[] input)
            throws Exception, NoSuchAlgorithmException {
        switch (method) {
            case "sha":
                MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
                return sha256.digest(input);
            default:
                throw new Exception("Unknown method: '%s'");
        }
    }

    public interface CLib extends Library {
        CLib INSTANCE = Native.load("c", CLib.class);
        Pointer shmat(int shmid, Pointer shmaddr, int shmflg);
        int shmdt(Pointer shmaddr);
    }

    private static SocketChannel connectToUnixSocket(String socketName) throws IOException {
        UnixDomainSocketAddress address = UnixDomainSocketAddress.of(socketName);
        return SocketChannel.open(address);
    }

    private static String getStrFromDriver(SocketChannel socketChannel) throws IOException {
        ByteBuffer buffer = ByteBuffer.allocate(64);
        int methodLength = socketChannel.read(buffer);
        if (methodLength <= 0) {
            throw new IOException("No data read from socket or end of stream.");
        }
        byte[] validBytes = new byte[methodLength];
        buffer.flip();
        buffer.get(validBytes, 0, methodLength);
        return new String(validBytes, StandardCharsets.UTF_8).replace("\0", "");
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

    private static record SharedMemory(int shmId, Pointer shmAddr) {}

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

        try (SocketChannel socketChannel = connectToUnixSocket("/tmp/eth-cl-fuzz")) {
            // Send client name
            ByteBuffer nameBuffer = ByteBuffer.wrap("java".getBytes());
            socketChannel.write(nameBuffer);

            // Attach to input shared memory
            int inputShmId = getIntFromDriver(socketChannel);
            final SharedMemory inputShm = attachSharedMemory(inputShmId);

            // Attach to output shared memory
            int outputShmId = getIntFromDriver(socketChannel);
            final SharedMemory outputShm = attachSharedMemory(outputShmId);

            // Set up Ctrl+C handling
            Runtime.getRuntime().addShutdownHook(new Thread(() -> {
                running.set(false);
                CLib.INSTANCE.shmdt(inputShm.shmAddr());
                CLib.INSTANCE.shmdt(outputShm.shmAddr());
                System.out.println("Goodbye!");
            }));

            // Get method name
            String method = getStrFromDriver(socketChannel);
            System.out.println("Fuzzing method: " + method);

            System.out.println("Running... Press Ctrl+C to exit");
            while (running.get()) {
                // Read the the input
                int inputSize = 0;
                try {
                    inputSize = getIntFromDriver(socketChannel);
                } catch (java.nio.BufferUnderflowException e) {
                    System.out.println("Driver disconnected");
                    break;
                }
                byte[] input = new byte[inputSize];
                inputShm.shmAddr().getByteBuffer(0, inputSize).get(input);

                // Process the input
                long startTime = System.nanoTime();
                byte[] result = processInput(method, input);

                // Write result to output buffer
                outputShm.shmAddr().getByteBuffer(0, result.length).put(result);
                long endTime = System.nanoTime();
                long duration = endTime - startTime;
                System.out.printf("Processing time: %.2fms%n", duration / 1_000_000.0);

                // Send response size back
                sendIntToDriver(socketChannel, result.length);
            }
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}