-- Índices propios de la API. La tabla la crea 001_simulaciones.sql.
-- No se modifican columnas ni tipos.
--
-- Índices:
--   idx_usuario_estado → límite de trabajos por usuario y duplicados.
--   idx_hash_estado    → caché por hash y búsqueda de duplicados.
CREATE INDEX IF NOT EXISTS idx_usuario_estado ON simulaciones (usuario, estado);
CREATE INDEX IF NOT EXISTS idx_hash_estado ON simulaciones (hash, estado, terminado);
