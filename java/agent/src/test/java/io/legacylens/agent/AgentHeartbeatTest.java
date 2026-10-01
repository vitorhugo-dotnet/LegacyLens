package io.legacylens.agent;

import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.Test;
import java.net.InetSocketAddress;
import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.TimeUnit;
import static org.junit.jupiter.api.Assertions.*;

class AgentHeartbeatTest {
    @Test void backgroundSenderAuthenticatesHeartbeatWithoutTraceOrSecrets() throws Exception {
        HttpServer server=HttpServer.create(new InetSocketAddress("127.0.0.1",0),0);
        ArrayBlockingQueue<String> seen=new ArrayBlockingQueue<String>(1);
        server.createContext("/v1/events",exchange->{
            ByteArrayOutputStream body=new ByteArrayOutputStream(); byte[] chunk=new byte[1024]; int count;
            while((count=exchange.getRequestBody().read(chunk))!=-1)body.write(chunk,0,count);
            seen.offer(Thread.currentThread().getName()+"\n"+exchange.getRequestHeaders().getFirst("Authorization")+"\n"+new String(body.toByteArray(),StandardCharsets.UTF_8));
            exchange.sendResponseHeaders(200,0);exchange.close();
        });
        server.start();
        try {
            String endpoint="http://127.0.0.1:"+server.getAddress().getPort()+"/v1/events";
            AgentTransport transport=AgentTransport.start(AgentConfig.forTesting("sample.app",endpoint));
            String observation=seen.poll(5,TimeUnit.SECONDS);
            assertNotNull(observation,"background heartbeat did not reach core");
            assertTrue(observation.contains("\"command\":\"agent.heartbeat\""));
            assertTrue(observation.contains("\"producerId\":\""+transport.producerId()+"\""));
            assertFalse(observation.contains("traceId"));
            assertFalse(observation.contains("sql"));
            transport.closeForTesting();
        } finally { server.stop(0); }
    }
}
