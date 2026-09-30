package io.legacylens.agent;
import javax.servlet.fake.FakeServlet;
/** Small entry point used only to smoke-test the shaded -javaagent artifact. */
public final class AgentSmokeMain {
 public static void main(String[] args){try{new FakeServlet().service(new FakeServlet.Request(null),new Object());}catch(Throwable failure){throw new AssertionError("agent smoke application unexpectedly failed",failure);}}
}
