package io.legacylens.fixture;

import javax.servlet.ServletException;
import javax.servlet.http.HttpServlet;
import javax.servlet.http.HttpServletRequest;
import javax.servlet.http.HttpServletResponse;
import java.io.IOException;

/** Minimal servlet endpoint connecting HTTP fetches to the existing service fixture. */
public final class OrderServlet extends HttpServlet {
    private static final long serialVersionUID = 1L;
    private final OrderService service = new OrderService();

    @Override protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        response.setContentType("text/plain");
        response.getWriter().write(String.valueOf(service.count()));
    }

    @Override protected void doPost(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {
        service.save(request.getParameter("note"));
        response.setStatus(HttpServletResponse.SC_NO_CONTENT);
    }
}
