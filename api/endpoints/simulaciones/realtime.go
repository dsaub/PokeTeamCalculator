package simulaciones

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
)

// Realtime conecta la base de datos con el hub de suscriptores SSE. Los
// cambios que aplica una instancia de la API llegan a las demás por Redis.
type Realtime struct {
	hub       *Hub
	svc       *Service
	cola      *ColaRedis
	intervalo time.Duration
	logger    *slog.Logger
}

func NewRealtime(hub *Hub, svc *Service, cola *ColaRedis, intervalo time.Duration, logger *slog.Logger) *Realtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Realtime{hub: hub, svc: svc, cola: cola, intervalo: intervalo, logger: logger}
}

func (rt *Realtime) Hub() *Hub {
	return rt.hub
}

// Run ejecuta el sondeo de respaldo. Cubre los avisos que se pierdan (Pub/Sub
// no los guarda) y los cambios de posición que no pasan por esta instancia.
func (rt *Realtime) Run(ctx context.Context) {
	ticker := time.NewTicker(rt.intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.PollOnce(ctx)
		}
	}
}

func (rt *Realtime) PollOnce(ctx context.Context) {
	ids := rt.hub.IDs()
	if len(ids) == 0 {
		return
	}
	jobs, err := rt.svc.GetMany(ctx, ids)
	if err != nil {
		rt.logger.Error("sondeo_sse", "error", err)
		return
	}
	// Sin Redis no hay posiciones, pero los demás estados se publican igual.
	posiciones, err := rt.svc.Posiciones(ctx)
	if err != nil {
		rt.logger.Warn("posiciones_sse", "error", err)
	}
	for i := range jobs {
		rt.publicarEstado(ctx, &jobs[i], posiciones)
	}
}

func (rt *Realtime) publicarEstado(ctx context.Context, job *Job, posiciones map[int64]int) {
	switch job.Estado {
	case EstadoPendiente:
		if pos := posiciones[job.ID]; pos > 0 {
			rt.hub.PublishPosition(job.ID, pos, rt.svc.EstimateSeconds(ctx, pos))
		}
	case EstadoProcesando:
		rt.hub.PublishState(job.ID, EstadoProcesando, eventoProcesando())
	case EstadoCompletado, EstadoError, EstadoCancelado:
		rt.PublishFinal(job)
	}
}

// PublishFinal avisa a los suscriptores de un trabajo terminado.
func (rt *Realtime) PublishFinal(job *Job) {
	if ev, ok := eventoFinal(job); ok {
		rt.hub.PublishFinal(job.ID, ev)
	}
}

// PublishAllPositions lee de Redis la posición de todos los pendientes y
// avisa solo a los que tengan suscriptores.
func (rt *Realtime) PublishAllPositions(ctx context.Context) {
	posiciones, err := rt.svc.Posiciones(ctx)
	if err != nil {
		rt.logger.Error("posiciones_sse", "error", err)
		return
	}
	rt.publicarPosiciones(ctx, posiciones)
}

func (rt *Realtime) publicarPosiciones(ctx context.Context, posiciones map[int64]int) {
	for id, pos := range posiciones {
		if rt.hub.HasSubscribers(id) {
			rt.hub.PublishPosition(id, pos, rt.svc.EstimateSeconds(ctx, pos))
		}
	}
}

// PublishCancel cierra el SSE de un trabajo cancelado en esta instancia y en
// las demás.
func (rt *Realtime) PublishCancel(ctx context.Context, id int64) {
	rt.hub.PublishFinal(id, eventoCancelado())
	rt.avisar(ctx, id)
	rt.PublishAllPositions(ctx)
}

// Notificar publica el nuevo estado de un trabajo al SSE de esta instancia y
// avisa a las demás.
func (rt *Realtime) Notificar(ctx context.Context, id int64) {
	rt.Refrescar(ctx, id)
	rt.avisar(ctx, id)
}

func (rt *Realtime) avisar(ctx context.Context, id int64) {
	if err := rt.cola.Avisar(ctx, id); err != nil {
		rt.logger.Warn("aviso_no_enviado", "id", id, "error", err)
	}
}

// Refrescar lee el trabajo, publica su estado a los suscriptores de esta
// instancia y recalcula las posiciones de los que esperan.
func (rt *Realtime) Refrescar(ctx context.Context, id int64) {
	// Sin Redis no hay posiciones, pero el estado del trabajo se publica igual.
	posiciones, err := rt.svc.Posiciones(ctx)
	if err != nil {
		rt.logger.Warn("posiciones_sse", "error", err)
	}
	if rt.hub.HasSubscribers(id) {
		job, err := rt.svc.GetJob(ctx, id)
		switch {
		case err == nil:
			rt.publicarEstado(ctx, job, posiciones)
		case !errors.Is(err, ErrNoEncontrado):
			rt.logger.Error("refrescar_sse", "id", id, "error", err)
		}
	}
	rt.publicarPosiciones(ctx, posiciones)
}

// Escuchar atiende los avisos de las demás instancias hasta que ctx termine.
func (rt *Realtime) Escuchar(ctx context.Context) {
	rt.cola.EscucharAvisos(ctx, func(id int64) { rt.Refrescar(ctx, id) })
}

func eventoPosicion(pos, estimado int) Event {
	data, _ := json.Marshal(struct {
		Posicion         int `json:"posicion"`
		EstimadoSegundos int `json:"estimadoSegundos"`
	}{pos, estimado})
	return Event{Name: "posicion", Data: data}
}

func eventoProcesando() Event {
	return Event{Name: "procesando", Data: []byte("{}")}
}

func eventoCompletado(resultado []byte) Event {
	raw := json.RawMessage(resultado)
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	data, err := json.Marshal(struct {
		Resultado json.RawMessage `json:"resultado"`
	}{raw})
	if err != nil {
		data = []byte(`{"resultado":null}`)
	}
	return Event{Name: "completado", Data: data, Final: true}
}

func eventoError(mensaje string) Event {
	data, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{mensaje})
	return Event{Name: "error", Data: data, Final: true}
}

func eventoCancelado() Event {
	return Event{Name: "cancelado", Data: []byte("{}"), Final: true}
}

func eventoFinal(job *Job) (Event, bool) {
	switch job.Estado {
	case EstadoCompletado:
		return eventoCompletado(job.Resultado), true
	case EstadoError:
		return eventoError(job.Error.String), true
	case EstadoCancelado:
		return eventoCancelado(), true
	default:
		return Event{}, false
	}
}
