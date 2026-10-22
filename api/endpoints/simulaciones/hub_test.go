package simulaciones

import (
	"testing"
	"time"
)

func TestHubAbandonadoDesde(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(7)

	if _, _, tiene := h.AbandonadoDesde(7); !tiene {
		t.Fatal("el trabajo debería tener suscriptores")
	}

	h.Unsubscribe(7, sub)
	desde, ok, tiene := h.AbandonadoDesde(7)
	if tiene || !ok {
		t.Fatalf("estado tras desuscribir: desde=%v ok=%v tiene=%v", desde, ok, tiene)
	}
	if time.Since(desde) > time.Second {
		t.Fatal("la marca de abandono no es reciente")
	}

	// Al volver a suscribirse se borra la marca.
	sub2 := h.Subscribe(7)
	if _, _, tiene := h.AbandonadoDesde(7); !tiene {
		t.Fatal("debería tener suscriptores de nuevo")
	}
	h.Unsubscribe(7, sub2)

	// Nunca suscrito: ok=false para que se use `creado`.
	if _, ok, tiene := h.AbandonadoDesde(99); ok || tiene {
		t.Fatalf("trabajo nunca suscrito: ok=%v tiene=%v", ok, tiene)
	}
}

// recibir devuelve el siguiente evento del canal o falla si no hay ninguno.
func recibir(t *testing.T, sub *Subscriber) Event {
	t.Helper()
	select {
	case ev := <-sub.Ch:
		return ev
	default:
		t.Fatal("no había ningún evento")
		return Event{}
	}
}

func sinEventos(t *testing.T, sub *Subscriber) {
	t.Helper()
	select {
	case ev := <-sub.Ch:
		t.Fatalf("evento inesperado %s %s", ev.Name, ev.Data)
	default:
	}
}

func cerrado(sub *Subscriber) bool {
	select {
	case <-sub.Done:
		return true
	default:
		return false
	}
}

func TestHubNoRepiteEstadosNiPosiciones(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(1)

	h.PublishPosition(1, 3, 30)
	if ev := recibir(t, sub); ev.Name != "posicion" || string(ev.Data) != `{"posicion":3,"estimadoSegundos":30}` {
		t.Fatalf("evento %s %s", ev.Name, ev.Data)
	}
	h.PublishPosition(1, 3, 30)
	sinEventos(t, sub)

	h.PublishState(1, EstadoProcesando, eventoProcesando())
	if ev := recibir(t, sub); ev.Name != "procesando" {
		t.Fatalf("evento %s", ev.Name)
	}
	h.PublishState(1, EstadoProcesando, eventoProcesando())
	sinEventos(t, sub)

	// Un reintento vuelve a la cola con la misma posición: sí se avisa.
	h.PublishPosition(1, 3, 30)
	if ev := recibir(t, sub); ev.Name != "posicion" {
		t.Fatalf("evento %s", ev.Name)
	}
}

func TestHubMarkFijaElEstadoInicial(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(1)
	h.Mark(sub, EstadoPendiente, 2)
	h.PublishPosition(1, 2, 10)
	sinEventos(t, sub)
	h.PublishPosition(1, 1, 5)
	recibir(t, sub)

	otro := h.Subscribe(2)
	h.Mark(otro, EstadoProcesando, 9)
	if otro.lastPos != 0 {
		t.Fatalf("la posición solo se guarda en pendiente: %d", otro.lastPos)
	}
	h.PublishState(2, EstadoProcesando, eventoProcesando())
	sinEventos(t, otro)
}

func TestHubFinalSoloUnaVezYLiberaElTrabajo(t *testing.T) {
	h := NewHub()
	a, b := h.Subscribe(1), h.Subscribe(1)
	h.PublishFinal(1, eventoCancelado())
	for _, sub := range []*Subscriber{a, b} {
		if ev := recibir(t, sub); ev.Name != "cancelado" || !ev.Final {
			t.Fatalf("evento %+v", ev)
		}
	}
	if h.HasSubscribers(1) || len(h.IDs()) != 0 {
		t.Fatal("el trabajo debería haberse liberado")
	}
	if _, ok, _ := h.AbandonadoDesde(1); ok {
		t.Fatal("tampoco debe quedar marca de abandono")
	}

	// Tras el final, nada más llega aunque siga registrado.
	c := h.Subscribe(2)
	if !c.marcarFinal() || c.marcarFinal() {
		t.Fatal("marcarFinal solo devuelve true la primera vez")
	}
	h.PublishState(2, EstadoProcesando, eventoProcesando())
	h.PublishPosition(2, 1, 1)
	sinEventos(t, c)
}

func TestHubCierraAlSuscriptorLento(t *testing.T) {
	h := NewHub()
	sub := h.Subscribe(1)
	for i := 1; i <= capacidadSuscriptor+1; i++ {
		h.PublishPosition(1, i, i)
	}
	if !cerrado(sub) {
		t.Fatal("un suscriptor con el canal lleno debería cerrarse")
	}
	if len(sub.Ch) != capacidadSuscriptor {
		t.Fatalf("eventos en el canal: %d", len(sub.Ch))
	}
	// Cerrar dos veces no entra en pánico.
	h.Unsubscribe(1, sub)
}

func TestHubClose(t *testing.T) {
	h := NewHub()
	a, b := h.Subscribe(1), h.Subscribe(2)
	h.Unsubscribe(2, b)
	c := h.Subscribe(3)
	h.Close()
	for _, sub := range []*Subscriber{a, b, c} {
		if !cerrado(sub) {
			t.Fatal("Close debe cerrar todas las conexiones")
		}
	}
	if len(h.IDs()) != 0 {
		t.Fatalf("quedan ids: %v", h.IDs())
	}
	if _, ok, _ := h.AbandonadoDesde(2); ok {
		t.Fatal("Close también olvida las marcas de abandono")
	}
	// El hub sigue usable.
	d := h.Subscribe(4)
	if !h.HasSubscribers(4) {
		t.Fatal("no se pudo suscribir tras Close")
	}
	h.Unsubscribe(4, d)
}
