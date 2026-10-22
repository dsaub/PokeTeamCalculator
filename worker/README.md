# Worker

Worker Bun que toma los trabajos de la cola en Redis y simula con Pokémon
Showdown. Solo habla con Redis: no conoce MariaDB ni sirve HTTP.

La documentación completa (instalación, variables, estados y tests) está en
[`../docs/worker.md`](../docs/worker.md).

```sh
bun install
bun src/index.ts
bun test                                      # REDIS_URL_TEST=redis://… para los de la cola
bunx tsc --noEmit
```
