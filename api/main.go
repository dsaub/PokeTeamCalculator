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

	worker := simulaciones.NewWorkerClient(cfg.URLWorker, cfg.TokenInterno, logger)
	repo := simulaciones.NewMySQLRepository(database.GetDatabase())
	catalog := simulaciones.NewMySQLCatalog(database.GetDatabase())
	svc := simulaciones.NewService(repo, cfg, worker, logger, catalog)
	hub := simulaciones.NewHub()
	rt := simulaciones.NewRealtime(hub, svc, cfg.IntervaloSondeoSSE, logger)
	handler := simulaciones.NewHandler(svc, rt, cfg, logger)
	internal := simulaciones.NewInternalHandler(svc, rt, cfg.TokenInterno, logger)

	catalogoSvc := catalogo.NewService(catalogo.NewMySQLRepository(database.GetDatabase()))
	catalogoHandler := catalogo.NewHandler(catalogoSvc, logger)

	router := gin.Default()
	router.HandleMethodNotAllowed = true
	router.GET("/api/v1/pokemon", pokemons.GetAllPokemons)
	router.GET("/api/v1/pokemon/:id", pokemons.GetPokemonByID)
	handler.Register(router)
	catalogoHandler.Register(router)
	router.NoRoute(codehandlers.ErrorNotFound)
	router.NoMethod(codehandlers.ErrorMethodNotAllowed)

	puerto := os.Getenv("PORT")
	if puerto == "" {
		puerto = "8080"
	}

	// Sin WriteTimeout: las conexiones SSE son de larga duración.
	publico := &http.Server{
		Addr:              ":" + puerto,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	interno := &http.Server{
		Addr:              cfg.DireccionInterna(),
		Handler:           internal,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go worker.Run(ctx)
	go handler.RateLimiter().Run(ctx)
	go rt.Run(ctx)
	go rt.CancelAbandonedLoop(ctx)
	go rt.CleanupLoop(ctx)

	errores := make(chan error, 2)
	go func() {
		logger.Info("api_escuchando", "direccion", publico.Addr)
		if err := publico.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errores <- err
		}
	}()
	go func() {
		logger.Info("api_interna_escuchando", "direccion", interno.Addr)
		if err := interno.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errores <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("apagado", "motivo", "señal")
	case err := <-errores:
		logger.Error("servidor", "error", err)
	}

	// Cerrar SSE y goroutines periódicas; después, los dos servidores.
	hub.Close()
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := publico.Shutdown(shutdownCtx); err != nil {
		logger.Error("apagado_api", "error", err)
	}
	if err := interno.Shutdown(shutdownCtx); err != nil {
		logger.Error("apagado_api_interna", "error", err)
	}
}
