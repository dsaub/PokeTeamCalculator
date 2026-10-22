package simulaciones

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	bloqueoEstados      = 5 * time.Second
	reclamarEstadosCada = 30 * time.Second
	// estadoInactivo es cuánto puede llevar un estado leído sin confirmar
	// antes de que otro consumidor (o este) lo reclame.
	estadoInactivo = time.Minute
	maxResultado   = 4 << 20
	maxWorker      = 64
)

// mensajeErrorInterno es el mismo que usa el worker para los fallos que no
// son culpa de la petición.
const mensajeErrorInterno = "No se pudo completar la simulación por un error interno. Inténtalo de nuevo más tarde."

// EventoWorker es una entrada del stream sim:estados.
type EventoWorker struct {
	Entrada   string
	ID        int64
	Tipo      string
	Intentos  int
	Worker    string
	Motivo    string
	Resultado []byte
	Error     string
}

// Estados aplica en MariaDB los cambios que el worker publica en
// sim:estados y avisa al SSE. Es la única vía por la que el worker informa a
// la API.
type Estados struct {
	cola   *ColaRedis
	svc    *Service
	rt     *Realtime
	logger *slog.Logger
	// bloqueo es cuánto espera cada lectura del stream (bloqueoEstados; los
	// tests lo acortan). Run tarda como mucho eso en terminar.
	bloqueo time.Duration
}

func NewEstados(cola *ColaRedis, svc *Service, rt *Realtime, logger *slog.Logger) *Estados {
	if logger == nil {
		logger = slog.Default()
	}
	return &Estados{cola: cola, svc: svc, rt: rt, logger: logger, bloqueo: bloqueoEstados}
}

// Run lee y aplica los estados hasta que ctx termine. Cada cierto tiempo
// reclama los que se leyeron y no se confirmaron (una instancia que murió, o
// un fallo de MariaDB en esta).
func (e *Estados) Run(ctx context.Context) {
	for {
		err := e.cola.prepararGrupo(ctx)
		if err == nil {
			break
		}
		e.logger.Error("estados_grupo", "error", err)
		if !dormir(ctx, 2*time.Second) {
			return
		}
	}
	var ultimoReclamo time.Time
	for ctx.Err() == nil {
		if time.Since(ultimoReclamo) >= reclamarEstadosCada {
			ultimoReclamo = time.Now()
			if mensajes, err := e.cola.ReclamarEstados(ctx, estadoInactivo); err != nil {
				e.logger.Error("estados_reclamar", "error", err)
			} else {
				e.aplicarTodos(ctx, mensajes)
			}
		}
		if err := e.Procesar(ctx, e.bloqueo); err != nil {
			if ctx.Err() != nil {
				return
			}
			e.logger.Error("estados_leer", "error", err)
			dormir(ctx, time.Second)
		}
	}
}

// Procesar lee una tanda de estados nuevos y los aplica. Con `bloqueo`
// negativo no espera a que lleguen.
func (e *Estados) Procesar(ctx context.Context, bloqueo time.Duration) error {
	mensajes, err := e.cola.LeerEstados(ctx, bloqueo)
	if err != nil {
		return err
	}
	e.aplicarTodos(ctx, mensajes)
	return nil
}

func (e *Estados) aplicarTodos(ctx context.Context, mensajes []redis.XMessage) {
	for _, msg := range mensajes {
		if err := e.aplicar(ctx, msg); err != nil {
			// Sin confirmar: se reintentará al reclamarlo.
			e.logger.Error("estado_no_aplicado", "entrada", msg.ID, "error", err)
			continue
		}
		if err := e.cola.ConfirmarEstado(ctx, msg.ID); err != nil {
			e.logger.Error("estado_no_confirmado", "entrada", msg.ID, "error", err)
		}
	}
}

func (e *Estados) aplicar(ctx context.Context, msg redis.XMessage) error {
	ev, err := leerEstado(msg)
	if err != nil {
		// No tiene arreglo: se registra y se confirma para no atascar el stream.
		e.logger.Warn("estado_invalido", "entrada", msg.ID, "error", err)
		return nil
	}
	if ev.Tipo == EstadoCompletado && !resultadoValido(ev.Resultado) {
		e.logger.Error("resultado_invalido", "id", ev.ID, "bytes", len(ev.Resultado))
		ev.Tipo, ev.Resultado, ev.Error = EstadoError, nil, mensajeErrorInterno
	}
	cambio, err := e.svc.AplicarEstado(ctx, ev)
	if err != nil {
		return err
	}
	e.logger.Info("estado_recibido", "id", ev.ID, "tipo", ev.Tipo, "motivo", ev.Motivo, "intentos", ev.Intentos, "aplicado", cambio)
	if cambio {
		e.rt.Notificar(ctx, ev.ID)
	}
	return nil
}

func leerEstado(msg redis.XMessage) (EventoWorker, error) {
	texto := func(campo string) string {
		v, _ := msg.Values[campo].(string)
		return v
	}
	ev := EventoWorker{
		Entrada:   msg.ID,
		Tipo:      texto("tipo"),
		Worker:    texto("worker"),
		Motivo:    texto("motivo"),
		Resultado: []byte(texto("resultado")),
		Error:     texto("error"),
	}
	id, err := strconv.ParseInt(texto("id"), 10, 64)
	if err != nil || id <= 0 {
		return ev, fmt.Errorf("id inválido: %q", texto("id"))
	}
	ev.ID = id
	if ev.Intentos, err = strconv.Atoi(texto("intentos")); err != nil || ev.Intentos < 0 {
		return ev, fmt.Errorf("intentos inválidos: %q", texto("intentos"))
	}
	switch ev.Tipo {
	case EstadoProcesando, EstadoPendiente, EstadoCompletado:
	case EstadoError:
		if ev.Error == "" {
			ev.Error = mensajeErrorInterno
		}
	default:
		return ev, fmt.Errorf("tipo desconocido: %q", ev.Tipo)
	}
	if len(ev.Worker) > maxWorker {
		ev.Worker = ev.Worker[:maxWorker]
	}
	return ev, nil
}

func resultadoValido(resultado []byte) bool {
	return len(resultado) > 0 && len(resultado) <= maxResultado && json.Valid(resultado)
}

// dormir espera `d` o hasta que ctx termine; devuelve false en ese caso.
func dormir(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
