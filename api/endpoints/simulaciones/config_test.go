package simulaciones

import (
	"strings"
	"testing"
	"time"
)

func limpiarEntorno(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"REDIS_URL", "LATIDO_CADUCADO_WORKER", "SECRETO_COOKIE",
		"SIMULACIONES", "ALEATORIEDAD_RIVAL", "TRABAJOS_SIMULTANEOS_WORKER",
		"MAX_PENDIENTES_POR_USUARIO", "MAX_COLA", "CACHE_TTL", "RETENCION",
		"CANCELAR_SIN_CLIENTE", "INTERVALO_SONDEO_SSE", "LIMITE_POR_MINUTO",
		"PROXIES_CONFIABLES",
	} {
		t.Setenv(k, "")
	}
}

func entornoMinimo(t *testing.T) {
	t.Helper()
	limpiarEntorno(t)
	t.Setenv("REDIS_URL", "redis://:clave@127.0.0.1:6379/0")
	t.Setenv("SECRETO_COOKIE", strings.Repeat("b", 32))
}

func TestLoadConfigValida(t *testing.T) {
	entornoMinimo(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLRedis != "redis://:clave@127.0.0.1:6379/0" {
		t.Fatalf("REDIS_URL %q", cfg.URLRedis)
	}
	if cfg.LatidoCaducadoWorker != 15*time.Second {
		t.Fatalf("LATIDO_CADUCADO_WORKER %v", cfg.LatidoCaducadoWorker)
	}
	if cfg.Simulaciones != 300 || cfg.MaxCola != 200 || cfg.LimitePorMinuto != 10 {
		t.Fatalf("defaults inesperados: %+v", cfg)
	}
	if cfg.AleatoriedadRival != 0.2 || cfg.TrabajosSimultaneos != 2 {
		t.Fatalf("defaults inesperados: %+v", cfg)
	}
}

func TestLoadConfigSinRedis(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("REDIS_URL", "")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "REDIS_URL es obligatoria") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigRedisInvalida(t *testing.T) {
	for _, url := range []string{"http://127.0.0.1:6379", "127.0.0.1:6379", "redis://127.0.0.1:6379/no-es-un-numero"} {
		entornoMinimo(t)
		t.Setenv("REDIS_URL", url)
		if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
			t.Fatalf("%q: error inesperado: %v", url, err)
		}
	}
}

func TestLoadConfigSecretoCorto(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("SECRETO_COOKIE", "corto")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "32") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigDuracionInvalida(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("CACHE_TTL", "no-es-una-duracion")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "CACHE_TTL") {
		t.Fatalf("error inesperado: %v", err)
	}
	entornoMinimo(t)
	t.Setenv("LATIDO_CADUCADO_WORKER", "0s")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "LATIDO_CADUCADO_WORKER") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigProxiesInvalidos(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("PROXIES_CONFIABLES", "10.0.0.0/33")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "PROXIES_CONFIABLES") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigValoresFueraDeRango(t *testing.T) {
	casos := []struct {
		variable, valor, mensaje string
	}{
		{"SIMULACIONES", "0", "SIMULACIONES debe ser un número entre 1 y 2000"},
		{"SIMULACIONES", "2001", "SIMULACIONES debe ser un número entre 1 y 2000"},
		{"SIMULACIONES", "diez", "SIMULACIONES debe ser un número entre 1 y 2000"},
		{"ALEATORIEDAD_RIVAL", "1.5", "ALEATORIEDAD_RIVAL debe ser un número entre 0 y 1"},
		{"ALEATORIEDAD_RIVAL", "-0.1", "ALEATORIEDAD_RIVAL debe ser un número entre 0 y 1"},
		{"ALEATORIEDAD_RIVAL", "mucha", "ALEATORIEDAD_RIVAL debe ser un número entre 0 y 1"},
		{"TRABAJOS_SIMULTANEOS_WORKER", "0", "TRABAJOS_SIMULTANEOS_WORKER"},
		{"MAX_PENDIENTES_POR_USUARIO", "0", "MAX_PENDIENTES_POR_USUARIO"},
		{"MAX_COLA", "0", "MAX_COLA"},
		{"LIMITE_POR_MINUTO", "0", "LIMITE_POR_MINUTO"},
		{"LATIDO_CADUCADO_WORKER", "0s", "LATIDO_CADUCADO_WORKER debe ser una duración válida"},
		{"CACHE_TTL", "-1h", "CACHE_TTL"},
		{"RETENCION", "un mes", "RETENCION"},
		{"CANCELAR_SIN_CLIENTE", "-1m", "CANCELAR_SIN_CLIENTE"},
		{"INTERVALO_SONDEO_SSE", "0", "INTERVALO_SONDEO_SSE"},
		{"PROXIES_CONFIABLES", "10.0.0.0/33", "CIDR inválido: 10.0.0.0/33"},
		{"PROXIES_CONFIABLES", "10.0.0.300", "IP inválida: 10.0.0.300"},
	}
	for _, caso := range casos {
		t.Run(caso.variable+"="+caso.valor, func(t *testing.T) {
			entornoMinimo(t)
			t.Setenv(caso.variable, caso.valor)
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), caso.mensaje) {
				t.Fatalf("error %v, esperado %q", err, caso.mensaje)
			}
		})
	}
}

func TestLoadConfigLeeTodasLasVariables(t *testing.T) {
	entornoMinimo(t)
	valores := map[string]string{
		"LATIDO_CADUCADO_WORKER":      "20s",
		"SIMULACIONES":                " 500 ",
		"ALEATORIEDAD_RIVAL":          "0.5",
		"TRABAJOS_SIMULTANEOS_WORKER": "4",
		"MAX_PENDIENTES_POR_USUARIO":  "2",
		"MAX_COLA":                    "50",
		"CACHE_TTL":                   "1h",
		"RETENCION":                   "48h",
		"CANCELAR_SIN_CLIENTE":        "0s",
		"INTERVALO_SONDEO_SSE":        "2s",
		"LIMITE_POR_MINUTO":           "30",
		"PROXIES_CONFIABLES":          " 10.0.0.0/8 , ,192.168.1.7, 2001:db8::/32 ",
	}
	for k, v := range valores {
		t.Setenv(k, v)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LatidoCaducadoWorker != 20*time.Second || cfg.Simulaciones != 500 || cfg.AleatoriedadRival != 0.5 ||
		cfg.TrabajosSimultaneos != 4 || cfg.MaxPendientesPorUsuario != 2 || cfg.MaxCola != 50 ||
		cfg.CacheTTL != time.Hour || cfg.Retencion != 48*time.Hour || cfg.CancelarSinCliente != 0 ||
		cfg.IntervaloSondeoSSE != 2*time.Second || cfg.LimitePorMinuto != 30 {
		t.Fatalf("config %+v", cfg)
	}
	if len(cfg.ProxiesConf) != 3 || cfg.ProxiesConf[1].String() != "192.168.1.7/32" {
		t.Fatalf("proxies %v", cfg.ProxiesConf)
	}
}

func TestParseProxies(t *testing.T) {
	for _, vacio := range []string{"", "   "} {
		if p, err := parseProxies(vacio); err != nil || p != nil {
			t.Fatalf("%q: %v %v", vacio, p, err)
		}
	}
	p, err := parseProxies("10.1.2.3/8")
	if err != nil || p[0].String() != "10.0.0.0/8" {
		t.Fatalf("el CIDR se normaliza: %v %v", p, err)
	}
}

func TestLoadConfigRedisConEspacios(t *testing.T) {
	entornoMinimo(t)
	t.Setenv("REDIS_URL", "  redis://127.0.0.1:6379  ")
	cfg, err := LoadConfig()
	if err != nil || cfg.URLRedis != "redis://127.0.0.1:6379" {
		t.Fatalf("%q %v", cfg.URLRedis, err)
	}
}
