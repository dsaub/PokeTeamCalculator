package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

// Rutas dentro de la imagen de la API (ver Dockerfile).
const (
	binarioPokedata  = "/pokedata-gen"
	migracionFormas  = "/migraciones/003_v_gym_teams_formas.sql"
	cerrojoPokedata  = "poketeam_importar_pokedata"
	esperaCerrojoSeg = 3600
)

// importador carga pokedata en una base vacía. Los campos se pueden cambiar en
// los tests.
type importador struct {
	binario   string
	migracion string
	ejecutar  func(ctx context.Context, binario string, args ...string) error
}

func nuevoImportador() *importador {
	return &importador{
		binario:   getEnv("POKEDATA_GEN", binarioPokedata),
		migracion: getEnv("POKEDATA_MIGRACION", migracionFormas),
		ejecutar:  ejecutarComando,
	}
}

// disponible indica si el binario está en la imagen. En desarrollo (go run) no
// suele estar y la importación se omite.
func (im *importador) disponible() bool {
	_, err := os.Stat(im.binario)
	return err == nil
}

// importar ejecuta pokedata-gen y aplica la migración 003, que pokedata pisa
// al recrear sus vistas. El cerrojo evita que dos instancias de la API borren
// y carguen las tablas a la vez; la que llega tarde ve los datos y no repite.
// db debe admitir varias sentencias por consulta (ver dsnImportacion).
func (im *importador) importar(ctx context.Context, db *sql.DB, cfg config) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	var obtenido sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", cerrojoPokedata, esperaCerrojoSeg).Scan(&obtenido); err != nil {
		return fmt.Errorf("cerrojo de importación: %w", err)
	}
	if obtenido.Int64 != 1 {
		return errors.New("cerrojo de importación: tiempo de espera agotado")
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", cerrojoPokedata) }()

	lleno, err := isPopulated(db, cfg.name)
	if err != nil {
		return err
	}
	if lleno {
		return nil
	}

	fuentes, err := os.MkdirTemp("", "pokedata-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(fuentes) }()

	if err := im.ejecutar(ctx, im.binario, argumentosPokedata(cfg, fuentes)...); err != nil {
		return fmt.Errorf("pokedata-gen: %w", err)
	}

	migracion, err := os.ReadFile(im.migracion)
	if err != nil {
		return fmt.Errorf("leer %s: %w", im.migracion, err)
	}
	if _, err := conn.ExecContext(ctx, string(migracion)); err != nil {
		return fmt.Errorf("aplicar %s: %w", im.migracion, err)
	}
	return nil
}

func argumentosPokedata(cfg config, fuentes string) []string {
	return []string{
		"--mode", "mysql",
		"--host", cfg.host,
		"--port", cfg.port,
		"--username", cfg.user,
		"--password", cfg.password,
		"--database", cfg.name,
		"--tls", cfg.tls,
		"--sources", fuentes,
	}
}

// dsnImportacion permite varias sentencias: la 003 lleva comentarios tras el
// último «;».
func dsnImportacion(cfg config) string {
	return dsn(cfg) + "&multiStatements=true"
}

func ejecutarComando(ctx context.Context, binario string, args ...string) error {
	cmd := exec.CommandContext(ctx, binario, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// importarEnSegundoPlano deja la API sirviendo (con /api/salud en
// «importando») mientras pokedata descarga las fuentes, que tarda minutos.
func importarEnSegundoPlano(im *importador, cfg config) {
	SetStatus("importando")
	log.Println("La base no tiene los datos de pokedata: importándolos")
	go func() {
		inicio := time.Now()
		db, err := sql.Open("mysql", dsnImportacion(cfg))
		if err == nil {
			err = im.importar(context.Background(), db, cfg)
			_ = db.Close()
		}
		if err != nil {
			SetStatus("error")
			log.Printf("Error al importar pokedata: %v", err)
			return
		}
		SetStatus("ok")
		log.Printf("pokedata importado en %s", time.Since(inicio).Round(time.Second))
	}()
}
