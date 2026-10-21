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
