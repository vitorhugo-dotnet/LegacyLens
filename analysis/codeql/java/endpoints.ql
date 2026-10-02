/**
 * @name Java Servlet request handlers
 * @description Lists methods recognized by CodeQL as Servlet request handlers, with source locations.
 * @kind table
 * @id legacylens/java-servlet-handlers
 */

import java
import semmle.code.java.frameworks.Servlets

from Method method
where
  isServletRequestMethod(method)
select
  method.getDeclaringType().getName() as className,
  method.getName() as methodName,
  method.getLocation().getFile().getRelativePath() as path,
  method.getLocation().getStartLine() as line,
  method.getLocation().getStartColumn() as column
