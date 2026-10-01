package io.legacylens.analyzer;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import net.sf.jsqlparser.parser.CCJSqlParserUtil;
import net.sf.jsqlparser.schema.Column;
import net.sf.jsqlparser.schema.Table;
import net.sf.jsqlparser.statement.Statement;
import net.sf.jsqlparser.statement.Statements;
import net.sf.jsqlparser.statement.update.Update;
import net.sf.jsqlparser.statement.insert.Insert;
import net.sf.jsqlparser.statement.delete.Delete;
import net.sf.jsqlparser.parser.ASTNodeAccess;
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
                List<Integer> starts = statementStarts(source.asText());
                int ordinal = 0;
                for (Statement statement : statements.getStatements()) {
                    int start = ordinal < starts.size() ? starts.get(ordinal) : 0;
                    int end = ordinal + 1 < starts.size() ? starts.get(ordinal + 1) : source.asText().length();
                    analyzeStatement(input, mapper, result, artifact, path, source.asText(), statement, ++ordinal, start, end);
                }
            } catch (Exception error) {
                AnalyzerMain.diagnostic(result, mapper, "sql.parse", "SQL source could not be parsed safely.", path);
            }
        }
        return result;
    }
    private static void analyzeStatement(JsonNode input, ObjectMapper mapper, ObjectNode result, JsonNode artifact, String path, String source, Statement statement, int ordinal, int start, int end) {
                List<Column> columns = new ArrayList<>();
                Map<String, Integer> tableLines = new HashMap<>();
                TablesNamesFinder finder = new TablesNamesFinder() {
                    @Override public void visit(Column column) { columns.add(column); super.visit(column); }
                    @Override public void visit(Table table) {
                        tableLines.putIfAbsent(stripQuotes(table.getName()), nodeLine(table, source, start, end, table.getName()));
                        super.visit(table);
                    }
                };
                List<String> tables = finder.getTableList(statement);
                int queryLine = sourceLine(source, start);
                ObjectNode query = AnalyzerMain.symbol(result, mapper, input, artifact, path + "#query:" + ordinal, "", "query", queryLine);
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
                boolean write = statement instanceof Update || statement instanceof Insert || statement instanceof Delete;
                String tableRelation = write ? "writes" : statement instanceof Select ? "reads" : "depends_on";
                if (!write && !(statement instanceof Select)) AnalyzerMain.diagnostic(result, mapper, "sql.operation_unclassified", "SQL operation direction could not be classified.", path);
                for (String table : tables) {
                    String clean = stripQuotes(table);
                    if (!seen.add("table:" + clean)) continue;
                    int tableLine = tableLines.getOrDefault(clean, tokenLine(source, start, end, clean, queryLine));
                    String target = AnalyzerMain.symbol(result, mapper, input, artifact, clean, "", "table", tableLine).path("id").asText();
                    AnalyzerMain.relation(result, mapper, queryId, target, tableRelation, "resolved", path, tableLine);
                }
                for (Column column : columns) {
                    String table = column.getTable() == null ? "" : stripQuotes(column.getTable().getName());
                    table = aliases.getOrDefault(table, table);
                    String name = table.isEmpty() ? stripQuotes(column.getColumnName()) : table + "." + stripQuotes(column.getColumnName());
                    if (!seen.add("column:" + name)) continue;
                    int columnLine = nodeLine(column, source, start, end, column.getColumnName());
                    String target = AnalyzerMain.symbol(result, mapper, input, artifact, name, "", "column", columnLine).path("id").asText();
                    AnalyzerMain.relation(result, mapper, queryId, target, "uses", table.isEmpty() ? "unresolved" : "resolved", path, columnLine);
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
    private static int nodeLine(ASTNodeAccess node, String source, int start, int end, String token) {
        if (node.getASTNode() != null && node.getASTNode().jjtGetFirstToken() != null) {
            int line = node.getASTNode().jjtGetFirstToken().beginLine;
            if (line > 0) return line;
        }
        return tokenLine(source, start, end, stripQuotes(token), sourceLine(source, start));
    }
    private static int tokenLine(String source, int start, int end, String token, int fallback) {
        if (token == null || token.isEmpty()) return fallback;
        int position = source.indexOf(token, start);
        while (position >= 0 && position < end) {
            int after = position + token.length();
            boolean beforeBoundary = position == 0 || !Character.isJavaIdentifierPart(source.charAt(position - 1));
            boolean afterBoundary = after >= source.length() || !Character.isJavaIdentifierPart(source.charAt(after));
            if (beforeBoundary && afterBoundary) return sourceLine(source, position);
            position = source.indexOf(token, after);
        }
        return fallback;
    }
    private static int sourceLine(String source, int offset) {
        int line = 1;
        for (int i = 0; i < Math.min(offset, source.length()); i++) if (source.charAt(i) == '\n') line++;
        return line;
    }
    private static List<Integer> statementStarts(String source) {
        List<Integer> starts = new ArrayList<>();
        boolean awaiting = true;
        char quote = 0;
        boolean lineComment = false, blockComment = false;
        for (int i = 0; i < source.length(); i++) {
            char c = source.charAt(i), next = i + 1 < source.length() ? source.charAt(i + 1) : 0;
            if (lineComment) { if (c == '\n') lineComment = false; continue; }
            if (blockComment) { if (c == '*' && next == '/') { blockComment = false; i++; } continue; }
            if (quote != 0) {
                if (c == quote) { if (next == quote) i++; else quote = 0; }
                else if (c == '\\' && next != 0) i++;
                continue;
            }
            if (c == '-' && next == '-') { lineComment = true; i++; continue; }
            if (c == '#') { lineComment = true; continue; }
            if (c == '/' && next == '*') { blockComment = true; i++; continue; }
            if (Character.isWhitespace(c)) continue;
            if (c == ';') { awaiting = true; continue; }
            if (awaiting) { starts.add(i); awaiting = false; }
            if (c == '\'' || c == '"' || c == '`') quote = c;
        }
        return starts;
    }
}
