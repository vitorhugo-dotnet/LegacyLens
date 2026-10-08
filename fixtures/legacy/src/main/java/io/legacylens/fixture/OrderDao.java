package io.legacylens.fixture;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;

public class OrderDao {
    private Connection connect() throws SQLException {
        return DriverManager.getConnection(System.getenv("LEGACY_DB_URL"), "legacy", System.getenv("LEGACY_DB_PASSWORD"));
    }
    public void insert(String note){
        try(Connection connection=connect();PreparedStatement statement=connection.prepareStatement("INSERT INTO orders (note) VALUES (?)")){
            statement.setString(1,note);statement.executeUpdate();
        }catch(SQLException failure){throw new IllegalStateException("Order insert failed");}
    }
    public int count(){
        try(Connection connection=connect();PreparedStatement statement=connection.prepareStatement("SELECT COUNT(*) FROM orders");ResultSet rows=statement.executeQuery()){
            return rows.next()?rows.getInt(1):0;
        }catch(SQLException failure){throw new IllegalStateException("Order count failed");}
    }
    public String databaseVersion(){
        try(Connection connection=connect();PreparedStatement statement=connection.prepareStatement("SELECT VERSION()");ResultSet rows=statement.executeQuery()){
            return rows.next()?rows.getString(1):"unknown";
        }catch(SQLException failure){throw new IllegalStateException("Database version query failed");}
    }
    public String driverVersion(){
        try(Connection connection=connect()){
            return connection.getMetaData().getDriverVersion();
        }catch(SQLException failure){throw new IllegalStateException("JDBC driver version query failed");}
    }
}
