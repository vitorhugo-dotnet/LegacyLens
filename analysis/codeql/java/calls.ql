/**
 * @name Direct Java method calls
 * @description Lists direct method call sites and their enclosing and resolved methods.
 * @kind table
 * @id legacylens/java-direct-calls
 */

import java

from MethodCall call, Method caller, Method callee
where
  caller = call.getEnclosingCallable() and
  callee = call.getMethod()
select
  caller.getName() as callerName,
  caller.getLocation().getFile().getRelativePath() as callerPath,
  caller.getLocation().getStartLine() as callerLine,
  caller.getLocation().getStartColumn() as callerColumn,
  callee.getName() as calleeName,
  callee.getLocation().getFile().getRelativePath() as calleePath,
  callee.getLocation().getStartLine() as calleeLine,
  callee.getLocation().getStartColumn() as calleeColumn,
  call.getLocation().getFile().getRelativePath() as callPath,
  call.getLocation().getStartLine() as callLine,
  call.getLocation().getStartColumn() as callColumn
