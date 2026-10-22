package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

var (
	statusMu sync.RWMutex
	status   string
)

type config struct {
	host     string
	port     string
	user     string
	password string
	name     string
	tls      string
}

func loadConfig() config {
	return config{
		host:     getEnv("DB_HOST", "127.0.0.1"),
		port:     getEnv("DB_PORT", "3306"),
		user:     getEnv("DB_USER", "root"),
		password: os.Getenv("DB_PASSWORD"),
		name:     getEnv("DB_NAME", "pokedata"),
		tls:      getEnv("DB_TLS", "preferred"),
	}
}

func Initialize() {
	SetStatus("starting")
	cfg := loadConfig()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&tls=%s",
		cfg.user, cfg.password, cfg.host, cfg.port, cfg.name, cfg.tls)
	database, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	DB = database

	// MySQL puede tardar en aceptar conexiones al arrancar.
	if err := waitForDatabase(DB, 30, 2*time.Second); err != nil {
		log.Fatalf("No se pudo conectar a MySQL (%s:%s): %v", cfg.host, cfg.port, err)
	}

	populated, err := isPopulated(DB, cfg.name)
	if err != nil {
		log.Fatalf("Error al comprobar la base de datos: %v", err)
	}
	if populated {
		SetStatus("ok")
	} else {
		SetStatus("error")
		log.Println("Error, database is NOT initialized")
	}

}

func waitForDatabase(db *sql.DB, attempts int, delay time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = db.Ping(); err == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return err
}

// isPopulated comprueba si pokedata terminó una importación anterior: las vistas
// se crean al final, así que si existe v_gimnasios los datos están completos.
func isPopulated(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.views WHERE table_schema = ? AND table_name = 'v_gimnasios'",
		name).Scan(&n)
	return n > 0, err
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func GetStatus() string {
	statusMu.RLock()
	defer statusMu.RUnlock()
	return status
}

// SetStatus fija el estado que devuelve GetStatus (lo usa /api/salud).
func SetStatus(statusn string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	status = statusn
}

func GetDatabase() *sql.DB {
	return DB
}
