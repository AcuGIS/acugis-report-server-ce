package com.acugis.reports;

import java.util.concurrent.ConcurrentHashMap;

public final class ReportImageStore {
    public static final ConcurrentHashMap<String, ConcurrentHashMap<String, byte[]>> IMAGES =
            new ConcurrentHashMap<>();

    private ReportImageStore() {
    }

    public static void put(String uuid, String imageName, byte[] bytes) {
        IMAGES.computeIfAbsent(uuid, k -> new ConcurrentHashMap<>()).put(imageName, bytes);
    }

    public static byte[] get(String uuid, String imageName) {
        ConcurrentHashMap<String, byte[]> m = IMAGES.get(uuid);
        return m == null ? null : m.get(imageName);
    }

    public static int countFor(String uuid) {
        ConcurrentHashMap<String, byte[]> m = IMAGES.get(uuid);
        return m == null ? 0 : m.size();
    }
}
