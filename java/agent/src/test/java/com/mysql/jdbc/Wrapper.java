package com.mysql.jdbc;

public class Wrapper {
    public void executeQuery(String sql) { new Driver().execute(sql); }

    public void executePrepared() {
        Driver.Prepared statement = new Driver().prepareStatement("SELECT * FROM orders WHERE id = ?");
        statement.setString(1, "parameter-secret");
        statement.executeQuery();
    }
}
