package io.legacylens.fixture;

import javax.faces.bean.ManagedBean;
import javax.faces.bean.ViewScoped;
import java.io.Serializable;

@ManagedBean(name="orderBean") @ViewScoped
public class OrderBean implements Serializable {
    private static final long serialVersionUID=1L;
    private String note;
    private String message;
    private final OrderService service=new OrderService();
    public String getNote(){return note;}
    public void setNote(String note){this.note=note;}
    public String getMessage(){return message;}
    public int getOrderCount(){return service.count();}
    public void save(){service.save(note);message="Order saved";}
}
