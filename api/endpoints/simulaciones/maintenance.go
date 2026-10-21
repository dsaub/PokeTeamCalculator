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
			rt.hub.PublishFinal(id, eventoCancelado())
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
