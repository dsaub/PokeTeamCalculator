package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PokeTeamCalculator/database"
	"PokeTeamCalculator/endpoints/catalogo"
	codehandlers "PokeTeamCalculator/endpoints/code_handlers"
	"PokeTeamCalculator/endpoints/pokemons"
	"PokeTeamCalculator/endpoints/simulaciones"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	database.Initialize()

	cfg, err := simulaciones.LoadConfig()
	if err != nil {
		logger.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rdb, err := simulaciones.ClienteRedis(cfg.URLRedis)
	if err != nil {
		logger.Error("configuración inválida", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()
	cola := simulaciones.NewColaRedis(rdb, cfg.LatidoCaducadoWorker, logger)
	// Sin Redis la API arranca igual: los trabajos se guardan en MariaDB y la
	// reconciliación los encola cuando vuelva.
	if err := cola.Esperar(ctx, 15, 2*time.Second); err != nil {
		logger.Error("cola_no_disponible", "error", err)
	}

	repo := simulaciones.NewMySQLRepository(database.GetDatabase())
	catalog := simulaciones.NewMySQLCatalog(database.GetDatabase())
	svc := simulaciones.NewService(repo, cfg, cola, logger, catalog)
	hub := simulaciones.NewHub()
	rt := simulaciones.NewRealtime(hub, svc, cola, cfg.IntervaloSondeoSSE, logger)
	estados := simulaciones.NewEstados(cola, svc, rt, logger)
	handler := simulaciones.NewHandler(svc, rt, cfg, logger)

	catalogoSvc := catalogo.NewService(catalogo.NewMySQLRepository(database.GetDatabase()))
	catalogoHandler := catalogo.NewHandler(catalogoSvc, logger)

	router := nuevoRouter(handler, catalogoHandler)

	// Sin WriteTimeout: las conexiones SSE son de larga duración.
	publico := &http.Server{
		Addr:              ":" + puerto(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go cola.Run(ctx)
	go estados.Run(ctx)
	go rt.Escuchar(ctx)
	go rt.ReconciliarLoop(ctx)
	go handler.RateLimiter().Run(ctx)
	go rt.Run(ctx)
	go rt.CancelAbandonedLoop(ctx)
	go rt.CleanupLoop(ctx)

	errores := make(chan error, 1)
	go func() {
		logger.Info("api_escuchando", "direccion", publico.Addr)
		if err := publico.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errores <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("apagado", "motivo", "señal")
	case err := <-errores:
		logger.Error("servidor", "error", err)
	}

	// Cerrar SSE y goroutines periódicas; después, el servidor. El cliente de
	// Redis se cierra al salir.
	hub.Close()
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := publico.Shutdown(shutdownCtx); err != nil {
		logger.Error("apagado_api", "error", err)
	}
}

// modulo es un grupo de rutas que se registra en el router (simulaciones, catálogo).
type modulo interface {
	Register(r *gin.Engine)
}

// nuevoRouter monta todas las rutas de la API y las respuestas 404 y 405.
func nuevoRouter(modulos ...modulo) *gin.Engine {
	router := gin.Default()
	router.HandleMethodNotAllowed = true
	router.GET("/api/v1/pokemon", pokemons.GetAllPokemons)
	router.GET("/api/v1/pokemon/:id", pokemons.GetPokemonByID)
	for _, m := range modulos {
		m.Register(router)
	}
	router.NoRoute(codehandlers.ErrorNotFound)
	router.NoMethod(codehandlers.ErrorMethodNotAllowed)
	return router
}

// puerto devuelve PORT o, si no está definida, 8080.
func puerto() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return "8080"
}
