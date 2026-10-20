package request

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCacheDataImmutable(t *testing.T) {
	var body atomic.Value
	body.Store("AAAA")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	defer srv.Close()

	lg := NewLockGeter()
	bs1, err := lg.Get(srv.URL, 100)
	if err != nil || string(bs1) != "AAAA" {
		t.Fatalf("first get: %q %v", bs1, err)
	}

	body.Store("BBBB")
	lg.time = 0
	lg.caches.Range(func(key, value any) bool {
		value.(*cacheItem).time = 0
		return true
	})
	bs2, err := lg.Get(srv.URL, 100)
	if err != nil || string(bs2) != "BBBB" {
		t.Fatalf("second get: %q %v", bs2, err)
	}
	if string(bs1) != "AAAA" {
		t.Fatalf("old cache data changed: %q", bs1)
	}
}
