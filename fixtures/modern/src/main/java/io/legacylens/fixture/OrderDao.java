package io.legacylens.fixture;

import java.io.Serializable;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

public class OrderDao implements Serializable {
    private static final long serialVersionUID = 1L;
    private Connection connect() throws SQLException {
        return DriverManager.getConnection(System.getenv("MODERN_DB_URL"), "modern", System.getenv("MODERN_DB_PASSWORD"));
    }
    public void insert(String note) {
        try (Connection connection = connect(); PreparedStatement statement = connection.prepareStatement("INSERT INTO orders (note) VALUES (?)")) {
            statement.setString(1, note);
            statement.executeUpdate();
        } catch (SQLException failure) { throw new IllegalStateException("Order insert failed", failure); }
    }
    public int count() {
        try (Connection connection = connect(); PreparedStatement statement = connection.prepareStatement("SELECT COUNT(*) FROM orders"); ResultSet rows = statement.executeQuery()) {
            return rows.next() ? rows.getInt(1) : 0;
        } catch (SQLException failure) { throw new IllegalStateException("Order count failed", failure); }
    }
    public String databaseVersion() {
        try (Connection connection = connect(); PreparedStatement statement = connection.prepareStatement("SELECT VERSION()"); ResultSet rows = statement.executeQuery()) {
            return rows.next() ? rows.getString(1) : "unknown";
        } catch (SQLException failure) { throw new IllegalStateException("Database version query failed", failure); }
    }
    public String driverVersion() {
        try (Connection connection = connect()) { return connection.getMetaData().getDriverVersion(); }
        catch (SQLException failure) { throw new IllegalStateException("JDBC driver version query failed", failure); }
    }
}
