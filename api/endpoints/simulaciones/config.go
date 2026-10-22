package simulaciones

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config contiene la configuración propia de simulaciones. La conexión a
// MariaDB se reutiliza de database.Initialize (DB_*), así que aquí no hay DSN.
type Config struct {
	URLRedis                string
	LatidoCaducadoWorker    time.Duration
	SecretoCookie           []byte
	Simulaciones            int
	AleatoriedadRival       float64
	TrabajosSimultaneos     int
	MaxPendientesPorUsuario int
	MaxCola                 int
	CacheTTL                time.Duration
	Retencion               time.Duration
	CancelarSinCliente      time.Duration
	IntervaloSondeoSSE      time.Duration
	LimitePorMinuto         int
	ProxiesConf             []netip.Prefix
}

func LoadConfig() (Config, error) {
	cfg := Config{URLRedis: strings.TrimSpace(os.Getenv("REDIS_URL"))}

	if cfg.URLRedis == "" {
		return Config{}, fmt.Errorf("REDIS_URL es obligatoria (por ejemplo redis://:clave@127.0.0.1:6379/0)")
	}
	if _, err := redis.ParseURL(cfg.URLRedis); err != nil {
		return Config{}, fmt.Errorf("REDIS_URL debe ser una URL redis:// o rediss:// válida")
	}
	secreto := os.Getenv("SECRETO_COOKIE")
	if len(secreto) < 32 {
		return Config{}, fmt.Errorf("SECRETO_COOKIE es obligatoria y debe tener al menos 32 bytes")
	}
	cfg.SecretoCookie = []byte(secreto)

	var err error
	if cfg.LatidoCaducadoWorker, err = envDur("LATIDO_CADUCADO_WORKER", 15*time.Second, true); err != nil {
		return Config{}, err
	}
	if cfg.Simulaciones, err = envInt("SIMULACIONES", 300, 1, 2000); err != nil {
		return Config{}, err
	}
	if cfg.AleatoriedadRival, err = envFloat("ALEATORIEDAD_RIVAL", 0.2, 0, 1); err != nil {
		return Config{}, err
	}
	if cfg.TrabajosSimultaneos, err = envInt("TRABAJOS_SIMULTANEOS_WORKER", 2, 1, 1000); err != nil {
		return Config{}, err
	}
	if cfg.MaxPendientesPorUsuario, err = envInt("MAX_PENDIENTES_POR_USUARIO", 1, 1, 10000); err != nil {
		return Config{}, err
	}
	if cfg.MaxCola, err = envInt("MAX_COLA", 200, 1, 1000000); err != nil {
		return Config{}, err
	}
	if cfg.CacheTTL, err = envDur("CACHE_TTL", 168*time.Hour, true); err != nil {
		return Config{}, err
	}
	if cfg.Retencion, err = envDur("RETENCION", 720*time.Hour, true); err != nil {
		return Config{}, err
	}
	if cfg.CancelarSinCliente, err = envDur("CANCELAR_SIN_CLIENTE", 2*time.Minute, false); err != nil {
		return Config{}, err
	}
	if cfg.IntervaloSondeoSSE, err = envDur("INTERVALO_SONDEO_SSE", 5*time.Second, true); err != nil {
		return Config{}, err
	}
	if cfg.LimitePorMinuto, err = envInt("LIMITE_POR_MINUTO", 10, 1, 1000000); err != nil {
		return Config{}, err
	}
	if cfg.ProxiesConf, err = parseProxies(os.Getenv("PROXIES_CONFIABLES")); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envInt(key string, fallback, min, max int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v < min || v > max {
		return 0, fmt.Errorf("%s debe ser un número entre %d y %d", key, min, max)
	}
	return v, nil
}

func envFloat(key string, fallback, min, max float64) (float64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v < min || v > max {
		return 0, fmt.Errorf("%s debe ser un número entre %g y %g", key, min, max)
	}
	return v, nil
}

func envDur(key string, fallback time.Duration, positivo bool) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || (positivo && v <= 0) || v < 0 {
		return 0, fmt.Errorf("%s debe ser una duración válida (por ejemplo 30s, 2m, 24h)", key)
	}
	return v, nil
}

func parseProxies(raw string) ([]netip.Prefix, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var prefijos []netip.Prefix
	for _, parte := range strings.Split(raw, ",") {
		parte = strings.TrimSpace(parte)
		if parte == "" {
			continue
		}
		if strings.Contains(parte, "/") {
			p, err := netip.ParsePrefix(parte)
			if err != nil {
				return nil, fmt.Errorf("PROXIES_CONFIABLES contiene un CIDR inválido: %s", parte)
			}
			prefijos = append(prefijos, p.Masked())
			continue
		}
		ip, err := netip.ParseAddr(parte)
		if err != nil {
			return nil, fmt.Errorf("PROXIES_CONFIABLES contiene una IP inválida: %s", parte)
		}
		prefijos = append(prefijos, netip.PrefixFrom(ip, ip.BitLen()))
	}
	return prefijos, nil
}
