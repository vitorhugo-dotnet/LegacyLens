package io.legacylens.agent;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public final class Sanitizer {
    private static final Pattern SQL=Pattern.compile("(?is)^\\s*(SELECT\\s+[a-z0-9_., *()=?]+\\s+FROM\\s+[a-z0-9_.]+(?:\\s+WHERE\\s+[a-z0-9_., =<>!?()]+)?|INSERT\\s+INTO\\s+[a-z0-9_.]+\\s*\\([a-z0-9_, ]+\\)\\s*VALUES\\s*\\([?, ]+\\)|UPDATE\\s+[a-z0-9_.]+\\s+SET\\s+[a-z0-9_]+\\s*=\\s*\\?\\s+WHERE\\s+[a-z0-9_]+\\s*=\\s*\\?)\\s*;?\\s*$");
    private static final Pattern VALUE=Pattern.compile("(?i)(?:=|<>|!=|<|>)\\s*-?[0-9]+(?:\\b|$)|(?:=|<>|!=|<|>)\\s*[a-z_][a-z0-9_.]*|\\b(?:NULL|TRUE|FALSE)\\b|\\b[0-9]+\\b");
    private Sanitizer(){}
    public static String sql(String raw){if(raw==null||raw.length()>512)return "";if(SQL.matcher(raw).matches()&&!VALUE.matcher(raw).find()&&!raw.contains("'")&&!raw.contains("\"")&&!raw.contains("--")&&!raw.contains("/*"))return raw.trim();return "";}
    static Map<String,String> metadata(String keyValues){Map<String,String> m=new LinkedHashMap<String,String>(); if(keyValues==null)return m;return m;}
}
