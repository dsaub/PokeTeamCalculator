package simulaciones

import (
	"strings"
	"testing"
)

func limpiarEntorno(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"TOKEN_INTERNO", "URL_WORKER", "HOST_INTERNO", "PUERTO_INTERNO", "SECRETO_COOKIE",
		"SIMULACIONES", "ALEATORIEDAD_RIVAL", "TRABAJOS_SIMULTANEOS_WORKER",
		"MAX_PENDIENTES_POR_USUARIO", "MAX_COLA", "CACHE_TTL", "RETENCION",
		"CANCELAR_SIN_CLIENTE", "INTERVALO_SONDEO_SSE", "LIMITE_POR_MINUTO",
		"PROXIES_CONFIABLES",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadConfigValida(t *testing.T) {
	limpiarEntorno(t)
	t.Setenv("TOKEN_INTERNO", strings.Repeat("a", 16))
	t.Setenv("SECRETO_COOKIE", strings.Repeat("b", 32))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URLWorker != "http://127.0.0.1:7070" {
		t.Fatalf("URL_WORKER %q", cfg.URLWorker)
	}
	if cfg.PuertoInterno != "8081" {
		t.Fatalf("PUERTO_INTERNO %q", cfg.PuertoInterno)
	}
	if cfg.HostInterno != "127.0.0.1" || cfg.DireccionInterna() != "127.0.0.1:8081" {
		t.Fatalf("HOST_INTERNO %q / dirección %q", cfg.HostInterno, cfg.DireccionInterna())
	}
	if cfg.Simulaciones != 300 || cfg.MaxCola != 200 || cfg.LimitePorMinuto != 10 {
		t.Fatalf("defaults inesperados: %+v", cfg)
	}
	if cfg.AleatoriedadRival != 0.2 || cfg.TrabajosSimultaneos != 2 {
		t.Fatalf("defaults inesperados: %+v", cfg)
	}
}

func TestLoadConfigTokenCorto(t *testing.T) {
	limpiarEntorno(t)
	t.Setenv("TOKEN_INTERNO", strings.Repeat("a", 15))
	t.Setenv("SECRETO_COOKIE", strings.Repeat("b", 32))
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "16") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigSecretoCorto(t *testing.T) {
	limpiarEntorno(t)
	t.Setenv("TOKEN_INTERNO", strings.Repeat("a", 16))
	t.Setenv("SECRETO_COOKIE", "corto")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "32") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigDuracionInvalida(t *testing.T) {
	limpiarEntorno(t)
	t.Setenv("TOKEN_INTERNO", strings.Repeat("a", 16))
	t.Setenv("SECRETO_COOKIE", strings.Repeat("b", 32))
	t.Setenv("CACHE_TTL", "no-es-una-duracion")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "CACHE_TTL") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigHostInternoInvalido(t *testing.T) {
	limpiarEntorno(t)
	t.Setenv("TOKEN_INTERNO", strings.Repeat("a", 16))
	t.Setenv("SECRETO_COOKIE", strings.Repeat("b", 32))
	t.Setenv("HOST_INTERNO", "no-es-una-ip")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "HOST_INTERNO") {
		t.Fatalf("error inesperado: %v", err)
	}
}

func TestLoadConfigProxiesInvalidos(t *testing.T) {
	limpiarEntorno(t)
	t.Setenv("TOKEN_INTERNO", strings.Repeat("a", 16))
	t.Setenv("SECRETO_COOKIE", strings.Repeat("b", 32))
	t.Setenv("PROXIES_CONFIABLES", "10.0.0.0/33")
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "PROXIES_CONFIABLES") {
		t.Fatalf("error inesperado: %v", err)
	}
}
