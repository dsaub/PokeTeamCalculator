package simulaciones

import (
	"context"
	"time"
)

// CancelAbandonedLoop cancela trabajos pendientes que llevan demasiado tiempo
// sin nadie escuchando. Se usa `creado` como referencia para cubrir también
// los que nunca llegaron a abrir el SSE.
func (rt *Realtime) CancelAbandonedLoop(ctx context.Context) {
	espera := rt.svc.cfg.CancelarSinCliente
	if espera <= 0 {
		return
	}
	intervalo := espera / 2
	if intervalo < time.Second {
		intervalo = time.Second
	}
	if intervalo > 30*time.Second {
		intervalo = 30 * time.Second
	}

	ticker := time.NewTicker(intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.cancelarAbandonados(ctx, espera)
		}
	}
}

func (rt *Realtime) cancelarAbandonados(ctx context.Context, espera time.Duration) {
	ids, err := rt.svc.AbandonedPending(ctx, time.Now().Add(-espera))
	if err != nil {
		rt.logger.Error("cancelar_abandonados", "error", err)
		return
	}
	for _, id := range ids {
		desde, hubo, tiene := rt.hub.AbandonadoDesde(id)
		if tiene {
			continue
		}
		if hubo && time.Since(desde) < espera {
			continue
		}
		ok, err := rt.svc.CancelIfPending(ctx, id)
		if err != nil {
			rt.logger.Error("cancelar_abandonados", "id", id, "error", err)
			continue
		}
		if ok {
			rt.logger.Info("simulacion_cancelada", "id", id, "motivo", "sin_cliente")
			rt.PublishCancel(ctx, id)
		}
	}
}

// CleanupLoop borra trabajos antiguos cada hora. Nunca toca pendientes ni
// procesando.
func (rt *Realtime) CleanupLoop(ctx context.Context) {
	rt.limpiar(ctx)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.limpiar(ctx)
		}
	}
}

func (rt *Realtime) limpiar(ctx context.Context) {
	counts, err := rt.svc.Cleanup(ctx)
	if err != nil {
		rt.logger.Error("limpieza_simulaciones", "error", err)
		return
	}
	if counts.Errores > 0 || counts.Completados > 0 {
		rt.logger.Info("limpieza_simulaciones", "errores", counts.Errores, "completados", counts.Completados)
	}
}

const (
	intervaloReconciliacion = 30 * time.Second
	// margenReconciliacion deja fuera los trabajos recién creados, que la API
	// está encolando en ese momento.
	margenReconciliacion = 10 * time.Second
)

// ReconciliarLoop mantiene Redis de acuerdo con MariaDB, que es la fuente de
// verdad: al arrancar y cada 30 segundos.
func (rt *Realtime) ReconciliarLoop(ctx context.Context) {
	rt.Reconciliar(ctx)
	ticker := time.NewTicker(intervaloReconciliacion)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rt.Reconciliar(ctx)
		}
	}
}

// Reconciliar vuelve a encolar, al final, los trabajos activos que Redis no
// conoce (se cayó al crearlos o perdió sus datos) y saca de la cola los que ya
// terminaron. Solo corre con todos los estados del worker ya aplicados, para
// no adelantarse a un final que todavía no ha llegado a MariaDB.
func (rt *Realtime) Reconciliar(ctx context.Context) {
	sinAplicar, err := rt.cola.EstadosSinAplicar(ctx)
	if err != nil {
		rt.logger.Error("reconciliar", "error", err)
		return
	}
	if sinAplicar > 0 {
		return
	}

	activos, err := rt.svc.Activos(ctx, time.Now().Add(-margenReconciliacion))
	if err != nil {
		rt.logger.Error("reconciliar", "error", err)
		return
	}
	cambios := false
	for _, a := range activos {
		pos, err := rt.cola.Encolar(ctx, a.ID, a.Peticion, a.Intentos)
		if err != nil {
			rt.logger.Error("reconciliar", "id", a.ID, "error", err)
			return
		}
		if pos == 0 {
			continue
		}
		rt.logger.Warn("trabajo_reencolado", "id", a.ID, "posicion", pos, "motivo", "falta en Redis")
		// Si estaba en proceso, el worker que lo tenía ya no puede publicarlo.
		if ok, err := rt.svc.AplicarEstado(ctx, EventoWorker{ID: a.ID, Tipo: EstadoPendiente, Intentos: a.Intentos}); err != nil {
			rt.logger.Error("reconciliar", "id", a.ID, "error", err)
		} else if ok {
			rt.Notificar(ctx, a.ID)
		}
		cambios = true
	}

	posiciones, err := rt.cola.Posiciones(ctx)
	if err != nil {
		rt.logger.Error("reconciliar", "error", err)
		return
	}
	ids := make([]int64, 0, len(posiciones))
	for id := range posiciones {
		ids = append(ids, id)
	}
	estados, err := rt.svc.EstadosDe(ctx, ids)
	if err != nil {
		rt.logger.Error("reconciliar", "error", err)
		return
	}
	for _, id := range ids {
		if e := estados[id]; e == EstadoPendiente || e == EstadoProcesando {
			continue
		}
		if err := rt.cola.Cancelar(ctx, id); err != nil {
			rt.logger.Error("reconciliar", "id", id, "error", err)
			continue
		}
		rt.logger.Warn("trabajo_quitado_de_la_cola", "id", id, "estado", estados[id])
		cambios = true
	}
	if cambios {
		rt.PublishAllPositions(ctx)
	}
}
