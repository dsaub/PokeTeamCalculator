package simulaciones

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestClienteRedis(t *testing.T) {
	rdb, err := ClienteRedis("redis://:clave@127.0.0.1:6380/3")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rdb.Close() }()
	opt := rdb.Options()
	if opt.Addr != "127.0.0.1:6380" || opt.Password != "clave" || opt.DB != 3 {
		t.Fatalf("opciones %+v", opt)
	}
	if _, err := ClienteRedis("http://no-es-redis"); err == nil {
		t.Fatal("se esperaba un error con una URL que no es de Redis")
	}
}

func TestNewColaRedisSinLogger(t *testing.T) {
	c := NewColaRedis(nil, time.Second, nil)
	if c.logger == nil || c.instancia == "" {
		t.Fatalf("cola %+v", c)
	}
	if otra := NewColaRedis(nil, time.Second, nil); otra.instancia == c.instancia {
		t.Fatal("cada instancia debe tener un nombre distinto")
	}
}

func TestEsperar(t *testing.T) {
	t.Run("responde a la primera", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		if err := a.cola.Esperar(context.Background(), 3, time.Millisecond); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("agota los intentos", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		a.mr.Close()
		if err := a.cola.Esperar(context.Background(), 2, time.Millisecond); err == nil {
			t.Fatal("se esperaba un error")
		}
	})

	t.Run("se corta al cancelar el contexto", func(t *testing.T) {
		a := nuevaAPI(t, nil)
		a.mr.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		inicio := time.Now()
		err := a.cola.Esperar(ctx, 100, time.Hour)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(inicio) > time.Second {
			t.Fatalf("error %v tras %v", err, time.Since(inicio))
		}
	})
}

func TestEncolarNoDuplicaYPosiciones(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	if pos, err := a.cola.Encolar(ctx, 1, []byte(`{}`), 0); err != nil || pos != 1 {
		t.Fatalf("primero: %d %v", pos, err)
	}
	if pos, err := a.cola.Encolar(ctx, 2, []byte(`{}`), 0); err != nil || pos != 2 {
		t.Fatalf("segundo: %d %v", pos, err)
	}
	if pos, err := a.cola.Encolar(ctx, 1, []byte(`{}`), 0); err != nil || pos != 0 {
		t.Fatalf("repetido: %d %v", pos, err)
	}
	// Una entrada que no es un número se ignora al calcular posiciones.
	if err := a.rdb.LPush(ctx, claveCola, "basura").Err(); err != nil {
		t.Fatal(err)
	}
	pos, err := a.cola.Posiciones(ctx)
	if err != nil || len(pos) != 2 || pos[1] != 1 || pos[2] != 2 {
		t.Fatalf("posiciones %v %v", pos, err)
	}

	a.mr.Close()
	if _, err := a.cola.Encolar(ctx, 3, []byte(`{}`), 0); err == nil {
		t.Fatal("sin Redis, Encolar debe fallar")
	}
	if _, err := a.cola.Posiciones(ctx); err == nil {
		t.Fatal("sin Redis, Posiciones debe fallar")
	}
}

func TestLeerEstadosSinGrupoLoRecrea(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	id := crearPendiente(t, a)
	a.aplicarEstados(t)

	// Redis perdió el stream y el grupo (reinicio sin datos).
	a.mr.Del(claveEstados)
	if msgs, err := a.cola.ReclamarEstados(ctx, 0); err != nil || msgs != nil {
		t.Fatalf("reclamar sin grupo: %v %v", msgs, err)
	}
	if msgs, err := a.cola.LeerEstados(ctx, -1); err != nil || msgs != nil {
		t.Fatalf("leer sin grupo: %v %v", msgs, err)
	}
	// El grupo vuelve a existir y lo publicado después se lee.
	a.tomar(t, "w1")
	if err := a.estados.Procesar(ctx, -1); err != nil {
		t.Fatal(err)
	}
	if a.repo.estado(id) != EstadoProcesando {
		t.Fatalf("estado %s", a.repo.estado(id))
	}
}

func TestLeerEstadosConEsperaSinMensajes(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	if err := a.cola.prepararGrupo(ctx); err != nil {
		t.Fatal(err)
	}
	// Crear el grupo dos veces no es un error.
	if err := a.cola.prepararGrupo(ctx); err != nil {
		t.Fatal(err)
	}
	msgs, err := a.cola.LeerEstados(ctx, 20*time.Millisecond)
	if err != nil || len(msgs) != 0 {
		t.Fatalf("%v %v", msgs, err)
	}
}

func TestColaSinRedis(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	if err := a.cola.prepararGrupo(ctx); err != nil {
		t.Fatal(err)
	}
	a.mr.Close()
	if err := a.cola.prepararGrupo(ctx); err == nil {
		t.Fatal("prepararGrupo sin Redis")
	}
	if _, err := a.cola.LeerEstados(ctx, -1); err == nil {
		t.Fatal("LeerEstados sin Redis")
	}
	if _, err := a.cola.ReclamarEstados(ctx, 0); err == nil {
		t.Fatal("ReclamarEstados sin Redis")
	}
	if err := a.cola.ConfirmarEstado(ctx, "1-0"); err == nil {
		t.Fatal("ConfirmarEstado sin Redis")
	}
	if err := a.cola.Avisar(ctx, 1); err == nil {
		t.Fatal("Avisar sin Redis")
	}
	if _, err := a.cola.EstadosSinAplicar(ctx); err == nil {
		t.Fatal("EstadosSinAplicar sin Redis")
	}
	if err := a.cola.Cancelar(ctx, 1); err == nil {
		t.Fatal("Cancelar sin Redis")
	}
}

func TestComprobarYDisponible(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	if !a.cola.Disponible() {
		t.Fatal("nuevaAPI arranca con un worker vivo")
	}
	// Un latido caducado no cuenta.
	a.mr.Del(claveWorkers)
	if _, err := a.mr.ZAdd(claveWorkers, float64(time.Now().Add(-time.Hour).UnixMilli()), "viejo"); err != nil {
		t.Fatal(err)
	}
	if a.cola.Comprobar(ctx) || a.cola.Disponible() {
		t.Fatal("un latido de hace una hora no es un worker vivo")
	}
	a.latir(t, "w2")
	if !a.cola.Disponible() {
		t.Fatal("con un latido reciente debería estar disponible")
	}
	// Si Redis no responde, no hay worker disponible.
	a.mr.Close()
	if a.cola.Comprobar(ctx) || a.cola.Disponible() {
		t.Fatal("sin Redis no debería haber worker disponible")
	}
}

func TestColaRunTerminaConElContexto(t *testing.T) {
	a := nuevaAPI(t, nil)
	terminaPronto(t, "ColaRedis.Run", func() { a.cola.Run(ctxCancelado()) })
}

func TestAvisosEntreInstancias(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var recibidos []int64
	hecho := make(chan struct{})
	go func() {
		a.cola.EscucharAvisos(ctx, func(id int64) {
			mu.Lock()
			recibidos = append(recibidos, id)
			mu.Unlock()
		})
		close(hecho)
	}()
	for i := 0; a.mr.PubSubNumSub(canalAvisos)[canalAvisos] == 0; i++ {
		if i > 200 {
			t.Fatal("no se suscribió")
		}
		time.Sleep(5 * time.Millisecond)
	}

	otra := NewColaRedis(a.rdb, time.Second, nil)
	publicar := func(datos string) {
		t.Helper()
		if err := a.rdb.Publish(context.Background(), canalAvisos, datos).Err(); err != nil {
			t.Fatal(err)
		}
	}
	// Los propios, los que no son JSON y los de id no válido se ignoran.
	if err := a.cola.Avisar(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	publicar("no es json")
	publicar(`{"id":0,"origen":"otra"}`)
	publicar(`{"id":-4,"origen":"otra"}`)
	if err := otra.Avisar(context.Background(), 7); err != nil {
		t.Fatal(err)
	}

	limite := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(recibidos)
		mu.Unlock()
		if n > 0 || time.Now().After(limite) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-hecho
	mu.Lock()
	defer mu.Unlock()
	if len(recibidos) != 1 || recibidos[0] != 7 {
		t.Fatalf("recibidos %v", recibidos)
	}
}

func TestAvisarPublicaIDYOrigen(t *testing.T) {
	a := nuevaAPI(t, nil)
	ctx := context.Background()
	sub := a.rdb.Subscribe(ctx, canalAvisos)
	defer func() { _ = sub.Close() }()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.cola.Avisar(ctx, 42); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var av aviso
	if err := json.Unmarshal([]byte(msg.Payload), &av); err != nil || av.ID != 42 || av.Origen != a.cola.instancia {
		t.Fatalf("aviso %q → %+v %v", msg.Payload, av, err)
	}
}

func TestEscucharAvisosTerminaSiSeCierraElCliente(t *testing.T) {
	a := nuevaAPI(t, nil)
	rdb := redis.NewClient(&redis.Options{Addr: a.mr.Addr()})
	cola := NewColaRedis(rdb, time.Second, nil)
	hecho := make(chan struct{})
	go func() {
		cola.EscucharAvisos(context.Background(), func(int64) {})
		close(hecho)
	}()
	for i := 0; a.mr.PubSubNumSub(canalAvisos)[canalAvisos] == 0; i++ {
		if i > 200 {
			t.Fatal("no se suscribió")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = rdb.Close()
	select {
	case <-hecho:
	case <-time.After(3 * time.Second):
		t.Fatal("EscucharAvisos no terminó al cerrarse el cliente")
	}
}

