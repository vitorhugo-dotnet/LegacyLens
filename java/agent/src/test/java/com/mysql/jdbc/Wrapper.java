package com.mysql.jdbc;
public class Wrapper { public void executeQuery(String sql){ new Driver().execute(sql); } }
