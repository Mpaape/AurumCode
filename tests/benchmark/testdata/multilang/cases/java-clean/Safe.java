import java.sql.*;

public class Safe {
    ResultSet find(Connection c, String name) throws SQLException {
        PreparedStatement s = c.prepareStatement("SELECT id FROM users WHERE name = ?");
        s.setString(1, name);
        return s.executeQuery();
    }
}
