package simulaciones

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Claves de Redis. Deben coincidir con worker/src/cola.ts; el contrato
// completo está en docs/cola-redis.md.
const (
	claveCola    = "sim:cola"
	claveWorkers = "sim:workers"
	claveEstados = "sim:estados"
	canalAvisos  = "sim:eventos"
	grupoEstados = "api"
)

func claveTrabajo(id int64) string {
	return "sim:trabajo:" + strconv.FormatInt(id, 10)
}

const (
	// ttlTrabajo es una red de seguridad: ningún trabajo espera tanto.
	ttlTrabajo = 24 * time.Hour
	// ttlLapida es lo que se guarda el hash de un trabajo cancelado.
	ttlLapida   = time.Hour
	timeoutPing = time.Second
)

// scriptEncolar crea el hash del trabajo y lo pone al final de la cola (el
// worker toma por la derecha). Si el hash ya existe (en cola, en curso o como
// lápida) no hace nada y devuelve -1; si no, devuelve la posición.
var scriptEncolar = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return -1 end
redis.call('HSET', KEYS[1], 'peticion', ARGV[2], 'intentos', ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[4])
return redis.call('LPUSH', KEYS[2], ARGV[1])`)

// scriptCancelar saca el trabajo de la cola si seguía esperando y marca el
// hash como cancelado: si ya estaba en curso, el worker no publicará nada.
var scriptCancelar = redis.NewScript(`
local quitados = redis.call('LREM', KEYS[2], 0, ARGV[1])
redis.call('HSET', KEYS[1], 'cancelado', '1')
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return quitados`)

// ColaRedis reparte los trabajos al worker a través de Redis y recibe de él
// los cambios de estado. MariaDB sigue siendo la fuente de verdad: si Redis
// pierde algo, la reconciliación lo vuelve a encolar.
type ColaRedis struct {
	rdb        *redis.Client
	caducado   time.Duration
	instancia  string
	disponible atomic.Bool
	logger     *slog.Logger
}

// ClienteRedis crea el cliente a partir de REDIS_URL (ya validada en LoadConfig).
func ClienteRedis(url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return redis.NewClient(opt), nil
}

// NewColaRedis usa `caducado` para decidir si un worker sigue vivo: lo está si
// su último latido es más reciente.
func NewColaRedis(rdb *redis.Client, caducado time.Duration, logger *slog.Logger) *ColaRedis {
	if logger == nil {
		logger = slog.Default()
	}
	return &ColaRedis{rdb: rdb, caducado: caducado, instancia: nombreInstancia(), logger: logger}
}

// nombreInstancia identifica a esta API como consumidora del stream y como
// origen de los avisos.
func nombreInstancia() string {
	host, _ := os.Hostname()
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), hex.EncodeToString(b))
}

// Esperar comprueba que Redis responde, con varios intentos al arrancar.
func (c *ColaRedis) Esperar(ctx context.Context, intentos int, espera time.Duration) error {
	var err error
	for i := 0; i < intentos; i++ {
		if err = c.Ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(espera):
		}
	}
	return err
}

func (c *ColaRedis) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, timeoutPing)
	defer cancel()
	return c.rdb.Ping(ctx).Err()
}

// Encolar pone el trabajo al final de la cola y devuelve su posición. Si
// Redis ya lo tenía, no lo duplica y devuelve 0.
func (c *ColaRedis) Encolar(ctx context.Context, id int64, peticion []byte, intentos int) (int, error) {
	pos, err := scriptEncolar.Run(ctx, c.rdb, []string{claveTrabajo(id), claveCola},
		id, peticion, intentos, ttlTrabajo.Milliseconds()).Int()
	if err != nil {
		return 0, err
	}
	return max(pos, 0), nil
}

// Cancelar saca el trabajo de la cola y deja la lápida para que el worker no
// lo simule ni publique su resultado.
func (c *ColaRedis) Cancelar(ctx context.Context, id int64) error {
	return scriptCancelar.Run(ctx, c.rdb, []string{claveTrabajo(id), claveCola}, id, ttlLapida.Milliseconds()).Err()
}

// Posiciones devuelve la posición de cada trabajo en cola; 1 es el siguiente.
func (c *ColaRedis) Posiciones(ctx context.Context) (map[int64]int, error) {
	ids, err := c.rdb.LRange(ctx, claveCola, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	posiciones := make(map[int64]int, len(ids))
	for i, s := range ids {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			posiciones[id] = len(ids) - i
		}
	}
	return posiciones, nil
}

// EstadosSinAplicar cuenta las entradas de sim:estados que la API aún no ha
// aplicado: al aplicarlas se borran.
func (c *ColaRedis) EstadosSinAplicar(ctx context.Context) (int64, error) {
	return c.rdb.XLen(ctx, claveEstados).Result()
}

func (c *ColaRedis) prepararGrupo(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, claveEstados, grupoEstados, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// LeerEstados devuelve los estados nuevos que ha publicado el worker. Espera
// hasta `bloqueo` si no hay ninguno; con un valor negativo no espera.
func (c *ColaRedis) LeerEstados(ctx context.Context, bloqueo time.Duration) ([]redis.XMessage, error) {
	streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    grupoEstados,
		Consumer: c.instancia,
		Streams:  []string{claveEstados, ">"},
		Count:    20,
		Block:    bloqueo,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil && strings.HasPrefix(err.Error(), "NOGROUP") {
		// Redis perdió el grupo (reinicio sin datos): se crea y se vuelve a leer.
		if err := c.prepararGrupo(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil || len(streams) == 0 {
		return nil, err
	}
	return streams[0].Messages, nil
}

// ReclamarEstados se queda con los estados que otro consumidor (o este, tras
// un fallo de MariaDB) leyó y no confirmó en `inactivo`.
func (c *ColaRedis) ReclamarEstados(ctx context.Context, inactivo time.Duration) ([]redis.XMessage, error) {
	mensajes, _, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   claveEstados,
		Group:    grupoEstados,
		Consumer: c.instancia,
		MinIdle:  inactivo,
		Start:    "0-0",
		Count:    20,
	}).Result()
	if err != nil && strings.HasPrefix(err.Error(), "NOGROUP") {
		return nil, nil
	}
	return mensajes, err
}

// ConfirmarEstado marca la entrada como aplicada y la borra del stream.
func (c *ColaRedis) ConfirmarEstado(ctx context.Context, entrada string) error {
	_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.XAck(ctx, claveEstados, grupoEstados, entrada)
		p.XDel(ctx, claveEstados, entrada)
		return nil
	})
	return err
}

type aviso struct {
	ID     int64  `json:"id"`
	Origen string `json:"origen"`
}

// Avisar cuenta a las demás instancias de la API que el trabajo cambió.
func (c *ColaRedis) Avisar(ctx context.Context, id int64) error {
	datos, err := json.Marshal(aviso{ID: id, Origen: c.instancia})
	if err != nil {
		return err
	}
	return c.rdb.Publish(ctx, canalAvisos, datos).Err()
}

// EscucharAvisos llama a `f` con cada aviso de otra instancia hasta que ctx
// termine. go-redis se vuelve a suscribir solo si se corta la conexión.
func (c *ColaRedis) EscucharAvisos(ctx context.Context, f func(id int64)) {
	sub := c.rdb.Subscribe(ctx, canalAvisos)
	defer func() { _ = sub.Close() }()
	canal := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-canal:
			if !ok {
				return
			}
			var a aviso
			if err := json.Unmarshal([]byte(msg.Payload), &a); err != nil || a.ID <= 0 {
				c.logger.Warn("aviso_invalido", "datos", msg.Payload)
				continue
			}
			if a.Origen != c.instancia {
				f(a.ID)
			}
		}
	}
}

// Run comprueba al arrancar y cada 15 segundos si hay algún worker vivo.
func (c *ColaRedis) Run(ctx context.Context) {
	c.Comprobar(ctx)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Comprobar(ctx)
		}
	}
}

// Comprobar cuenta los workers con un latido más reciente que `caducado`. Usa
// la hora de Redis, la misma con la que los workers escriben su latido.
func (c *ColaRedis) Comprobar(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ahora, err := c.rdb.Time(ctx).Result()
	if err != nil {
		c.cambiarDisponibilidad(false)
		return false
	}
	desde := strconv.FormatInt(ahora.Add(-c.caducado).UnixMilli(), 10)
	vivos, err := c.rdb.ZCount(ctx, claveWorkers, desde, "+inf").Result()
	ok := err == nil && vivos > 0
	c.cambiarDisponibilidad(ok)
	return ok
}

// Disponible indica si en la última comprobación había algún worker vivo.
func (c *ColaRedis) Disponible() bool {
	return c.disponible.Load()
}

func (c *ColaRedis) cambiarDisponibilidad(ok bool) {
	antes := c.disponible.Swap(ok)
	if antes && !ok {
		c.logger.Warn("worker_no_disponible", "motivo", "sin latidos recientes")
	}
}
