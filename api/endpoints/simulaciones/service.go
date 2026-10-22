package simulaciones

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"
)

type CreateResponse struct {
	ID               int64
	Estado           string
	Posicion         int
	EstimadoSegundos int
	Cache            bool
	Duplicado        bool
	Resultado        []byte
	WorkerDisponible bool
}

type JobView struct {
	ID               int64
	Estado           string
	Posicion         int
	EstimadoSegundos int
	Resultado        []byte
	Error            string
}

type Service struct {
	repo    Repository
	cfg     Config
	cola    *ColaRedis
	est     *Estimator
	logger  *slog.Logger
	catalog Catalog
}

func NewService(repo Repository, cfg Config, cola *ColaRedis, logger *slog.Logger, cataloges ...Catalog) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &Service{
		repo:   repo,
		cfg:    cfg,
		cola:   cola,
		est:    NewEstimator(repo, cfg.TrabajosSimultaneos),
		logger: logger,
	}
	if len(cataloges) > 0 {
		svc.catalog = cataloges[0]
	}
	return svc
}

func (s *Service) Create(ctx context.Context, usuario string, p Peticion) (CreateResponse, error) {
	if generacionDe(p.Gen) == 0 {
		return CreateResponse{}, errorCampo("gen", "debe ser gen1, gen2, …, gen9")
	}
	if err := validarOpciones(p.Opciones); err != nil {
		return CreateResponse{}, err
	}
	p.Simulaciones = s.cfg.Simulaciones
	if p.Opciones == nil {
		p.Opciones = &Opciones{}
	}
	aleatoriedad := s.cfg.AleatoriedadRival
	p.Opciones.AleatoriedadRival = &aleatoriedad

	// Antes de hashear y guardar: fuera campos que el worker rechaza en
	// generaciones antiguas y habilidades que no existen para esa especie/gen.
	SanearPorGeneracion(&p)
	s.revisarHabilidades(ctx, &p)

	hash, err := HashPeticion(p)
	if err != nil {
		return CreateResponse{}, err
	}
	original, err := json.Marshal(p)
	if err != nil {
		return CreateResponse{}, err
	}

	out, err := s.repo.CreateOrFind(ctx, CreateInput{
		Hash:          hash,
		Usuario:       usuario,
		Peticion:      original,
		CacheDesde:    time.Now().Add(-s.cfg.CacheTTL),
		MaxPendientes: s.cfg.MaxPendientesPorUsuario,
		MaxCola:       s.cfg.MaxCola,
	})
	if err != nil {
		return CreateResponse{}, err
	}

	switch {
	case out.Cached:
		s.logger.Info("simulacion_creada", "id", out.Job.ID, "cache", true, "duplicada", false)
		return CreateResponse{
			ID:        out.Job.ID,
			Estado:    out.Job.Estado,
			Cache:     true,
			Resultado: out.Job.Resultado,
		}, nil

	case out.Duplicado:
		pos := 0
		if out.Job.Estado == EstadoPendiente {
			pos = s.posicion(ctx, out.Job.ID)
		}
		s.logger.Info("simulacion_creada", "id", out.Job.ID, "cache", false, "duplicada", true)
		return CreateResponse{
			ID:               out.Job.ID,
			Estado:           out.Job.Estado,
			Posicion:         pos,
			EstimadoSegundos: s.EstimateSeconds(ctx, pos),
			Duplicado:        true,
			WorkerDisponible: s.workerDisponibleAhora(ctx),
		}, nil

	case out.LimitUsuario:
		return CreateResponse{}, &LimiteUsuarioError{ID: out.ActivoID}

	case out.LimitGlobal:
		return CreateResponse{}, ErrLimiteGlobal
	}

	// Nuevo trabajo: al final de la cola de Redis. Si Redis falla, el trabajo
	// ya está en MariaDB y lo encolará la reconciliación.
	disponible := s.workerDisponibleAhora(ctx)
	pos, err := s.cola.Encolar(ctx, out.Job.ID, out.Job.Peticion, 0)
	if err != nil {
		s.logger.Error("cola_no_disponible", "id", out.Job.ID, "error", err)
		disponible = false
	}
	if pos == 0 {
		// Ya estaba encolado (la reconciliación se adelantó) o Redis no responde.
		pos = s.posicion(ctx, out.Job.ID)
	}
	if !disponible {
		s.logger.Warn("worker_no_disponible", "id", out.Job.ID)
	}
	s.logger.Info("simulacion_creada", "id", out.Job.ID, "cache", false, "duplicada", false)
	return CreateResponse{
		ID:               out.Job.ID,
		Estado:           out.Job.Estado,
		Posicion:         pos,
		EstimadoSegundos: s.EstimateSeconds(ctx, pos),
		WorkerDisponible: disponible,
	}, nil
}

func (s *Service) GetForUser(ctx context.Context, id int64, usuario string) (*JobView, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoEncontrado
		}
		return nil, err
	}
	// Los resultados completados son públicos; el resto solo para su dueño.
	if job.Estado != EstadoCompletado && job.Usuario != usuario {
		return nil, ErrNoEncontrado
	}
	return s.view(ctx, job)
}

func (s *Service) GetByID(ctx context.Context, id int64) (*JobView, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoEncontrado
		}
		return nil, err
	}
	return s.view(ctx, job)
}

// GetJob devuelve la fila cruda; lo usa el SSE (Realtime.Refrescar), que no
// tiene usuario y necesita el resultado/error originales.
func (s *Service) GetJob(ctx context.Context, id int64) (*Job, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoEncontrado
		}
		return nil, err
	}
	return job, nil
}

func (s *Service) view(ctx context.Context, job *Job) (*JobView, error) {
	v := &JobView{
		ID:     job.ID,
		Estado: job.Estado,
	}
	switch job.Estado {
	case EstadoPendiente:
		v.Posicion = s.posicion(ctx, job.ID)
		v.EstimadoSegundos = s.EstimateSeconds(ctx, v.Posicion)
	case EstadoCompletado:
		v.Resultado = job.Resultado
	case EstadoError:
		v.Error = job.Error.String
	}
	return v, nil
}

func (s *Service) Cancel(ctx context.Context, id int64, usuario string) error {
	out, err := s.repo.Cancel(ctx, id, usuario)
	if err != nil {
		return err
	}
	switch out {
	case NoEncontrado:
		return ErrNoEncontrado
	case YaTerminado:
		return ErrYaTerminado
	}
	s.logger.Info("simulacion_cancelada", "id", id)
	s.cancelarEnCola(ctx, id)
	return nil
}

func (s *Service) CancelIfPending(ctx context.Context, id int64) (bool, error) {
	ok, err := s.repo.CancelIfPending(ctx, id)
	if ok {
		s.cancelarEnCola(ctx, id)
	}
	return ok, err
}

// cancelarEnCola avisa al worker por Redis. Si Redis falla, el trabajo ya está
// cancelado en MariaDB: la reconciliación lo sacará de la cola y la API
// ignorará su resultado.
func (s *Service) cancelarEnCola(ctx context.Context, id int64) {
	if err := s.cola.Cancelar(ctx, id); err != nil {
		s.logger.Warn("cancelar_en_cola", "id", id, "error", err)
	}
}

func (s *Service) GetMany(ctx context.Context, ids []int64) ([]Job, error) {
	return s.repo.GetMany(ctx, ids)
}

// Posiciones devuelve la posición en la cola de cada trabajo pendiente.
func (s *Service) Posiciones(ctx context.Context) (map[int64]int, error) {
	return s.cola.Posiciones(ctx)
}

// posicion devuelve la posición de un trabajo pendiente. Si Redis no lo tiene
// en la cola (un worker acaba de tomarlo y aún no ha llegado su `procesando`)
// o no responde, se da como el siguiente.
func (s *Service) posicion(ctx context.Context, id int64) int {
	posiciones, err := s.cola.Posiciones(ctx)
	if err != nil {
		s.logger.Warn("posicion_no_disponible", "id", id, "error", err)
		return 1
	}
	return max(posiciones[id], 1)
}

// AplicarEstado lleva a MariaDB un cambio que publicó el worker. Devuelve
// true si la fila cambió.
func (s *Service) AplicarEstado(ctx context.Context, ev EventoWorker) (bool, error) {
	switch ev.Tipo {
	case EstadoProcesando:
		return s.repo.MarcarProcesando(ctx, ev.ID, ev.Worker, ev.Intentos)
	case EstadoPendiente:
		return s.repo.MarcarPendiente(ctx, ev.ID, ev.Intentos)
	case EstadoCompletado:
		return s.repo.Completar(ctx, ev.ID, ev.Resultado)
	case EstadoError:
		return s.repo.Fallar(ctx, ev.ID, ev.Error)
	default:
		return false, fmt.Errorf("tipo de estado desconocido: %q", ev.Tipo)
	}
}

func (s *Service) Activos(ctx context.Context, antesDe time.Time) ([]Activo, error) {
	return s.repo.ListActivos(ctx, antesDe)
}

func (s *Service) EstadosDe(ctx context.Context, ids []int64) (map[int64]string, error) {
	return s.repo.EstadosDe(ctx, ids)
}

func (s *Service) AbandonedPending(ctx context.Context, antesDe time.Time) ([]int64, error) {
	return s.repo.ListAbandonedPending(ctx, antesDe)
}

func (s *Service) Cleanup(ctx context.Context) (CleanupCounts, error) {
	return s.repo.Cleanup(ctx, s.cfg.Retencion)
}

func (s *Service) EstimateSeconds(ctx context.Context, posicion int) int {
	return s.est.Seconds(ctx, posicion)
}

func (s *Service) WorkerDisponible() bool {
	return s.cola.Disponible()
}

// workerDisponibleAhora repite la comprobación si la última no vio ningún
// worker: uno que acaba de arrancar tarda hasta 15 s en aparecer en ella.
func (s *Service) workerDisponibleAhora(ctx context.Context) bool {
	return s.cola.Disponible() || s.cola.Comprobar(ctx)
}

// ColaResponde indica si Redis contesta ahora mismo.
func (s *Service) ColaResponde(ctx context.Context) bool {
	return s.cola.Ping(ctx) == nil
}

// Estimator guarda en memoria la duración media de los últimos 20 trabajos
// completados y la recalcula como mucho cada 30 segundos.
type Estimator struct {
	mu          sync.Mutex
	repo        Repository
	simultaneos int
	media       time.Duration
	ultimoCalc  time.Time
}

func NewEstimator(repo Repository, simultaneos int) *Estimator {
	if simultaneos < 1 {
		simultaneos = 1
	}
	return &Estimator{
		repo:        repo,
		simultaneos: simultaneos,
		media:       10 * time.Second,
	}
}

func (e *Estimator) Seconds(ctx context.Context, posicion int) int {
	if posicion <= 0 {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.ultimoCalc.IsZero() || time.Since(e.ultimoCalc) >= 30*time.Second {
		duraciones, err := e.repo.LastDurations(ctx, 20)
		if err == nil {
			e.media = mediaDuraciones(duraciones)
		}
		// También fijamos la marca si falla, para no golpear la base de datos
		// en cada petición mientras haya un problema.
		e.ultimoCalc = time.Now()
	}

	media := e.media
	if media <= 0 {
		media = 10 * time.Second
	}
	segundos := float64(posicion) * media.Seconds() / float64(e.simultaneos)
	return int(math.Ceil(segundos))
}

func mediaDuraciones(duraciones []time.Duration) time.Duration {
	if len(duraciones) == 0 {
		return 10 * time.Second
	}
	var total time.Duration
	for _, d := range duraciones {
		total += d
	}
	media := total / time.Duration(len(duraciones))
	if media <= 0 {
		return 10 * time.Second
	}
	return media
}
