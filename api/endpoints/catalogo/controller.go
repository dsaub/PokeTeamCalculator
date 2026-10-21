package catalogo

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc    *Service
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

func (h *Handler) Register(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("/juegos", h.juegos)
	api.GET("/juegos/:juego/lideres", h.lideres)
	api.GET("/pokemon", h.buscar)
	api.GET("/pokemon/:especie", h.pokemon)
}

// ok cachea solo las respuestas correctas: un 400/404/500 no debe quedarse
// una hora en la caché del navegador ni de un proxy intermedio.
func ok(c *gin.Context, cuerpo any) {
	c.Header("Cache-Control", "public, max-age=3600")
	c.JSON(http.StatusOK, cuerpo)
}

func (h *Handler) juegos(c *gin.Context) {
	juegos, err := h.svc.Juegos(c.Request.Context())
	if err != nil {
		h.error(c, "listar_juegos", err)
		return
	}
	ok(c, juegos)
}

func (h *Handler) lideres(c *gin.Context) {
	lideres, err := h.svc.Lideres(c.Request.Context(), c.Param("juego"))
	if err != nil {
		h.error(c, "listar_lideres", err)
		return
	}
	ok(c, lideres)
}

func (h *Handler) buscar(c *gin.Context) {
	resultados, err := h.svc.BuscarPokemon(c.Request.Context(), c.Query("gen"), c.Query("q"))
	if err != nil {
		h.error(c, "buscar_pokemon", err)
		return
	}
	ok(c, resultados)
}

func (h *Handler) pokemon(c *gin.Context) {
	detalle, err := h.svc.Pokemon(c.Request.Context(), c.Query("gen"), c.Param("especie"))
	if err != nil {
		h.error(c, "detalle_pokemon", err)
		return
	}
	ok(c, detalle)
}

func (h *Handler) error(c *gin.Context, evento string, err error) {
	var validacion *ErrorValidacion
	switch {
	case errors.As(err, &validacion):
		c.JSON(http.StatusBadRequest, gin.H{"error": validacion.Error()})
	case errors.Is(err, ErrNoEncontrado):
		c.JSON(http.StatusNotFound, gin.H{"error": "No encontrado"})
	default:
		h.logger.Error(evento, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error interno"})
	}
}
