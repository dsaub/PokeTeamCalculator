package catalogo

import "strings"

var reemplazosNombre = strings.NewReplacer(
	"á", "a", "à", "a", "ä", "a", "â", "a",
	"é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i",
	"ó", "o", "ò", "o", "ö", "o", "ô", "o",
	"ú", "u", "ù", "u", "ü", "u", "û", "u",
	"ñ", "n", "ç", "c",
	"♀", "f", "♂", "m", "×", "x",
	"’", "", "'", "", ".", "", ",", "", "!", "", "?", "", ":", "",
)

// Slug convierte un nombre de juego en un identificador estable.
func Slug(s string) string {
	s = strings.ToLower(reemplazosNombre.Replace(s))
	var b strings.Builder
	guion := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			guion = false
			continue
		}
		if !guion && b.Len() > 0 {
			b.WriteByte('-')
			guion = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func NombreJuego(nombres []string) string {
	if len(nombres) == 0 {
		return "Pokémon"
	}
	return "Pokémon " + strings.Join(nombres, " y ")
}

// NombreShowdown reconstruye el nombre de especie de Showdown a partir del
// identificador de pokedata. Ejemplos:
//
//	yamask-galar              -> Yamask-Galar
//	darmanitan-galar-standard -> Darmanitan-Galar
//	meowstic-female           -> Meowstic-Female
func NombreShowdown(identifier, speciesIdentifier string, isDefault bool, nombreBase string) string {
	if nombreBase == "" {
		nombreBase = tituloTokens(identifier)
	}
	if isDefault || identifier == speciesIdentifier {
		return nombreBase
	}
	sufijo := strings.TrimPrefix(identifier, speciesIdentifier+"-")
	if sufijo == identifier {
		// El identificador no empieza por el de la especie; no duplicamos.
		sufijo = identifier
	}
	sufijo = strings.TrimSuffix(sufijo, "-standard")
	if sufijo == "" {
		return nombreBase
	}
	return nombreBase + "-" + tituloTokens(sufijo)
}

func tituloTokens(s string) string {
	partes := strings.Split(s, "-")
	for i, parte := range partes {
		if parte == "" {
			continue
		}
		partes[i] = strings.ToUpper(parte[:1]) + parte[1:]
	}
	return strings.Join(partes, "-")
}
