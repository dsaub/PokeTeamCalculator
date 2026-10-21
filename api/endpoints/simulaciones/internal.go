package simulaciones

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// NewInternalHandler sirve los endpoints internos. Se monta en un servidor
// aparte que escucha solo en 127.0.0.1 para que nunca quede expuesto.
func NewInternalHandler(svc *Service, rt *Realtime, token string, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	esperado := "Bearer " + token

	mux := http.NewServeMux()
	mux.HandleFunc("POST /interno/simulaciones/{id}/lista", func(w http.ResponseWriter, r *http.Request) {
		autorizacion := r.Header.Get("Authorization")
		if subtle.ConstantTimeCompare([]byte(autorizacion), []byte(esperado)) != 1 {
			responderJSON(w, http.StatusUnauthorized, map[string]string{"error": "No autorizado"})
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			responderJSON(w, http.StatusNotFound, map[string]string{"error": "Simulación no encontrada"})
			return
		}

		job, err := svc.GetJob(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrNoEncontrado) {
				responderJSON(w, http.StatusNotFound, map[string]string{"error": "Simulación no encontrada"})
				return
			}
			logger.Error("callback_recibido", "id", id, "error", err)
			responderJSON(w, http.StatusInternalServerError, map[string]string{"error": "Error interno"})
			return
		}

		logger.Info("callback_recibido", "id", id, "estado", job.Estado)
		if job.Estado == EstadoCompletado || job.Estado == EstadoError || job.Estado == EstadoCancelado {
			// El aviso al hub es en memoria y rápido. El recálculo de
			// posiciones sí va en segundo plano para no retrasar al worker.
			rt.PublishFinal(job)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				rt.PublishAllPositions(ctx)
			}()
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func responderJSON(w http.ResponseWriter, codigo int, cuerpo any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(cuerpo)
}
