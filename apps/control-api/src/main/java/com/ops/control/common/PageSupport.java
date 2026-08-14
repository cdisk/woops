package com.ops.control.common;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/** Shared paging helpers for list APIs returning {@code {items,total,page,pageSize}}. */
public final class PageSupport {
    private PageSupport() {}

    public static int clampPageSize(int pageSize) {
        return Math.min(Math.max(pageSize, 1), 200);
    }

    public static Map<String, Object> emptyPage(int page, int pageSize) {
        return pageResult(List.of(), 0, page, pageSize);
    }

    public static Map<String, Object> pageResult(List<?> items, long total, int page, int pageSize) {
        Map<String, Object> out = new LinkedHashMap<>();
        out.put("items", items == null ? List.of() : items);
        out.put("total", total);
        out.put("page", Math.max(page, 1));
        out.put("pageSize", pageSize);
        return out;
    }
}
