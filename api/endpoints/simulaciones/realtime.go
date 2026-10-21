package simulaciones

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// Realtime conecta la base de datos con el hub de suscriptores SSE.
type Realtime struct {
	hub       *Hub
	svc       *Service
	intervalo time.Duration
	logger    *slog.Logger
}

func NewRealtime(hub *Hub, svc *Service, intervalo time.Duration, logger *slog.Logger) *Realtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Realtime{hub: hub, svc: svc, intervalo: intervalo, logger: logger}
}

func (rt *Realtime) Hub() *Hub {
	return rt.hub
}

// Run ejecuta el sondeo de respaldo. Cubre el paso a `procesando` (el worker
// no avisa de eso) y los finales cuyo callback se haya perdido.
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
	for i := range jobs {
		job := &jobs[i].Job
		switch job.Estado {
		case EstadoPendiente:
			pos := jobs[i].Posicion
			rt.hub.PublishPosition(job.ID, pos, rt.svc.EstimateSeconds(ctx, pos))
		case EstadoProcesando:
			rt.hub.PublishState(job.ID, EstadoProcesando, eventoProcesando())
		case EstadoCompletado, EstadoError, EstadoCancelado:
			if ev, ok := eventoFinal(job); ok {
				rt.hub.PublishFinal(job.ID, ev)
			}
		}
	}
}

// PublishFinal avisa a los suscriptores de un trabajo terminado.
func (rt *Realtime) PublishFinal(job *Job) {
	if ev, ok := eventoFinal(job); ok {
		rt.hub.PublishFinal(job.ID, ev)
	}
}

// PublishAllPositions recalcula con una sola consulta la posición de todos los
// pendientes y avisa solo a los que tengan suscriptores.
func (rt *Realtime) PublishAllPositions(ctx context.Context) {
	posiciones, err := rt.svc.PendingPositions(ctx)
	if err != nil {
		rt.logger.Error("posiciones_sse", "error", err)
		return
	}
	for id, pos := range posiciones {
		if rt.hub.HasSubscribers(id) {
			rt.hub.PublishPosition(id, pos, rt.svc.EstimateSeconds(ctx, pos))
		}
	}
}

func (rt *Realtime) PublishCancel(id int64) {
	rt.hub.PublishFinal(id, eventoCancelado())
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
