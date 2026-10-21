package simulaciones

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"
)

// Waker avisa al worker de que hay trabajo nuevo. La implementación real
// lanza una goroutine con timeout de 1 s y nunca bloquea la petición.
type Waker interface {
	Wake()
	Disponible() bool
}

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
	waker   Waker
	est     *Estimator
	logger  *slog.Logger
	catalog Catalog
}

func NewService(repo Repository, cfg Config, waker Waker, logger *slog.Logger, cataloges ...Catalog) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &Service{
		repo:   repo,
		cfg:    cfg,
		waker:  waker,
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
		est := 0
		if out.Job.Estado == EstadoPendiente && out.Posicion > 0 {
			est = s.EstimateSeconds(ctx, out.Posicion)
		}
		s.logger.Info("simulacion_creada", "id", out.Job.ID, "cache", false, "duplicada", true)
		return CreateResponse{
			ID:               out.Job.ID,
			Estado:           out.Job.Estado,
			Posicion:         out.Posicion,
			EstimadoSegundos: est,
			Duplicado:        true,
			WorkerDisponible: s.waker.Disponible(),
		}, nil

	case out.LimitUsuario:
		return CreateResponse{}, &LimiteUsuarioError{ID: out.ActivoID}

	case out.LimitGlobal:
		return CreateResponse{}, ErrLimiteGlobal
	}

	// Nuevo trabajo: despertar al worker en segundo plano.
	s.waker.Wake()
	disponible := s.waker.Disponible()
	if !disponible {
		s.logger.Warn("worker_no_disponible", "id", out.Job.ID)
	}
	est := s.EstimateSeconds(ctx, out.Posicion)
	s.logger.Info("simulacion_creada", "id", out.Job.ID, "cache", false, "duplicada", false)
	return CreateResponse{
		ID:               out.Job.ID,
		Estado:           out.Job.Estado,
		Posicion:         out.Posicion,
		EstimadoSegundos: est,
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

// GetJob devuelve la fila cruda; lo usa el callback interno, que no tiene
// usuario y necesita el resultado/error originales.
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
		pos, err := s.repo.Position(ctx, job.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrNoEncontrado
			}
			return nil, err
		}
		v.Posicion = pos
		v.EstimadoSegundos = s.EstimateSeconds(ctx, pos)
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
	return nil
}

func (s *Service) CancelIfPending(ctx context.Context, id int64) (bool, error) {
	return s.repo.CancelIfPending(ctx, id)
}

func (s *Service) GetMany(ctx context.Context, ids []int64) ([]JobConPosicion, error) {
	return s.repo.GetMany(ctx, ids)
}

func (s *Service) PendingPositions(ctx context.Context) (map[int64]int, error) {
	return s.repo.PendingPositions(ctx)
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
	return s.waker.Disponible()
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
