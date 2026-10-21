# Web

Interfaz React + Vite + TypeScript de Contrataque.

- Documentación de producto: [`../docs/web/spec.md`](../docs/web/spec.md)
- Arquitectura y contrato con la API: [`../docs/arquitectura.md`](../docs/arquitectura.md)
- Historial de la API antigua: [`../docs/web/api-pendiente.md`](../docs/web/api-pendiente.md)

```sh
bun install
bun run dev        # proxy /api -> http://127.0.0.1:8080
bun run dev:mock   # API simulada en memoria, sin base de datos
bun run check      # typecheck + lint + tests
bun run build
```
