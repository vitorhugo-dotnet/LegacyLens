package io.legacylens.analyzer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import net.sf.jsqlparser.parser.CCJSqlParserUtil;
import net.sf.jsqlparser.schema.Column;
import net.sf.jsqlparser.schema.Table;
import net.sf.jsqlparser.statement.Statement;
import net.sf.jsqlparser.statement.Statements;
import net.sf.jsqlparser.statement.select.FromItem;
import net.sf.jsqlparser.statement.select.Join;
import net.sf.jsqlparser.statement.select.PlainSelect;
import net.sf.jsqlparser.statement.select.Select;
import net.sf.jsqlparser.util.TablesNamesFinder;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/** SQL dependencies come from the parsed statement tree, never from text matching. */
public final class SqlAnalyzer {
    public ObjectNode analyze(JsonNode input, ObjectMapper mapper) {
        ObjectNode result = AnalyzerMain.empty(mapper);
        for (JsonNode artifact : input.path("artifacts")) {
            if (!"sql".equalsIgnoreCase(artifact.path("language").asText())) continue;
            String path = artifact.path("path").asText();
            JsonNode source = input.path("sources").get(path);
            if (source == null || !source.isTextual()) continue;
            try {
                Statements statements = CCJSqlParserUtil.parseStatements(source.asText());
                int ordinal = 0;
                for (Statement statement : statements.getStatements()) analyzeStatement(input, mapper, result, artifact, path, statement, ++ordinal);
            } catch (Exception error) {
                AnalyzerMain.diagnostic(result, mapper, "sql.parse", "SQL source could not be parsed safely.", path);
            }
        }
        return result;
    }
    private static void analyzeStatement(JsonNode input, ObjectMapper mapper, ObjectNode result, JsonNode artifact, String path, Statement statement, int ordinal) {
                List<Column> columns = new ArrayList<>();
                TablesNamesFinder finder = new TablesNamesFinder() {
                    @Override public void visit(Column column) { columns.add(column); super.visit(column); }
                };
                List<String> tables = finder.getTableList(statement);
                ObjectNode query = AnalyzerMain.symbol(result, mapper, input, artifact, path + "#query:" + ordinal, "", "query", 1);
                String queryId = query.path("id").asText();
                Map<String, String> aliases = new HashMap<>();
                if (statement instanceof Select) {
                    PlainSelect plain = ((Select) statement).getPlainSelect();
                    if (plain != null) {
                        addAlias(aliases, plain.getFromItem());
                        if (plain.getJoins() != null) for (Join join : plain.getJoins()) addAlias(aliases, join.getRightItem());
                    }
                }
                Set<String> seen = new HashSet<>();
                for (String table : tables) {
                    String clean = stripQuotes(table);
                    if (!seen.add("table:" + clean)) continue;
                    String target = AnalyzerMain.symbol(result, mapper, input, artifact, clean, "", "table", 1).path("id").asText();
                    AnalyzerMain.relation(result, mapper, queryId, target, "reads", "resolved", path, 1);
                }
                for (Column column : columns) {
                    String table = column.getTable() == null ? "" : stripQuotes(column.getTable().getName());
                    table = aliases.getOrDefault(table, table);
                    String name = table.isEmpty() ? stripQuotes(column.getColumnName()) : table + "." + stripQuotes(column.getColumnName());
                    if (!seen.add("column:" + name)) continue;
                    String target = AnalyzerMain.symbol(result, mapper, input, artifact, name, "", "column", 1).path("id").asText();
                    AnalyzerMain.relation(result, mapper, queryId, target, "uses", table.isEmpty() ? "unresolved" : "resolved", path, 1);
                }
    }
    private static void addAlias(Map<String, String> aliases, FromItem item) {
        if (item instanceof Table) {
            Table table = (Table) item;
            if (table.getAlias() != null) aliases.put(stripQuotes(table.getAlias().getName()), stripQuotes(table.getName()));
        }
    }
    private static String stripQuotes(String value) {
        if (value == null) return "";
        if (value.length() > 1 && ((value.startsWith("`") && value.endsWith("`")) || (value.startsWith("\"") && value.endsWith("\"")))) return value.substring(1, value.length() - 1);
        return value;
    }
}
