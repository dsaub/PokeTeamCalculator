package simulaciones

import (
	"sync"
	"time"
)

const capacidadSuscriptor = 32

// Event es un evento SSE ya serializado.
type Event struct {
	Name  string
	Data  []byte
	Final bool
}

// Subscriber representa una conexión SSE. Si su canal se llena, se cierra
// para no bloquear al resto de clientes.
type Subscriber struct {
	Ch   chan Event
	Done chan struct{}

	once sync.Once
	mu   sync.Mutex

	lastEstado string
	lastPos    int
	closed     bool
}

func (s *Subscriber) cerrar() {
	s.once.Do(func() { close(s.Done) })
}

func (s *Subscriber) enviar(ev Event) {
	select {
	case s.Ch <- ev:
	default:
		s.cerrar()
	}
}

func (s *Subscriber) posicionCambiada(pos int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.lastPos == pos {
		return false
	}
	s.lastPos = pos
	return true
}

func (s *Subscriber) estadoCambiado(estado string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.lastEstado == estado {
		return false
	}
	s.lastEstado = estado
	return true
}

func (s *Subscriber) marcarFinal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.closed = true
	return true
}

// Hub vive en un solo proceso. Si algún día hay varias instancias de la API,
// cada una tendrá su propio hub y el sondeo de respaldo seguirá cubriendo los
// callbacks que no lleguen a esta instancia.
type Hub struct {
	mu       sync.Mutex
	jobs     map[int64]map[*Subscriber]struct{}
	sinGente map[int64]time.Time
}

func NewHub() *Hub {
	return &Hub{
		jobs:     make(map[int64]map[*Subscriber]struct{}),
		sinGente: make(map[int64]time.Time),
	}
}

func (h *Hub) Subscribe(id int64) *Subscriber {
	sub := &Subscriber{
		Ch:   make(chan Event, capacidadSuscriptor),
		Done: make(chan struct{}),
	}
	h.mu.Lock()
	if h.jobs[id] == nil {
		h.jobs[id] = make(map[*Subscriber]struct{})
	}
	h.jobs[id][sub] = struct{}{}
	delete(h.sinGente, id)
	h.mu.Unlock()
	return sub
}

func (h *Hub) Unsubscribe(id int64, sub *Subscriber) {
	h.mu.Lock()
	if subs := h.jobs[id]; subs != nil {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(h.jobs, id)
			h.sinGente[id] = time.Now()
		}
	}
	h.mu.Unlock()
	sub.cerrar()
}

// AbandonadoDesde indica desde cuándo no tiene suscriptores. Si tiene alguno,
// el tercer valor es true. Si nunca tuvo suscriptores, ok es false y hay que
// usar la fecha de creación del trabajo.
func (h *Hub) AbandonadoDesde(id int64) (desde time.Time, ok bool, tiene bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.jobs[id]) > 0 {
		return time.Time{}, false, true
	}
	desde, ok = h.sinGente[id]
	return desde, ok, false
}

// Mark fija el estado inicial ya enviado al cliente, para no repetirlo.
func (h *Hub) Mark(sub *Subscriber, estado string, pos int) {
	sub.mu.Lock()
	sub.lastEstado = estado
	if estado == EstadoPendiente {
		sub.lastPos = pos
	}
	sub.mu.Unlock()
}

func (h *Hub) PublishPosition(id int64, pos, estimado int) {
	ev := eventoPosicion(pos, estimado)
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.jobs[id] {
		if sub.posicionCambiada(pos) {
			sub.enviar(ev)
		}
	}
}

func (h *Hub) PublishState(id int64, estado string, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.jobs[id] {
		if sub.estadoCambiado(estado) {
			sub.enviar(ev)
		}
	}
}

func (h *Hub) PublishFinal(id int64, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.jobs[id] {
		if sub.marcarFinal() {
			sub.enviar(ev)
		}
	}
	// El handler ya tiene su evento final en el canal; liberamos el registro.
	delete(h.jobs, id)
	delete(h.sinGente, id)
}

func (h *Hub) IDs() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	ids := make([]int64, 0, len(h.jobs))
	for id := range h.jobs {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) HasSubscribers(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.jobs[id]) > 0
}

// Close cierra todas las conexiones SSE (apagado limpio).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, subs := range h.jobs {
		for sub := range subs {
			sub.cerrar()
		}
	}
	h.jobs = make(map[int64]map[*Subscriber]struct{})
	h.sinGente = make(map[int64]time.Time)
}
