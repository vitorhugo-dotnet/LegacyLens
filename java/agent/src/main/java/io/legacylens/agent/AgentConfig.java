package io.legacylens.agent;

import java.io.FileInputStream;
import java.io.IOException;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.Properties;
import java.util.Set;

public final class AgentConfig {
    public final String endpoint, token, projectId, revision, producerId;
    public final Set<String> packages;
    private AgentConfig(String e,String t,String p,String r,String producer,Set<String> a){endpoint=e;token=t;projectId=p;revision=r;producerId=producer;packages=a;}
    public static AgentConfig load(String options) throws IOException {
        if(options==null || !options.startsWith("config=")) throw new IOException("agent requires config=<path>");
        Properties p=new Properties(); try(FileInputStream in=new FileInputStream(options.substring(7))){p.load(in);}
        String endpoint=required(p,"endpoint"), token=required(p,"token"), project=required(p,"projectId"), producer=required(p,"producerId");
        if(!endpoint.matches("http://(127\\.0\\.0\\.1|localhost):[0-9]{1,5}/v1/events")) throw new IOException("endpoint must be local /v1/events");
        int port=Integer.parseInt(endpoint.replaceFirst("^http://[^:]+:","").replaceFirst("/v1/events$",""));if(port<1||port>65535)throw new IOException("endpoint port is invalid");
        if(!token.matches("[A-Za-z0-9._~-]{16,512}"))throw new IOException("agent token format is invalid");
        String raw=required(p,"packages"); Set<String> packages=new HashSet<String>(); for(String item:raw.split(",")){String x=item.trim();if(x.matches("[A-Za-z_$][A-Za-z0-9_$.]*"))packages.add(x);}
        if(packages.isEmpty())throw new IOException("at least one application package is required");
        return new AgentConfig(endpoint,token,project,p.getProperty("revision"),producer,Collections.unmodifiableSet(packages));
    }
    private static String required(Properties p,String key)throws IOException{String v=p.getProperty(key);if(v==null||v.trim().isEmpty())throw new IOException("missing agent config: "+key);return v.trim();}
    static AgentConfig forTesting(String pkg){return new AgentConfig("http://127.0.0.1:1/v1/events","test","p1",null,"producer-test",new HashSet<String>(Arrays.asList(pkg)));}
    static AgentConfig forTesting(String pkg,String endpoint){return new AgentConfig(endpoint,"0123456789abcdef0123456789abcdef","p1",null,"producer-test",new HashSet<String>(Arrays.asList(pkg)));}
    static String deploymentIdentity(String label,ClassLoader loader){return label+"@loader-"+Integer.toHexString(System.identityHashCode(loader));}
}
