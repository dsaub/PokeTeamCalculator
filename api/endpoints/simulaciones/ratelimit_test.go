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

func TestRateLimiterMinimoUno(t *testing.T) {
	l := NewRateLimiter(0)
	if !l.Allow("1.1.1.1") || l.Allow("1.1.1.1") {
		t.Fatal("con 0 por minuto debería permitir una")
	}
}

func TestRateLimiterNoSuperaLaCapacidad(t *testing.T) {
	l := NewRateLimiter(2)
	ahora := time.Unix(1000, 0)
	l.ahora = func() time.Time { return ahora }
	l.Allow("1.1.1.1")
	ahora = ahora.Add(time.Hour)
	permitidas := 0
	for i := 0; i < 5; i++ {
		if l.Allow("1.1.1.1") {
			permitidas++
		}
	}
	if permitidas != 2 {
		t.Fatalf("tras una hora se permiten %d, esperadas 2", permitidas)
	}
	// Un reloj que retrocede no regala tokens.
	ahora = ahora.Add(-time.Minute)
	if l.Allow("1.1.1.1") {
		t.Fatal("no debería haber tokens")
	}
}

func TestRateLimiterRunTerminaConElContexto(t *testing.T) {
	l := NewRateLimiter(1)
	terminaPronto(t, "RateLimiter.Run", func() { l.Run(ctxCancelado()) })
}

func TestClientIPCasosLimite(t *testing.T) {
	confiables := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	casos := []struct {
		nombre, remoto, xff, ip string
	}{
		{"sin puerto", "198.51.100.5", "", "198.51.100.5"},
		{"no es una IP", "socket-unix", "", "socket-unix"},
		{"IPv4 mapeada en IPv6", "[::ffff:10.0.0.1]:80", "203.0.113.9", "203.0.113.9"},
		{"XFF inválido desde proxy", "10.0.0.1:80", "basura", "10.0.0.1"},
		{"XFF vacío desde proxy", "10.0.0.1:80", "", "10.0.0.1"},
		{"XFF mapeado", "10.0.0.1:80", "::ffff:203.0.113.9", "203.0.113.9"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = caso.remoto
			if caso.xff != "" {
				req.Header.Set("X-Forwarded-For", caso.xff)
			}
			if got := ClientIP(req, confiables); got != caso.ip {
				t.Fatalf("%q, esperada %q", got, caso.ip)
			}
		})
	}
}
