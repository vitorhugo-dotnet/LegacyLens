package io.legacylens.fixture;

import javax.faces.bean.ManagedBean;
import javax.faces.bean.ViewScoped;
import javax.faces.context.FacesContext;
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
    public String getJavaVersion(){return System.getProperty("java.runtime.version","unknown");}
    public String getFacesVersion(){return version(FacesContext.class);}
    public String getPrimeFacesVersion(){return version(primeFacesClass());}
    public String getMysqlVersion(){return service.databaseVersion();}
    public String getConnectorJVersion(){return service.driverVersion();}
    public void save(){service.save(note);message="Order saved";}
    private static String version(Class<?> type){Package pkg=type.getPackage();String value=pkg==null?null:pkg.getImplementationVersion();return value==null||value.trim().isEmpty()?"unknown":value.trim();}
    private static Class<?> primeFacesClass(){
        try{return Class.forName("org.primefaces.PrimeFaces",false,OrderBean.class.getClassLoader());}
        catch(ClassNotFoundException legacy){try{return Class.forName("org.primefaces.context.RequestContext",false,OrderBean.class.getClassLoader());}catch(ClassNotFoundException missing){return OrderBean.class;}}
    }
}
