using System.Security.Cryptography;

public class Hasher {
    public byte[] Hash(byte[] password) {
        using var md5 = MD5.Create();
        return md5.ComputeHash(password);
    }
}
