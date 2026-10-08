package io.legacylens.fixture;

public class OrderService {
    private final OrderDao dao=new OrderDao();
    public void save(String note){if(note==null||note.trim().isEmpty())throw new IllegalArgumentException("note required");dao.insert(note);}
    public int count(){return dao.count();}
    public String databaseVersion(){return dao.databaseVersion();}
    public String driverVersion(){return dao.driverVersion();}
}
