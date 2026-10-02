using System.Data.SqlClient;

public class Users {
    public SqlDataReader Find(SqlConnection c, string name) {
        var cmd = c.CreateCommand();
        cmd.CommandText = "SELECT id FROM users WHERE name = '" + name + "'";
        return cmd.ExecuteReader();
    }
}
