package simulaciones

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	EstadoPendiente  = "pendiente"
	EstadoProcesando = "procesando"
	EstadoCompletado = "completado"
	EstadoError      = "error"
	EstadoCancelado  = "cancelado"
)

var (
	// ErrNoEncontrado se devuelve cuando el trabajo no existe o no pertenece al usuario.
	ErrNoEncontrado = errors.New("simulación no encontrada")
	// ErrYaTerminado se devuelve al cancelar un trabajo que ya había acabado.
	ErrYaTerminado = errors.New("la simulación ya había terminado")
	// ErrLimiteUsuario se devuelve cuando el usuario agota sus trabajos en curso.
	ErrLimiteUsuario = errors.New("ya tienes una simulación en cola")
	// ErrLimiteGlobal se devuelve cuando la cola global está llena.
	ErrLimiteGlobal = errors.New("la cola está llena, prueba en unos minutos")
)

// LimiteUsuarioError lleva el id del trabajo que ya tiene el usuario.
type LimiteUsuarioError struct {
	ID int64
}

func (e *LimiteUsuarioError) Error() string {
	return "Ya tienes una simulación en cola"
}

func (e *LimiteUsuarioError) Unwrap() error {
	return ErrLimiteUsuario
}

type Job struct {
	ID        int64
	Hash      string
	Usuario   string
	Peticion  []byte
	Estado    string
	Resultado []byte
	Error     sql.NullString
	Creado    time.Time
	Terminado sql.NullTime
}

type JobConPosicion struct {
	Job
	Posicion int
}

type CreateInput struct {
	Hash          string
	Usuario       string
	Peticion      []byte
	CacheDesde    time.Time
	MaxPendientes int
	MaxCola       int
}

type CreateOutcome struct {
	Job          *Job
	Posicion     int
	ActivoID     int64
	Cached       bool
	Duplicado    bool
	LimitUsuario bool
	LimitGlobal  bool
}

type CancelOutcome int

const (
	Cancelado CancelOutcome = iota
	NoEncontrado
	YaTerminado
)

type CleanupCounts struct {
	Errores     int64
	Completados int64
}

type Repository interface {
	CreateOrFind(ctx context.Context, in CreateInput) (CreateOutcome, error)
	Get(ctx context.Context, id int64) (*Job, error)
	GetMany(ctx context.Context, ids []int64) ([]JobConPosicion, error)
	Position(ctx context.Context, id int64) (int, error)
	PendingPositions(ctx context.Context) (map[int64]int, error)
	Cancel(ctx context.Context, id int64, usuario string) (CancelOutcome, error)
	CancelIfPending(ctx context.Context, id int64) (bool, error)
	LastDurations(ctx context.Context, limit int) ([]time.Duration, error)
	Cleanup(ctx context.Context, retencion time.Duration) (CleanupCounts, error)
	ListAbandonedPending(ctx context.Context, antesDe time.Time) ([]int64, error)
}

const columnasJob = `id, hash, usuario, peticion, estado, resultado, error, creado, terminado`

const (
	queryCache = `SELECT ` + columnasJob + `
FROM simulaciones
WHERE estado = 'completado' AND hash = ? AND terminado IS NOT NULL AND terminado >= ?
ORDER BY terminado DESC LIMIT 1`

	queryDuplicado = `SELECT ` + columnasJob + `
FROM simulaciones
WHERE usuario = ? AND hash = ? AND estado IN ('pendiente','procesando')
ORDER BY id LIMIT 1`

	queryGet = `SELECT ` + columnasJob + ` FROM simulaciones WHERE id = ?`

	queryPosicion = `SELECT COUNT(*) FROM simulaciones
WHERE estado = 'pendiente'
  AND (creado < ? OR (creado = ? AND id < ?))`
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) CreateOrFind(ctx context.Context, in CreateInput) (CreateOutcome, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return CreateOutcome{}, err
	}
	defer func() { _ = conn.Close() }()

	// Bloqueo de asesoramiento de MariaDB: serializa el flujo caché →
	// duplicado → límites → inserción. Es global y muy corto; evita que dos
	// peticiones simultáneas se salten los límites, incluso entre instancias.
	var lock sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 5)", "poketeam_simulaciones_crear").Scan(&lock); err != nil {
		return CreateOutcome{}, fmt.Errorf("no se pudo bloquear la cola: %w", err)
	}
	if !lock.Valid || lock.Int64 != 1 {
		return CreateOutcome{}, errors.New("la cola está ocupada, inténtalo de nuevo")
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "DO RELEASE_LOCK(?)", "poketeam_simulaciones_crear")
	}()

	// 1. Caché.
	if job, ok, err := getJobMaybe(ctx, conn, queryCache, in.Hash, in.CacheDesde); err != nil {
		return CreateOutcome{}, err
	} else if ok {
		return CreateOutcome{Job: job, Cached: true}, nil
	}

	// 2. Duplicado del mismo usuario.
	if job, ok, err := getJobMaybe(ctx, conn, queryDuplicado, in.Usuario, in.Hash); err != nil {
		return CreateOutcome{}, err
	} else if ok {
		pos := 0
		if job.Estado == EstadoPendiente {
			if pos, err = posicionDe(ctx, conn, job); err != nil {
				return CreateOutcome{}, err
			}
		}
		return CreateOutcome{Job: job, Posicion: pos, Duplicado: true}, nil
	}

	// 3. Límite por usuario.
	var activos int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM simulaciones WHERE usuario = ? AND estado IN ('pendiente','procesando')`,
		in.Usuario).Scan(&activos); err != nil {
		return CreateOutcome{}, err
	}
	if activos >= in.MaxPendientes {
		var activo int64
		err := conn.QueryRowContext(ctx,
			`SELECT id FROM simulaciones WHERE usuario = ? AND estado IN ('pendiente','procesando')
			 ORDER BY creado, id LIMIT 1`, in.Usuario).Scan(&activo)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return CreateOutcome{}, err
		}
		return CreateOutcome{LimitUsuario: true, ActivoID: activo}, nil
	}

	// 4. Límite global.
	var pendientes int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM simulaciones WHERE estado = 'pendiente'`).Scan(&pendientes); err != nil {
		return CreateOutcome{}, err
	}
	if pendientes >= in.MaxCola {
		return CreateOutcome{LimitGlobal: true}, nil
	}

	// 5. Encolar.
	res, err := conn.ExecContext(ctx,
		`INSERT INTO simulaciones (hash, usuario, peticion) VALUES (?, ?, ?)`,
		in.Hash, in.Usuario, in.Peticion)
	if err != nil {
		return CreateOutcome{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return CreateOutcome{}, err
	}
	job, err := getJob(ctx, conn, id)
	if err != nil {
		return CreateOutcome{}, err
	}
	pos, err := posicionDe(ctx, conn, job)
	if err != nil {
		return CreateOutcome{}, err
	}
	return CreateOutcome{Job: job, Posicion: pos}, nil
}

func (r *MySQLRepository) Get(ctx context.Context, id int64) (*Job, error) {
	return getJob(ctx, r.db, id)
}

func (r *MySQLRepository) GetMany(ctx context.Context, ids []int64) ([]JobConPosicion, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	marcadores := make([]string, len(ids))
	for i, id := range ids {
		args[i] = id
		marcadores[i] = "?"
	}
	query := `SELECT s.` + strings.ReplaceAll(columnasJob, ", ", ", s.") + `,
	CASE WHEN s.estado = 'pendiente' THEN (
	  SELECT COUNT(*) FROM simulaciones p
	  WHERE p.estado = 'pendiente'
	    AND (p.creado < s.creado OR (p.creado = s.creado AND p.id < s.id))
	) + 1 ELSE 0 END AS posicion
FROM simulaciones s WHERE s.id IN (` + strings.Join(marcadores, ",") + `)`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var jobs []JobConPosicion
	for rows.Next() {
		var j JobConPosicion
		if err := rows.Scan(&j.ID, &j.Hash, &j.Usuario, &j.Peticion, &j.Estado, &j.Resultado,
			&j.Error, &j.Creado, &j.Terminado, &j.Posicion); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (r *MySQLRepository) Position(ctx context.Context, id int64) (int, error) {
	job, err := getJob(ctx, r.db, id)
	if err != nil {
		return 0, err
	}
	return posicionDe(ctx, r.db, job)
}

func (r *MySQLRepository) PendingPositions(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, ROW_NUMBER() OVER (ORDER BY creado, id) AS posicion
FROM simulaciones WHERE estado = 'pendiente'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	posiciones := make(map[int64]int)
	for rows.Next() {
		var id int64
		var pos int
		if err := rows.Scan(&id, &pos); err != nil {
			return nil, err
		}
		posiciones[id] = pos
	}
	return posiciones, rows.Err()
}

func (r *MySQLRepository) Cancel(ctx context.Context, id int64, usuario string) (CancelOutcome, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE simulaciones SET estado = 'cancelado', terminado = NOW(3)
		 WHERE id = ? AND usuario = ? AND estado IN ('pendiente','procesando')`,
		id, usuario)
	if err != nil {
		return NoEncontrado, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return NoEncontrado, err
	}
	if n > 0 {
		return Cancelado, nil
	}

	var dueño, estado string
	err = r.db.QueryRowContext(ctx, `SELECT usuario, estado FROM simulaciones WHERE id = ?`, id).Scan(&dueño, &estado)
	if errors.Is(err, sql.ErrNoRows) {
		return NoEncontrado, nil
	}
	if err != nil {
		return NoEncontrado, err
	}
	if dueño != usuario {
		return NoEncontrado, nil
	}
	return YaTerminado, nil
}

func (r *MySQLRepository) CancelIfPending(ctx context.Context, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE simulaciones SET estado = 'cancelado', terminado = NOW(3)
		 WHERE id = ? AND estado = 'pendiente'`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *MySQLRepository) LastDurations(ctx context.Context, limit int) ([]time.Duration, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT iniciado, terminado FROM simulaciones
		 WHERE estado = 'completado' AND iniciado IS NOT NULL AND terminado IS NOT NULL
		 ORDER BY terminado DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var duraciones []time.Duration
	for rows.Next() {
		var inicio, fin time.Time
		if err := rows.Scan(&inicio, &fin); err != nil {
			return nil, err
		}
		duraciones = append(duraciones, fin.Sub(inicio))
	}
	return duraciones, rows.Err()
}

func (r *MySQLRepository) Cleanup(ctx context.Context, retencion time.Duration) (CleanupCounts, error) {
	ahora := time.Now()
	var counts CleanupCounts

	res, err := r.db.ExecContext(ctx,
		`DELETE FROM simulaciones
		 WHERE estado IN ('error','cancelado') AND COALESCE(terminado, creado) < ?`,
		ahora.Add(-7*24*time.Hour))
	if err != nil {
		return counts, err
	}
	counts.Errores, _ = res.RowsAffected()

	res, err = r.db.ExecContext(ctx,
		`DELETE FROM simulaciones
		 WHERE estado = 'completado' AND COALESCE(terminado, creado) < ?`,
		ahora.Add(-retencion))
	if err != nil {
		return counts, err
	}
	counts.Completados, _ = res.RowsAffected()
	return counts, nil
}

func (r *MySQLRepository) ListAbandonedPending(ctx context.Context, antesDe time.Time) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM simulaciones WHERE estado = 'pendiente' AND creado < ?`, antesDe)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getJob(ctx context.Context, q rowQuerier, id int64) (*Job, error) {
	var j Job
	err := q.QueryRowContext(ctx, queryGet, id).Scan(
		&j.ID, &j.Hash, &j.Usuario, &j.Peticion, &j.Estado, &j.Resultado,
		&j.Error, &j.Creado, &j.Terminado)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func getJobMaybe(ctx context.Context, q rowQuerier, query string, args ...any) (*Job, bool, error) {
	job, err := func() (*Job, error) {
		var j Job
		err := q.QueryRowContext(ctx, query, args...).Scan(
			&j.ID, &j.Hash, &j.Usuario, &j.Peticion, &j.Estado, &j.Resultado,
			&j.Error, &j.Creado, &j.Terminado)
		return &j, err
	}()
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return job, true, nil
}

func posicionDe(ctx context.Context, q rowQuerier, job *Job) (int, error) {
	var antes int
	err := q.QueryRowContext(ctx, queryPosicion, job.Creado, job.Creado, job.ID).Scan(&antes)
	if err != nil {
		return 0, err
	}
	return antes + 1, nil
}
