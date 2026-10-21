import { Battle, Pokemon } from "pokemon-showdown";
import type { ActiveMove, Move } from "pokemon-showdown/dist/sim/dex-moves";

// IAs que eligen movimiento leyendo el estado real del combate. El azar usa
// Math.random() y no battle.prng para no alterar la secuencia aleatoria del
// propio combate.

type Lado = "p1" | "p2";

/** Habilidades que anulan por completo los movimientos de un tipo. */
const INMUNIDAD_HABILIDAD: Record<string, string> = {
  levitate: "Ground",
  eartheater: "Ground",
  flashfire: "Fire",
  wellbakedbody: "Fire",
  waterabsorb: "Water",
  stormdrain: "Water",
  dryskin: "Water",
  voltabsorb: "Electric",
  lightningrod: "Electric",
  motordrive: "Electric",
  sapsipper: "Grass",
};

/** Lightning Rod y Storm Drain solo dan inmunidad desde gen 5. */
const INMUNIDAD_DESDE_GEN5 = new Set(["lightningrod", "stormdrain"]);

const ROMPEMOLDES = ["moldbreaker", "teravolt", "turboblaze"];

/** Media de golpes de los movimientos de 2 a 5 golpes. */
const GOLPES_MEDIOS_2_A_5 = 3.1;

/** Potencia supuesta para movimientos de potencia variable (Return, Low Kick…). */
const POTENCIA_VARIABLE = 60;

const FACTOR_ALEATORIO_MEDIO = 0.925;

interface Golpe {
  /** Daño si acierta. */
  dano: number;
  /** Probabilidad de acertar (0–1). */
  precision: number;
}

function precisionDe(move: Move): number {
  return move.accuracy === true ? 1 : move.accuracy / 100;
}

function esInmunePorHabilidad(battle: Battle, atacante: Pokemon, defensor: Pokemon, move: Move): boolean {
  if (atacante.hasAbility(ROMPEMOLDES)) return false;
  const habilidad = defensor.ability;
  if (habilidad === "wonderguard") {
    return battle.dex.getEffectiveness(move.type, defensor.getTypes()) <= 0;
  }
  const tipo = INMUNIDAD_HABILIDAD[habilidad];
  if (!tipo || tipo !== move.type) return false;
  return !(INMUNIDAD_DESDE_GEN5.has(habilidad) && battle.gen < 5);
}

/**
 * Asigna temporalmente una propiedad que el objeto ya tiene y devuelve cómo
 * dejarla como estaba. Solo para propiedades propias: añadir y borrar
 * propiedades cambia la forma del objeto y el JIT deja de optimizar todo el
 * simulador (los combates iban el doble de lentos).
 */
function sustituir<T extends object, K extends keyof T>(objeto: T, clave: K, nuevo: T[K]): () => void {
  const anterior = objeto[clave];
  objeto[clave] = nuevo;
  return () => { objeto[clave] = anterior; };
}

/**
 * Mientras es true, estamos estimando y nada debe cambiar el combate. Los
 * únicos manejadores de estos eventos con efectos son las bayas que reducen
 * el daño súper eficaz (Occa Berry…): llaman a eatItem() y escriben en el
 * registro. Con la marca puesta, eatItem() solo dice si se podría comer la
 * baya y add() no escribe nada.
 *
 * Se envuelven una sola vez en el prototipo (no en cada instancia) para no
 * cambiar la forma de los objetos; fuera de la estimación solo cuesta mirar
 * la marca.
 */
let estimando = false;

const eatItemOriginal = Pokemon.prototype.eatItem;
Pokemon.prototype.eatItem = function (this: Pokemon, ...args: Parameters<typeof eatItemOriginal>) {
  if (estimando) return !!this.item && !this.ignoringItem();
  return eatItemOriginal.apply(this, args);
};

const addOriginal = Battle.prototype.add;
Battle.prototype.add = function (this: Battle, ...args: Parameters<typeof addOriginal>) {
  if (estimando) return;
  return addOriginal.apply(this, args);
};

/**
 * Lanza un evento de Showdown (BasePower, ModifyAtk, ModifyDamage…) igual que
 * su propio cálculo de daño: con el movimiento activo como efecto, porque
 * muchos manejadores lo leen (Blaze en gen 9 mira move.type). Así cuentan
 * Choice Band, Huge Power, Guts, Technician, Life Orb, Expert Belt, Filter…
 * Se llama con `estimando` activado (ver arriba).
 *
 * Si algún manejador lanza una excepción (no está pensado para llamarse
 * fuera de un turno), se usa el valor sin modificar. runEvent no restaura su
 * pila de eventos si un manejador lanza, así que la restauramos nosotros para
 * no dejar el combate en mal estado.
 */
function conModificadores(
  battle: Battle, evento: string, objetivo: Pokemon, fuente: Pokemon, move: ActiveMove, valor: number, onEffect = false,
): number {
  const { event, eventDepth, effect, effectState } = battle;
  try {
    const r = battle.runEvent(evento, objetivo, fuente, move, valor, onEffect);
    return typeof r === "number" && Number.isFinite(r) ? r : valor;
  } catch {
    Object.assign(battle, { event, eventDepth, effect, effectState });
    return valor;
  }
}

/**
 * Valor de referencia para sacar el multiplicador del evento ModifyDamage.
 * Ningún manejador de ese evento depende del daño (solo multiplican), y con
 * un valor grande el redondeo de Showdown no se come el multiplicador.
 */
const DANO_REFERENCIA = 1_000_000;

/** Daño estimado de un golpe y su precisión, sin tener en cuenta los PS del defensor. */
function estimarGolpe(battle: Battle, atacante: Pokemon, defensor: Pokemon, move: Move): Golpe {
  const nulo: Golpe = { dano: 0, precision: 0 };
  const categoria = battle.getCategory(move);
  if (categoria === "Status") return nulo;

  const precision = precisionDe(move);
  // Struggle no tiene tipo efectivo: golpea a todo.
  if (move.id !== "struggle") {
    if (!battle.dex.getImmunity(move.type, defensor.getTypes())) return nulo;
    if (esInmunePorHabilidad(battle, atacante, defensor, move)) return nulo;
  }

  // Casos especiales que no usan la fórmula general.
  if (move.ohko) {
    if (atacante.level < defensor.level) return nulo;
    return { dano: defensor.hp, precision };
  }
  if (move.id === "superfang") return { dano: Math.max(1, Math.floor(defensor.hp / 2)), precision };
  if (move.damage === "level") return { dano: atacante.level, precision };
  if (typeof move.damage === "number") return { dano: move.damage, precision };

  let potencia = move.basePower;
  if (!potencia && move.basePowerCallback) potencia = POTENCIA_VARIABLE;
  if (!potencia) return nulo;

  // Copia del movimiento, como hace Showdown: los manejadores pueden modificarla.
  const activo = battle.dex.getActiveMove(move);
  if (atacante.hasAbility(ROMPEMOLDES)) activo.ignoreAbility = true;

  // Durante getDamage, Showdown tiene el movimiento y su usuario como
  // activos; así sabe, por ejemplo, que Mold Breaker anula Multiscale.
  // Reproducimos ese contexto solo mientras duran los eventos.
  const deshacer = [
    sustituir(battle, "activeMove", activo),
    sustituir(battle, "activePokemon", atacante),
    sustituir(battle, "activeTarget", defensor),
  ];
  estimando = true;
  try {
    return calcularGolpe(battle, atacante, defensor, move, activo, categoria, potencia, precision);
  } finally {
    estimando = false;
    for (const d of deshacer) d();
  }
}

/** Fórmula general de daño con los eventos de Showdown, ya con el contexto preparado. */
function calcularGolpe(
  battle: Battle, atacante: Pokemon, defensor: Pokemon, move: Move, activo: ActiveMove,
  categoria: "Physical" | "Special", potenciaBase: number, precision: number,
): Golpe {
  const potencia = conModificadores(battle, "BasePower", atacante, defensor, activo, potenciaBase, true);
  if (!potencia) return { dano: 0, precision: 0 };

  // Stats con sus subidas y bajadas (getStat sin eventos) y después los
  // eventos Modify* con el movimiento, en el mismo orden que getDamage.
  const fisico = categoria === "Physical";
  const [statAtaque, statDefensa] = fisico ? ["atk", "def"] as const : ["spa", "spd"] as const;
  const nombreEvento = { atk: "Atk", def: "Def", spa: "SpA", spd: "SpD" };
  const ataque = conModificadores(battle, `Modify${nombreEvento[statAtaque]}`, atacante, defensor, activo,
    atacante.getStat(statAtaque, false, true));
  const defensa = Math.max(1, conModificadores(battle, `Modify${nombreEvento[statDefensa]}`, defensor, atacante, activo,
    defensor.getStat(statDefensa, false, true)));
  const base = Math.floor(Math.floor(Math.floor((2 * atacante.level) / 5 + 2) * potencia * ataque / defensa) / 50) + 2;

  let modificador = FACTOR_ALEATORIO_MEDIO;
  let efectividad = 0;
  if (move.id !== "struggle") {
    if (atacante.hasType(move.type)) modificador *= atacante.hasAbility("adaptability") ? 2 : 1.5;
    efectividad = Math.max(-6, Math.min(6, battle.dex.getEffectiveness(move.type, defensor.getTypes())));
    modificador *= 2 ** efectividad;
  }
  if (fisico && atacante.status === "brn" && !atacante.hasAbility("guts")) modificador *= 0.5;

  // Último paso de getDamage: el evento ModifyDamage (Life Orb, Expert Belt,
  // Multiscale, Filter, bayas…). Varios manejadores leen la eficacia de los
  // datos del golpe, que Showdown rellena justo antes.
  defensor.getMoveHitData(activo).typeMod = efectividad;
  modificador *= conModificadores(battle, "ModifyDamage", atacante, defensor, activo, DANO_REFERENCIA) / DANO_REFERENCIA;

  let golpes = 1;
  if (Array.isArray(move.multihit)) golpes = move.multihit[0] === 2 && move.multihit[1] === 5 ? GOLPES_MEDIOS_2_A_5 : (move.multihit[0]! + move.multihit[1]!) / 2;
  else if (typeof move.multihit === "number") golpes = move.multihit;

  return { dano: base * modificador * golpes, precision };
}

/** Daño esperado (ya multiplicado por la precisión) de `move` contra `defensor`. */
export function danoEsperado(battle: Battle, atacante: Pokemon, defensor: Pokemon, move: Move): number {
  const { dano, precision } = estimarGolpe(battle, atacante, defensor, move);
  return dano * precision;
}

interface Opcion {
  /** Elección para battle.choose: "move 1" … "move 4". */
  eleccion: string;
  move: Move;
}

/** Movimientos usables según la petición actual del lado. */
function movimientosUsables(battle: Battle, lado: Lado): Opcion[] {
  const peticion = battle[lado].activeRequest;
  if (!peticion || !("active" in peticion) || !peticion.active) return [];
  const movs = peticion.active[0]?.moves ?? [];
  const opciones: Opcion[] = [];
  movs.forEach((m, i) => {
    if (m.disabled || m.pp === 0) return;
    opciones.push({ eleccion: `move ${i + 1}`, move: battle.dex.moves.get(m.id) });
  });
  return opciones;
}

function alAzar<T>(lista: T[]): T {
  return lista[Math.floor(Math.random() * lista.length)]!;
}

/**
 * El movimiento con más daño esperado (acotado a los PS del rival). Si alguno
 * lo debilita, prefiere el de más prioridad y, a igualdad, el más preciso.
 */
export function elegirMejorMovimiento(battle: Battle, lado: Lado): string {
  const opciones = movimientosUsables(battle, lado);
  const atacante = battle[lado].active[0];
  const defensor = battle[lado].foe.active[0];
  if (!opciones.length || !atacante || !defensor) return "default";

  const evaluadas = opciones.map((o) => {
    const golpe = estimarGolpe(battle, atacante, defensor, o.move);
    return {
      ...o,
      golpe,
      valor: Math.min(golpe.dano, defensor.hp) * golpe.precision,
      debilita: golpe.precision > 0 && golpe.dano >= defensor.hp,
    };
  });

  const queDebilitan = evaluadas.filter((e) => e.debilita);
  if (queDebilitan.length) {
    queDebilitan.sort((a, b) =>
      b.move.priority - a.move.priority || b.golpe.precision - a.golpe.precision || b.valor - a.valor);
    return queDebilitan[0]!.eleccion;
  }

  // Entre empatados (por ejemplo, solo movimientos de estado), uno al azar.
  const maximo = Math.max(...evaluadas.map((e) => e.valor));
  return alAzar(evaluadas.filter((e) => e.valor === maximo)).eleccion;
}

/**
 * IA imperfecta como la de los juegos: con probabilidad `aleatoriedad` usa un
 * movimiento al azar; si no, el mejor.
 */
export function elegirMovimientoLider(battle: Battle, lado: Lado, aleatoriedad: number): string {
  if (Math.random() < aleatoriedad) {
    const opciones = movimientosUsables(battle, lado);
    if (opciones.length) return alAzar(opciones).eleccion;
  }
  return elegirMejorMovimiento(battle, lado);
}
