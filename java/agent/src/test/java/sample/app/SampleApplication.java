package sample.app;
public class SampleApplication { public void load(String ignored) { new com.mysql.jdbc.Wrapper().executeQuery("SELECT * FROM orders WHERE id = ?"); if(ignored!=null)throw new IllegalStateException(ignored); } }
