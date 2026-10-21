package simulaciones

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	nombreCookieSID = "sid"
	contextoUsuario = "simulaciones.usuario"
	duracionCookie  = 365 * 24 * 60 * 60 // un año, en segundos
)

// Sessions gestiona la cookie anónima firmada. No hay usuarios reales en el
// backend, así que `usuario` es el identificador aleatorio de la cookie.
type Sessions struct {
	secreto []byte
}

func NewSessions(secreto []byte) *Sessions {
	return &Sessions{secreto: secreto}
}

// Ensure valida la cookie o emite una nueva y deja el identificador en el
// contexto de Gin.
func (s *Sessions) Ensure(c *gin.Context) {
	sid, ok := s.leer(c.Request)
	if !ok {
		sid = nuevoSID()
		s.escribir(c, sid)
	}
	c.Set(contextoUsuario, sid)
	c.Next()
}

func UsuarioActual(c *gin.Context) string {
	return c.GetString(contextoUsuario)
}

func (s *Sessions) leer(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(nombreCookieSID)
	if err != nil {
		return "", false
	}
	partes := strings.Split(cookie.Value, ".")
	if len(partes) != 2 {
		return "", false
	}
	sid := partes[0]
	if len(sid) != 32 {
		return "", false
	}
	if _, err := hex.DecodeString(sid); err != nil {
		return "", false
	}
	esperada := s.firma(sid)
	if !hmac.Equal([]byte(esperada), []byte(partes[1])) {
		return "", false
	}
	return sid, true
}

func (s *Sessions) escribir(c *gin.Context, sid string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     nombreCookieSID,
		Value:    sid + "." + s.firma(sid),
		Path:     "/",
		MaxAge:   duracionCookie,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   esHTTPS(c.Request),
	})
}

func (s *Sessions) firma(sid string) string {
	mac := hmac.New(sha256.New, s.secreto)
	mac.Write([]byte(sid))
	return hex.EncodeToString(mac.Sum(nil))
}

func nuevoSID() string {
	b := make([]byte, 16) // 128 bits
	if _, err := rand.Read(b); err != nil {
		panic("no se pudo generar el identificador de sesión: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func esHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
