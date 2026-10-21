package simulaciones

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	timeoutDespertar = time.Second
	timeoutSalud     = 3 * time.Second
)

// WorkerClient habla con el worker Bun. Se usa para despertarlo y para
// conocer su salud; la cola real vive en MariaDB.
type WorkerClient struct {
	baseURL    string
	token      string
	client     *http.Client
	disponible atomic.Bool
	logger     *slog.Logger
}

func NewWorkerClient(baseURL, token string, logger *slog.Logger) *WorkerClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &WorkerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: timeoutSalud},
		logger:  logger,
	}
}

// Wake lanza el aviso en segundo plano: la API no espera al worker.
func (w *WorkerClient) Wake() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeoutDespertar)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/despertar", nil)
		if err != nil {
			w.logger.Warn("worker_no_disponible", "motivo", "petición inválida", "error", err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+w.token)

		resp, err := w.client.Do(req)
		if err != nil {
			w.logger.Warn("worker_no_disponible", "motivo", "despertar", "error", err)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= 300 {
			w.logger.Warn("worker_no_disponible", "motivo", "despertar", "status", resp.StatusCode)
		}
	}()
}

func (w *WorkerClient) Disponible() bool {
	return w.disponible.Load()
}

// Check consulta GET /salud y actualiza el estado en memoria.
func (w *WorkerClient) Check(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, timeoutSalud)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.baseURL+"/salud", nil)
	if err != nil {
		w.cambiarDisponibilidad(false)
		return false
	}
	resp, err := w.client.Do(req)
	if err != nil {
		w.cambiarDisponibilidad(false)
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	ok := resp.StatusCode == http.StatusOK
	w.cambiarDisponibilidad(ok)
	return ok
}

func (w *WorkerClient) cambiarDisponibilidad(ok bool) {
	antes := w.disponible.Swap(ok)
	if antes && !ok {
		w.logger.Warn("worker_no_disponible", "motivo", "salud")
	}
}

// Run comprueba la salud al arrancar y después cada 15 segundos.
func (w *WorkerClient) Run(ctx context.Context) {
	w.Check(ctx)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Check(ctx)
		}
	}
}
