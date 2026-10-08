package io.legacylens.fixture;

import java.io.Serializable;

public class OrderService implements Serializable {
    private static final long serialVersionUID = 1L;
    private final OrderDao dao = new OrderDao();
    public void save(String note) { if (note == null || note.trim().isEmpty()) throw new IllegalArgumentException("note required"); dao.insert(note); }
    public int count() { return dao.count(); }
    public String databaseVersion() { return dao.databaseVersion(); }
    public String driverVersion() { return dao.driverVersion(); }
}
