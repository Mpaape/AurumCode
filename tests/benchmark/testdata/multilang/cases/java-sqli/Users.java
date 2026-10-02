import java.sql.*;

public class Users {
    ResultSet find(Connection c, String name) throws SQLException {
        Statement s = c.createStatement();
        return s.executeQuery("SELECT id FROM users WHERE name = '" + name + "'");
    }
}
