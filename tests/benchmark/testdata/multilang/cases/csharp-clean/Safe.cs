using System.Data.SqlClient;

public class Safe {
    public SqlDataReader Find(SqlConnection c, string name) {
        var cmd = c.CreateCommand();
        cmd.CommandText = "SELECT id FROM users WHERE name = @name";
        cmd.Parameters.AddWithValue("@name", name);
        return cmd.ExecuteReader();
    }
}
