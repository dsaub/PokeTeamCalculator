package simulaciones

import (
	"context"
	"database/sql"
	"sort"
	"sync"
	"time"
)

// memRepo es una implementación en memoria para las pruebas sin MariaDB.
type memRepo struct {
	mu         sync.Mutex
	jobs       map[int64]*Job
	nextID     int64
	ahora      func() time.Time
	duraciones []time.Duration
}

func newMemRepo() *memRepo {
	return &memRepo{jobs: make(map[int64]*Job), ahora: time.Now}
}

func (m *memRepo) addJob(j Job) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	j.ID = m.nextID
	if j.Creado.IsZero() {
		j.Creado = m.ahora()
	}
	m.jobs[j.ID] = copiarJob(&j)
	return copiarJob(m.jobs[j.ID])
}

func (m *memRepo) setEstado(id int64, estado string, resultado []byte, mensajeError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil {
		return
	}
	j.Estado = estado
	j.Resultado = append([]byte(nil), resultado...)
	if mensajeError != "" {
		j.Error = sql.NullString{String: mensajeError, Valid: true}
	}
	if estado == EstadoCompletado || estado == EstadoError || estado == EstadoCancelado {
		j.Terminado = sql.NullTime{Time: m.ahora(), Valid: true}
	}
}

func (m *memRepo) setProcesando(id int64) {
	m.setEstado(id, EstadoProcesando, nil, "")
}

func (m *memRepo) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.jobs)
}

func (m *memRepo) CreateOrFind(_ context.Context, in CreateInput) (CreateOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cache *Job
	for _, j := range m.jobs {
		if j.Estado != EstadoCompletado || j.Hash != in.Hash || !j.Terminado.Valid {
			continue
		}
		if j.Terminado.Time.Before(in.CacheDesde) {
			continue
		}
		if cache == nil || j.Terminado.Time.After(cache.Terminado.Time) {
			cache = j
		}
	}
	if cache != nil {
		return CreateOutcome{Job: copiarJob(cache), Cached: true}, nil
	}

	var duplicado *Job
	for _, j := range m.jobs {
		if j.Usuario != in.Usuario || j.Hash != in.Hash {
			continue
		}
		if j.Estado != EstadoPendiente && j.Estado != EstadoProcesando {
			continue
		}
		if duplicado == nil || j.ID < duplicado.ID {
			duplicado = j
		}
	}
	if duplicado != nil {
		pos := 0
		if duplicado.Estado == EstadoPendiente {
			pos = m.posicionLocked(duplicado)
		}
		return CreateOutcome{Job: copiarJob(duplicado), Posicion: pos, Duplicado: true}, nil
	}

	activos := 0
	for _, j := range m.jobs {
		if j.Usuario == in.Usuario && (j.Estado == EstadoPendiente || j.Estado == EstadoProcesando) {
			activos++
		}
	}
	if activos >= in.MaxPendientes {
		var activo *Job
		for _, j := range m.jobs {
			if j.Usuario == in.Usuario && (j.Estado == EstadoPendiente || j.Estado == EstadoProcesando) {
				if activo == nil || j.Creado.Before(activo.Creado) || (j.Creado.Equal(activo.Creado) && j.ID < activo.ID) {
					activo = j
				}
			}
		}
		var id int64
		if activo != nil {
			id = activo.ID
		}
		return CreateOutcome{LimitUsuario: true, ActivoID: id}, nil
	}

	pendientes := 0
	for _, j := range m.jobs {
		if j.Estado == EstadoPendiente {
			pendientes++
		}
	}
	if pendientes >= in.MaxCola {
		return CreateOutcome{LimitGlobal: true}, nil
	}

	m.nextID++
	job := &Job{
		ID:       m.nextID,
		Hash:     in.Hash,
		Usuario:  in.Usuario,
		Peticion: append([]byte(nil), in.Peticion...),
		Estado:   EstadoPendiente,
		Creado:   m.ahora(),
	}
	m.jobs[job.ID] = job
	return CreateOutcome{Job: copiarJob(job), Posicion: m.posicionLocked(job)}, nil
}

func (m *memRepo) Get(_ context.Context, id int64) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil {
		return nil, sql.ErrNoRows
	}
	return copiarJob(j), nil
}

func (m *memRepo) GetMany(_ context.Context, ids []int64) ([]JobConPosicion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []JobConPosicion
	for _, id := range ids {
		j := m.jobs[id]
		if j == nil {
			continue
		}
		pos := 0
		if j.Estado == EstadoPendiente {
			pos = m.posicionLocked(j)
		}
		out = append(out, JobConPosicion{Job: *copiarJob(j), Posicion: pos})
	}
	return out, nil
}

func (m *memRepo) Position(_ context.Context, id int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil {
		return 0, sql.ErrNoRows
	}
	return m.posicionLocked(j), nil
}

func (m *memRepo) PendingPositions(context.Context) (map[int64]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var pendientes []*Job
	for _, j := range m.jobs {
		if j.Estado == EstadoPendiente {
			pendientes = append(pendientes, j)
		}
	}
	sort.Slice(pendientes, func(i, j int) bool {
		if pendientes[i].Creado.Equal(pendientes[j].Creado) {
			return pendientes[i].ID < pendientes[j].ID
		}
		return pendientes[i].Creado.Before(pendientes[j].Creado)
	})
	out := make(map[int64]int, len(pendientes))
	for i, j := range pendientes {
		out[j.ID] = i + 1
	}
	return out, nil
}

func (m *memRepo) Cancel(_ context.Context, id int64, usuario string) (CancelOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil || j.Usuario != usuario {
		return NoEncontrado, nil
	}
	if j.Estado != EstadoPendiente && j.Estado != EstadoProcesando {
		return YaTerminado, nil
	}
	j.Estado = EstadoCancelado
	j.Terminado = sql.NullTime{Time: m.ahora(), Valid: true}
	return Cancelado, nil
}

func (m *memRepo) CancelIfPending(_ context.Context, id int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil || j.Estado != EstadoPendiente {
		return false, nil
	}
	j.Estado = EstadoCancelado
	j.Terminado = sql.NullTime{Time: m.ahora(), Valid: true}
	return true, nil
}

func (m *memRepo) LastDurations(context.Context, int) ([]time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]time.Duration(nil), m.duraciones...), nil
}

func (m *memRepo) Cleanup(context.Context, time.Duration) (CleanupCounts, error) {
	return CleanupCounts{}, nil
}

func (m *memRepo) ListAbandonedPending(_ context.Context, antesDe time.Time) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []int64
	for id, j := range m.jobs {
		if j.Estado == EstadoPendiente && j.Creado.Before(antesDe) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (m *memRepo) posicionLocked(job *Job) int {
	pos := 1
	for _, otro := range m.jobs {
		if otro.ID == job.ID || otro.Estado != EstadoPendiente {
			continue
		}
		if otro.Creado.Before(job.Creado) || (otro.Creado.Equal(job.Creado) && otro.ID < job.ID) {
			pos++
		}
	}
	return pos
}

func copiarJob(j *Job) *Job {
	c := *j
	c.Peticion = append([]byte(nil), j.Peticion...)
	c.Resultado = append([]byte(nil), j.Resultado...)
	return &c
}

type wakerFalso struct {
	mu         sync.Mutex
	llamadas   int
	disponible bool
}

func (w *wakerFalso) Wake() {
	w.mu.Lock()
	w.llamadas++
	w.mu.Unlock()
}

func (w *wakerFalso) Disponible() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.disponible
}

func (w *wakerFalso) numLlamadas() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.llamadas
}
