import java.security.MessageDigest;

public class Hasher {
    byte[] hash(String password) throws Exception {
        MessageDigest md = MessageDigest.getInstance("MD5");
        return md.digest(password.getBytes());
    }
}
