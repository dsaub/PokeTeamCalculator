package simulaciones

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// ClientIP obtiene la IP del cliente. Solo confía en X-Forwarded-For cuando
// la conexión viene de un proxy de PROXIES_CONFIABLES.
func ClientIP(r *http.Request, proxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	addr = addr.Unmap()

	if esProxyConfiable(addr, proxies) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			primera := strings.TrimSpace(strings.Split(xff, ",")[0])
			if a, err := netip.ParseAddr(primera); err == nil {
				return a.Unmap().String()
			}
		}
	}
	return addr.String()
}

func esProxyConfiable(addr netip.Addr, proxies []netip.Prefix) bool {
	for _, p := range proxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

// RateLimiter es un token bucket por IP, en memoria.
type RateLimiter struct {
	mu             sync.Mutex
	buckets        map[string]*rateBucket
	capacidad      float64
	recarga        float64 // tokens por segundo
	ahora          func() time.Time
	ultimaLimpieza time.Time
}

func NewRateLimiter(porMinuto int) *RateLimiter {
	if porMinuto < 1 {
		porMinuto = 1
	}
	return &RateLimiter{
		buckets:   make(map[string]*rateBucket),
		capacidad: float64(porMinuto),
		recarga:   float64(porMinuto) / 60,
		ahora:     time.Now,
	}
}

func (l *RateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	ahora := l.ahora()
	if l.ultimaLimpieza.IsZero() || ahora.Sub(l.ultimaLimpieza) >= 5*time.Minute {
		l.limpiarLocked(ahora)
		l.ultimaLimpieza = ahora
	}

	b, ok := l.buckets[ip]
	if !ok {
		b = &rateBucket{tokens: l.capacidad, last: ahora}
		l.buckets[ip] = b
	} else {
		transcurrido := ahora.Sub(b.last).Seconds()
		if transcurrido > 0 {
			b.tokens += transcurrido * l.recarga
			if b.tokens > l.capacidad {
				b.tokens = l.capacidad
			}
			b.last = ahora
		}
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *RateLimiter) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			l.limpiarLocked(l.ahora())
			l.mu.Unlock()
		}
	}
}

func (l *RateLimiter) limpiarLocked(ahora time.Time) {
	for ip, b := range l.buckets {
		if ahora.Sub(b.last) >= 10*time.Minute {
			delete(l.buckets, ip)
		}
	}
}
