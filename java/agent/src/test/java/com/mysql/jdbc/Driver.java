package com.mysql.jdbc;

public class Driver {
    public void execute(String sql) { }

    public Prepared prepareStatement(String sql) { return new Prepared(sql); }

    public static final class Prepared {
        private final String sql;
        Prepared(String sql) { this.sql = sql; }
        public void setString(int index, String value) { }
        public void executeQuery() { new Driver().execute(sql); }
        public boolean execute() { new Driver().execute(sql); return true; }
        public int executeUpdate() { new Driver().execute(sql); return 1; }
    }
}
