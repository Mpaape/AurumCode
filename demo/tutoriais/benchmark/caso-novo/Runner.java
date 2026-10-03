import java.io.IOException;

public class Runner {
    Process run(String host) throws IOException {
        return Runtime.getRuntime().exec("ping -c 1 " + host);
    }
}
