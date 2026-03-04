package cli

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPassphraseCacheSetGetClear(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "masterpass.cache")
	t.Setenv("ONESSH_CACHE_FILE", cacheFile)

	cache, err := newPassphraseCache("/tmp/config-a.enc", time.Minute, false)
	if err != nil {
		t.Fatalf("newPassphraseCache: %v", err)
	}

	if err := cache.Set([]byte("secret-pass")); err != nil {
		t.Fatalf("cache.Set: %v", err)
	}

	got, ok, err := cache.Get()
	if err != nil {
		t.Fatalf("cache.Get: %v", err)
	}
	if !ok {
		t.Fatalf("expected cached value")
	}
	if string(got) != "secret-pass" {
		t.Fatalf("unexpected cached value: %q", string(got))
	}
	wipe(got)

	if err := cache.Clear(); err != nil {
		t.Fatalf("cache.Clear: %v", err)
	}
	got, ok, err = cache.Get()
	if err != nil {
		t.Fatalf("cache.Get after clear: %v", err)
	}
	if ok || len(got) != 0 {
		t.Fatalf("expected empty cache after clear")
	}
}

func TestPassphraseCacheConfigPathMismatch(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "masterpass.cache")
	t.Setenv("ONESSH_CACHE_FILE", cacheFile)

	cacheA, err := newPassphraseCache("/tmp/config-a.enc", time.Minute, false)
	if err != nil {
		t.Fatalf("newPassphraseCache A: %v", err)
	}
	if err := cacheA.Set([]byte("secret-pass")); err != nil {
		t.Fatalf("cacheA.Set: %v", err)
	}

	cacheB, err := newPassphraseCache("/tmp/config-b.enc", time.Minute, false)
	if err != nil {
		t.Fatalf("newPassphraseCache B: %v", err)
	}

	got, ok, err := cacheB.Get()
	if err != nil {
		t.Fatalf("cacheB.Get: %v", err)
	}
	if ok || len(got) != 0 {
		t.Fatalf("expected cache miss on different config path")
	}
}

func TestPassphraseCacheExpiration(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "masterpass.cache")
	t.Setenv("ONESSH_CACHE_FILE", cacheFile)

	cache, err := newPassphraseCache("/tmp/config-a.enc", time.Second, false)
	if err != nil {
		t.Fatalf("newPassphraseCache: %v", err)
	}
	if err := cache.Set([]byte("secret-pass")); err != nil {
		t.Fatalf("cache.Set: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)

	got, ok, err := cache.Get()
	if err != nil {
		t.Fatalf("cache.Get: %v", err)
	}
	if ok || len(got) != 0 {
		t.Fatalf("expected cache to expire")
	}
}
