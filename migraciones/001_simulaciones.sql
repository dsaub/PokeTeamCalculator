-- Cola de simulaciones. La API (Go) inserta los trabajos; el worker (Bun) los
-- toma y los actualiza.
--
-- Orden de aplicación: 001 → 002 → 003.
-- Requiere MariaDB 10.6 o superior (SELECT ... FOR UPDATE SKIP LOCKED).
CREATE TABLE IF NOT EXISTS simulaciones (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  hash       CHAR(64) NOT NULL,
  usuario    VARCHAR(64) NOT NULL,
  peticion   JSON NOT NULL,
  estado     ENUM('pendiente','procesando','completado','error','cancelado') NOT NULL DEFAULT 'pendiente',
  resultado  JSON NULL,
  error      TEXT NULL,
  intentos   INT NOT NULL DEFAULT 0,
  worker     VARCHAR(64) NULL,
  creado     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  iniciado   DATETIME(3) NULL,
  latido     DATETIME(3) NULL,
  terminado  DATETIME(3) NULL,
  INDEX idx_cola (estado, creado),
  INDEX idx_hash (hash)
) ENGINE=InnoDB;
