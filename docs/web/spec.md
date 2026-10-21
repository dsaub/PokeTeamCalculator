# Product Spec: Contrataque

> **Status**: Implemented (MVP)

## Problema

Antes de un combate contra un líder de gimnasio, el jugador no sabe qué Pokémon
de su equipo tiene más probabilidades de ganar. La información disponible
(niveles, tipos, movimientos) no basta para decidir: importan la habilidad, el
objeto, los EVs/IVs, la naturaleza y la IA del rival.

## Usuarios

- Jugadores de Pokémon que preparan un combate contra un líder de gimnasio.
- No hay cuentas ni datos personales; la identidad es una cookie anónima.

## Flujo principal

1. Elegir juego (solo aparecen los que tienen gimnasios).
2. Elegir líder y el Pokémon del líder que se usará como rival.
3. Añadir hasta 6 Pokémon al equipo, con nivel, movimientos y habilidad.
4. Enviar la simulación.
5. Seguir la cola (SSE con respaldo por sondeo) y ver el ranking de winrate.
6. Leer la recomendación y, si se quiere, volver a simular.

## Escenarios de aceptación

- Si el juego no tiene líderes, la interfaz lo indica y no permite continuar.
- No se puede enviar un equipo vacío ni un Pokémon sin movimientos.
- Un `429` por simulación en curso lleva al jugador a la simulación existente.
- Un resultado cacheado se muestra al instante y se marca como tal.
- Si se pierde la conexión SSE, la interfaz sigue consultando el estado.
- Un trabajo cancelado por el jugador se distingue de uno cancelado por el
  servidor.
- La interfaz funciona sin `localStorage` (modo privado): el equipo vive en
  memoria y no se pierde la sesión actual.

## Fuera de alcance

- Cuentas, historial permanente y compartir resultados entre usuarios.
- Catálogo de objetos y naturalezas (la API aún no los expone).
- Simulaciones dobles o de más de un rival.
- Edición avanzada de EVs/IVs en la primera versión de la interfaz.

## Restricciones

- Los nombres de especie, movimiento, habilidad y objeto que se envían van en
  inglés y formato Showdown.
- La UI está en español.
- La web solo habla con `/api` en el mismo origen.
