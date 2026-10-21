package simulaciones

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(3)
	ahora := time.Unix(1000, 0)
	limiter.ahora = func() time.Time { return ahora }

	for i := 0; i < 3; i++ {
		if !limiter.Allow("1.2.3.4") {
			t.Fatalf("petición %d debería permitirse", i+1)
		}
	}
	if limiter.Allow("1.2.3.4") {
		t.Fatal("la cuarta petición debería rechazarse")
	}
	if !limiter.Allow("5.6.7.8") {
		t.Fatal("otra IP no debería estar limitada")
	}

	ahora = ahora.Add(60 * time.Second)
	if !limiter.Allow("1.2.3.4") {
		t.Fatal("tras un minuto debería reponerse el cubo")
	}
}

func TestRateLimiterLimpiaIPs(t *testing.T) {
	limiter := NewRateLimiter(2)
	ahora := time.Unix(1000, 0)
	limiter.ahora = func() time.Time { return ahora }
	limiter.Allow("1.1.1.1")

	ahora = ahora.Add(11 * time.Minute)
	limiter.Allow("2.2.2.2")
	limiter.mu.Lock()
	_, existe := limiter.buckets["1.1.1.1"]
	limiter.mu.Unlock()
	if existe {
		t.Fatal("la IP inactiva debería haberse limpiado")
	}
}

func TestClientIP(t *testing.T) {
	confiables := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.2")
	if got := ClientIP(req, confiables); got != "203.0.113.9" {
		t.Fatalf("IP detrás de proxy: %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "198.51.100.5:1234"
	req2.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := ClientIP(req2, confiables); got != "198.51.100.5" {
		t.Fatalf("no se debe confiar en XFF de una IP no confiable: %q", got)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "[2001:db8::1]:1234"
	if got := ClientIP(req3, confiables); got != "2001:db8::1" {
		t.Fatalf("IPv6: %q", got)
	}
}
