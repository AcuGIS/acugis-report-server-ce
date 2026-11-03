package com.acugis.reports;

import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

import jakarta.servlet.http.HttpSession;

public class SessionImageStore {
    private static final String SESSION_KEY = "RR_IMG_STORE";
    private static final long DEFAULT_TTL_MS = 1_800_000L;

    private SessionImageStore() {
    }

    public static String createToken(HttpSession session) {
        String token = UUID.randomUUID().toString();
        ConcurrentHashMap<String, TokenBucket> store = getOrCreateStore(session);
        store.put(token, new TokenBucket());
        return token;
    }

    public static void putImage(HttpSession session, String token, String imageName, byte[] bytes) {
        if (session == null || token == null || imageName == null || bytes == null) {
            return;
        }
        ConcurrentHashMap<String, TokenBucket> store = getOrCreateStore(session);
        TokenBucket bucket = store.computeIfAbsent(token, k -> new TokenBucket());
        bucket.getImages().put(imageName, bytes);
    }

    public static byte[] getImage(HttpSession session, String token, String imageName) {
        if (session == null || token == null || imageName == null) {
            return null;
        }
        ConcurrentHashMap<String, TokenBucket> store = getStore(session);
        if (store == null) {
            return null;
        }
        TokenBucket bucket = store.get(token);
        if (bucket == null) {
            return null;
        }
        return bucket.getImages().get(imageName);
    }

    public static void purgeExpired(HttpSession session) {
        purgeExpired(session, DEFAULT_TTL_MS);
    }

    public static void purgeExpired(HttpSession session, long ttlMillis) {
        ConcurrentHashMap<String, TokenBucket> store = getStore(session);
        if (store == null) {
            return;
        }
        long now = System.currentTimeMillis();
        store.entrySet().removeIf(entry -> now - entry.getValue().getCreatedAt() > ttlMillis);
    }

    @SuppressWarnings("unchecked")
    private static ConcurrentHashMap<String, TokenBucket> getStore(HttpSession session) {
        Object attr = session.getAttribute(SESSION_KEY);
        if (attr instanceof ConcurrentHashMap) {
            return (ConcurrentHashMap<String, TokenBucket>) attr;
        }
        return null;
    }

    private static ConcurrentHashMap<String, TokenBucket> getOrCreateStore(HttpSession session) {
        ConcurrentHashMap<String, TokenBucket> store = getStore(session);
        if (store == null) {
            store = new ConcurrentHashMap<>();
            session.setAttribute(SESSION_KEY, store);
        }
        return store;
    }

    public static final class TokenBucket {
        private final long createdAt = System.currentTimeMillis();
        private final ConcurrentHashMap<String, byte[]> images = new ConcurrentHashMap<>();

        public long getCreatedAt() {
            return createdAt;
        }

        public Map<String, byte[]> getImages() {
            return images;
        }
    }
}
