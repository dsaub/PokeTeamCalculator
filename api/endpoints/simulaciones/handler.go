package simulaciones

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"PokeTeamCalculator/database"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc      *Service
	rt       *Realtime
	cfg      Config
	sessions *Sessions
	limiter  *RateLimiter
	logger   *slog.Logger
}

func NewHandler(svc *Service, rt *Realtime, cfg Config, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		svc:      svc,
		rt:       rt,
		cfg:      cfg,
		sessions: NewSessions(cfg.SecretoCookie),
		limiter:  NewRateLimiter(cfg.LimitePorMinuto),
		logger:   logger,
	}
}

func (h *Handler) RateLimiter() *RateLimiter {
	return h.limiter
}

func (h *Handler) Register(r *gin.Engine) {
	api := r.Group("/api")
	api.Use(h.sessions.Ensure)
	api.POST("/simulaciones", h.crear)
	api.GET("/simulaciones/:id", h.obtener)
	api.DELETE("/simulaciones/:id", h.cancelar)
	api.GET("/simulaciones/:id/eventos", h.eventos)
	api.GET("/salud", h.salud)
}

type creacionDTO struct {
	ID               int64           `json:"id"`
	Estado           string          `json:"estado"`
	Posicion         *int            `json:"posicion,omitempty"`
	EstimadoSegundos *int            `json:"estimadoSegundos,omitempty"`
	Cache            bool            `json:"cache,omitempty"`
	Resultado        json.RawMessage `json:"resultado,omitempty"`
	WorkerDisponible *bool           `json:"workerDisponible,omitempty"`
}

func (h *Handler) crear(c *gin.Context) {
	ip := ClientIP(c.Request, h.cfg.ProxiesConf)
	if !h.limiter.Allow(ip) {
		h.logger.Warn("limite_superado", "ip", ip)
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Demasiadas peticiones, espera un minuto"})
		return
	}

	p, err := LeerPeticion(c.Writer, c.Request)
	if err != nil {
		var validacion *ValidationError
		if errors.As(err, &validacion) {
			c.JSON(http.StatusBadRequest, gin.H{"error": validacion.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "cuerpo: JSON inválido"})
		return
	}

	usuario := UsuarioActual(c)
	resp, err := h.svc.Create(c.Request.Context(), usuario, Peticion{
		Gen:        p.Gen,
		Rival:      p.Rival,
		Candidatos: p.Candidatos,
	})
	if err != nil {
		var validacion *ValidationError
		switch {
		case errors.As(err, &validacion):
			c.JSON(http.StatusBadRequest, gin.H{"error": validacion.Error()})
		case errors.Is(err, ErrLimiteUsuario):
			cuerpo := gin.H{"error": "Ya tienes una simulación en cola"}
			var limite *LimiteUsuarioError
			if errors.As(err, &limite) && limite.ID > 0 {
				cuerpo["id"] = limite.ID
			}
			c.JSON(http.StatusTooManyRequests, cuerpo)
		case errors.Is(err, ErrLimiteGlobal):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrLimiteGlobal.Error()})
		default:
			h.logger.Error("crear_simulacion", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo crear la simulación"})
		}
		return
	}

	dto := creacionDTO{
		ID:     resp.ID,
		Estado: resp.Estado,
	}
	if resp.Cache {
		dto.Cache = true
		dto.Resultado = json.RawMessage(resp.Resultado)
		c.JSON(http.StatusOK, dto)
		return
	}
	if resp.Posicion > 0 {
		dto.Posicion = &resp.Posicion
		dto.EstimadoSegundos = &resp.EstimadoSegundos
	}
	if !resp.WorkerDisponible {
		falso := false
		dto.WorkerDisponible = &falso
	}
	c.JSON(http.StatusAccepted, dto)
}

type estadoDTO struct {
	ID               int64           `json:"id"`
	Estado           string          `json:"estado"`
	Posicion         *int            `json:"posicion,omitempty"`
	EstimadoSegundos *int            `json:"estimadoSegundos,omitempty"`
	Resultado        json.RawMessage `json:"resultado,omitempty"`
	Error            string          `json:"error,omitempty"`
}

func (h *Handler) obtener(c *gin.Context) {
	id, ok := idDeRuta(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Simulación no encontrada"})
		return
	}
	view, err := h.svc.GetForUser(c.Request.Context(), id, UsuarioActual(c))
	if err != nil {
		h.responderError(c, err)
		return
	}
	c.JSON(http.StatusOK, dtoDeVista(view))
}

func (h *Handler) cancelar(c *gin.Context) {
	id, ok := idDeRuta(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Simulación no encontrada"})
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id, UsuarioActual(c)); err != nil {
		switch {
		case errors.Is(err, ErrNoEncontrado):
			c.JSON(http.StatusNotFound, gin.H{"error": "Simulación no encontrada"})
		case errors.Is(err, ErrYaTerminado):
			c.JSON(http.StatusConflict, gin.H{"error": "La simulación ya había terminado"})
		default:
			h.logger.Error("cancelar_simulacion", "id", id, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo cancelar la simulación"})
		}
		return
	}
	h.rt.PublishCancel(c.Request.Context(), id)
	c.Status(http.StatusNoContent)
}

func (h *Handler) eventos(c *gin.Context) {
	id, ok := idDeRuta(c)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Simulación no encontrada"})
		return
	}
	usuario := UsuarioActual(c)

	// Comprobación de acceso antes de abrir el stream.
	if _, err := h.svc.GetForUser(c.Request.Context(), id, usuario); err != nil {
		h.responderError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// El servidor no tiene WriteTimeout, pero por si esta ruta se monta en
	// otro servidor, desactivamos el deadline de escritura.
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})

	sub := h.rt.Hub().Subscribe(id)
	defer h.rt.Hub().Unsubscribe(id, sub)

	// Volvemos a leer después de suscribirnos para no perder un cambio entre
	// la comprobación de acceso y el registro.
	view, err := h.svc.GetForUser(c.Request.Context(), id, usuario)
	if err != nil {
		return
	}
	ev, final, estado, pos := eventoInicial(view)
	if escribirEvento(c.Writer, ev) != nil {
		return
	}
	h.rt.Hub().Mark(sub, estado, pos)
	if final {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case ev := <-sub.Ch:
			if escribirEvento(c.Writer, ev) != nil {
				return
			}
			if ev.Final {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		case <-sub.Done:
			return
		}
	}
}

// salud responde 503 solo si falla MariaDB: sin Redis el catálogo funciona y
// los trabajos se guardan igual (la reconciliación los encola después).
func (h *Handler) salud(c *gin.Context) {
	estado := database.GetStatus()
	codigo := http.StatusOK
	if estado != "ok" {
		codigo = http.StatusServiceUnavailable
	}
	cola := "ok"
	if !h.svc.ColaResponde(c.Request.Context()) {
		cola = "error"
	}
	c.JSON(codigo, gin.H{
		"status":           estado,
		"baseDatos":        estado,
		"cola":             cola,
		"workerDisponible": h.svc.WorkerDisponible(),
	})
}

func (h *Handler) responderError(c *gin.Context, err error) {
	if errors.Is(err, ErrNoEncontrado) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Simulación no encontrada"})
		return
	}
	h.logger.Error("leer_simulacion", "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo leer la simulación"})
}

func dtoDeVista(v *JobView) estadoDTO {
	dto := estadoDTO{
		ID:     v.ID,
		Estado: v.Estado,
	}
	if v.Posicion > 0 {
		dto.Posicion = &v.Posicion
		dto.EstimadoSegundos = &v.EstimadoSegundos
	}
	if len(v.Resultado) > 0 {
		dto.Resultado = json.RawMessage(v.Resultado)
	}
	if v.Error != "" {
		dto.Error = v.Error
	}
	return dto
}

func eventoInicial(v *JobView) (Event, bool, string, int) {
	switch v.Estado {
	case EstadoPendiente:
		return eventoPosicion(v.Posicion, v.EstimadoSegundos), false, EstadoPendiente, v.Posicion
	case EstadoProcesando:
		return eventoProcesando(), false, EstadoProcesando, 0
	case EstadoCompletado:
		return eventoCompletado(v.Resultado), true, EstadoCompletado, 0
	case EstadoError:
		return eventoError(v.Error), true, EstadoError, 0
	case EstadoCancelado:
		return eventoCancelado(), true, EstadoCancelado, 0
	default:
		return eventoCancelado(), true, EstadoCancelado, 0
	}
}

func escribirEvento(w http.ResponseWriter, ev Event) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Name, ev.Data); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func idDeRuta(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
