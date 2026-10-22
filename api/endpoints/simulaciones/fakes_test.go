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
	intentos   map[int64]int
	workers    map[int64]string
	nextID     int64
	ahora      func() time.Time
	duraciones []time.Duration
	// errEstados, si no es nil, lo devuelven los métodos que aplican los
	// estados del worker (simula un fallo de MariaDB).
	errEstados error
	// errores simula un fallo de MariaDB en un método concreto, por nombre.
	errores map[string]error
	// limpieza es lo que devuelve Cleanup; retencion, lo último que recibió.
	limpieza  CleanupCounts
	retencion time.Duration
	// llamadas cuenta las llamadas a cada método que lo registra.
	llamadas map[string]int
}

func newMemRepo() *memRepo {
	return &memRepo{
		jobs:     make(map[int64]*Job),
		intentos: make(map[int64]int),
		workers:  make(map[int64]string),
		errores:  make(map[string]error),
		llamadas: make(map[string]int),
		ahora:    time.Now,
	}
}

// fallarEn hace que `metodo` devuelva err hasta que se llame con nil.
func (m *memRepo) fallarEn(metodo string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errores[metodo] = err
}

// errDe registra la llamada y devuelve el error configurado. Con m.mu tomado.
func (m *memRepo) errDe(metodo string) error {
	m.llamadas[metodo]++
	return m.errores[metodo]
}

func (m *memRepo) numLlamadas(metodo string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.llamadas[metodo]
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

// envejecer adelanta la creación del trabajo, para que la reconciliación no
// lo tome por recién creado.
func (m *memRepo) envejecer(id int64, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[id]; j != nil {
		j.Creado = j.Creado.Add(-d)
	}
}

func (m *memRepo) estado(id int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[id]; j != nil {
		return j.Estado
	}
	return ""
}

func (m *memRepo) setErrEstados(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errEstados = err
}

func (m *memRepo) CreateOrFind(_ context.Context, in CreateInput) (CreateOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("CreateOrFind"); err != nil {
		return CreateOutcome{}, err
	}

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
		return CreateOutcome{Job: copiarJob(duplicado), Duplicado: true}, nil
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
	return CreateOutcome{Job: copiarJob(job)}, nil
}

func (m *memRepo) Get(_ context.Context, id int64) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("Get"); err != nil {
		return nil, err
	}
	j := m.jobs[id]
	if j == nil {
		return nil, sql.ErrNoRows
	}
	return copiarJob(j), nil
}

func (m *memRepo) GetMany(_ context.Context, ids []int64) ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("GetMany"); err != nil {
		return nil, err
	}
	var out []Job
	for _, id := range ids {
		if j := m.jobs[id]; j != nil {
			out = append(out, *copiarJob(j))
		}
	}
	return out, nil
}

func (m *memRepo) Cancel(_ context.Context, id int64, usuario string) (CancelOutcome, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("Cancel"); err != nil {
		return NoEncontrado, err
	}
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
	if err := m.errDe("CancelIfPending"); err != nil {
		return false, err
	}
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
	if err := m.errDe("LastDurations"); err != nil {
		return nil, err
	}
	return append([]time.Duration(nil), m.duraciones...), nil
}

func (m *memRepo) Cleanup(_ context.Context, retencion time.Duration) (CleanupCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retencion = retencion
	if err := m.errDe("Cleanup"); err != nil {
		return CleanupCounts{}, err
	}
	return m.limpieza, nil
}

func (m *memRepo) ListAbandonedPending(_ context.Context, antesDe time.Time) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("ListAbandonedPending"); err != nil {
		return nil, err
	}
	var ids []int64
	for id, j := range m.jobs {
		if j.Estado == EstadoPendiente && j.Creado.Before(antesDe) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// activoLocked dice si el trabajo admite un estado del worker: los finales
// nunca se pisan.
func (m *memRepo) activoLocked(id int64) (*Job, bool) {
	j := m.jobs[id]
	return j, j != nil && (j.Estado == EstadoPendiente || j.Estado == EstadoProcesando)
}

func (m *memRepo) MarcarProcesando(_ context.Context, id int64, worker string, intentos int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errEstados != nil {
		return false, m.errEstados
	}
	j, ok := m.activoLocked(id)
	if !ok {
		return false, nil
	}
	j.Estado = EstadoProcesando
	m.intentos[id] = intentos
	m.workers[id] = worker
	return true, nil
}

func (m *memRepo) MarcarPendiente(_ context.Context, id int64, intentos int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errEstados != nil {
		return false, m.errEstados
	}
	j := m.jobs[id]
	if j == nil || j.Estado != EstadoProcesando {
		return false, nil
	}
	j.Estado = EstadoPendiente
	m.intentos[id] = intentos
	delete(m.workers, id)
	return true, nil
}

func (m *memRepo) Completar(_ context.Context, id int64, resultado []byte) (bool, error) {
	return m.terminar(id, EstadoCompletado, resultado, "")
}

func (m *memRepo) Fallar(_ context.Context, id int64, mensaje string) (bool, error) {
	return m.terminar(id, EstadoError, nil, mensaje)
}

func (m *memRepo) terminar(id int64, estado string, resultado []byte, mensaje string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errEstados != nil {
		return false, m.errEstados
	}
	j, ok := m.activoLocked(id)
	if !ok {
		return false, nil
	}
	j.Estado = estado
	j.Resultado = append([]byte(nil), resultado...)
	j.Error = sql.NullString{String: mensaje, Valid: mensaje != ""}
	j.Terminado = sql.NullTime{Time: m.ahora(), Valid: true}
	return true, nil
}

func (m *memRepo) ListActivos(_ context.Context, antesDe time.Time) ([]Activo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("ListActivos"); err != nil {
		return nil, err
	}
	var activos []*Job
	for id, j := range m.jobs {
		if _, ok := m.activoLocked(id); ok && j.Creado.Before(antesDe) {
			activos = append(activos, j)
		}
	}
	sort.Slice(activos, func(i, j int) bool {
		if activos[i].Creado.Equal(activos[j].Creado) {
			return activos[i].ID < activos[j].ID
		}
		return activos[i].Creado.Before(activos[j].Creado)
	})
	out := make([]Activo, 0, len(activos))
	for _, j := range activos {
		out = append(out, Activo{ID: j.ID, Peticion: append([]byte(nil), j.Peticion...), Intentos: m.intentos[j.ID]})
	}
	return out, nil
}

func (m *memRepo) EstadosDe(_ context.Context, ids []int64) (map[int64]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.errDe("EstadosDe"); err != nil {
		return nil, err
	}
	estados := make(map[int64]string, len(ids))
	for _, id := range ids {
		if j := m.jobs[id]; j != nil {
			estados[id] = j.Estado
		}
	}
	return estados, nil
}

func copiarJob(j *Job) *Job {
	c := *j
	c.Peticion = append([]byte(nil), j.Peticion...)
	c.Resultado = append([]byte(nil), j.Resultado...)
	return &c
}
