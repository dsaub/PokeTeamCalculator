import { afterEach, describe, expect, spyOn, test } from "bun:test";
import { Battle, toID } from "pokemon-showdown";
import { danoEsperado, elegirMejorMovimiento, elegirMovimientoLider } from "../src/sim/ia";
import { leerPokemon } from "../src/sim/simular";

/** Combate recién empezado, ya en la petición de movimiento. */
function combate(gen: number, jugador: string, rival: string): Battle {
  const battle = new Battle({ formatid: toID(`gen${gen}customgame`) });
  battle.setPlayer("p1", { name: "jugador", team: [leerPokemon(jugador, gen, "Jugador")] });
  battle.setPlayer("p2", { name: "rival", team: [leerPokemon(rival, gen, "Rival")] });
  if (battle.p1.requestState === "teampreview") {
    battle.choose("p1", "default");
    battle.choose("p2", "default");
  }
  expect(battle.p1.requestState).toBe("move");
  return battle;
}

/** Daño esperado del primer Pokémon del jugador al del rival con `movimiento`. */
function dano(battle: Battle, movimiento: string): number {
  return danoEsperado(battle, battle.p1.active[0]!, battle.p2.active[0]!, battle.dex.moves.get(movimiento));
}

const RIVAL = "Snorlax\nLevel: 50\n- Rest";

describe("danoEsperado con los modificadores de Showdown", () => {
  test("Choice Band multiplica el ataque por 1,5", () => {
    const con = dano(combate(9, "Tauros @ Choice Band\nLevel: 50\n- Body Slam", RIVAL), "Body Slam");
    const sin = dano(combate(9, "Tauros\nLevel: 50\n- Body Slam", RIVAL), "Body Slam");
    expect(con / sin).toBeWithin(1.4, 1.6);
  });

  test("Huge Power duplica el ataque", () => {
    const con = dano(combate(9, "Azumarill\nAbility: Huge Power\nLevel: 50\n- Aqua Tail", RIVAL), "Aqua Tail");
    const sin = dano(combate(9, "Azumarill\nAbility: Thick Fat\nLevel: 50\n- Aqua Tail", RIVAL), "Aqua Tail");
    expect(con / sin).toBeWithin(1.85, 2.15);
  });

  test("Guts sube el ataque con quemadura y la quemadura no lo reduce", () => {
    const guts = combate(9, "Heracross\nAbility: Guts\nLevel: 50\n- Close Combat", RIVAL);
    const sinQuemar = dano(guts, "Close Combat");
    guts.p1.active[0]!.setStatus("brn");
    expect(dano(guts, "Close Combat") / sinQuemar).toBeWithin(1.4, 1.6);

    const moxie = combate(9, "Heracross\nAbility: Moxie\nLevel: 50\n- Close Combat", RIVAL);
    const moxieSano = dano(moxie, "Close Combat");
    moxie.p1.active[0]!.setStatus("brn");
    expect(dano(moxie, "Close Combat") / moxieSano).toBeWithin(0.4, 0.6);
  });

  test("Blaze en gen 9 funciona con pocos PS y no rompe el cálculo", () => {
    const battle = combate(9, "Charizard\nAbility: Blaze\nLevel: 100\n- Flamethrower", RIVAL);
    const lleno = dano(battle, "Flamethrower");
    battle.p1.active[0]!.hp = 1;
    expect(dano(battle, "Flamethrower") / lleno).toBeWithin(1.4, 1.6);
  });

  test("Blaze en gen 4 (evento BasePower) también cuenta", () => {
    const battle = combate(4, "Monferno\nAbility: Blaze\nLevel: 16\n- Ember", "Cranidos\nLevel: 14\n- Headbutt");
    const lleno = dano(battle, "Ember");
    battle.p1.active[0]!.hp = 1;
    expect(dano(battle, "Ember") / lleno).toBeWithin(1.3, 1.7);
  });

  test("si un manejador lanza, usa el valor sin modificar y deja el combate en buen estado", () => {
    const sinObjeto = dano(combate(9, "Tauros\nLevel: 50\n- Body Slam", RIVAL), "Body Slam");
    const battle = combate(9, "Tauros @ Choice Band\nLevel: 50\n- Body Slam", RIVAL);
    const original = battle.runEvent.bind(battle);
    // Imita un manejador que falla a mitad de evento: runEvent ya ha apilado
    // su contexto y no lo restaura.
    battle.runEvent = ((evento: string, ...resto: unknown[]) => {
      if (evento === "ModifyAtk") {
        battle.event = { id: evento };
        battle.eventDepth++;
        throw new TypeError("null is not an object (evaluating 'move.type')");
      }
      return (original as (...a: unknown[]) => unknown)(evento, ...resto);
    }) as typeof battle.runEvent;
    const eventoAntes = battle.event;
    const profundidadAntes = battle.eventDepth;

    expect(dano(battle, "Body Slam")).toBeCloseTo(sinObjeto, 5);
    expect(battle.eventDepth).toBe(profundidadAntes);
    expect(battle.event).toBe(eventoAntes);

    // El combate sigue jugándose hasta el final.
    battle.runEvent = original;
    for (let i = 0; i < 200 && !battle.ended; i++) {
      if (!battle.p1.isChoiceDone()) battle.choose("p1", "default");
      if (!battle.p2.isChoiceDone()) battle.choose("p2", "default");
    }
    expect(battle.ended).toBeTrue();
  });
});

describe("danoEsperado con los modificadores del daño final (ModifyDamage)", () => {
  /** Daño de `jugador` a `rival` con el único movimiento de `jugador`. */
  function danoEntre(jugador: string, rival: string, gen = 9): number {
    const battle = combate(gen, jugador, rival);
    return dano(battle, battle.p1.active[0]!.moveSlots[0]!.id);
  }

  test("Life Orb: ×1,3", () => {
    const con = danoEntre("Tauros @ Life Orb\nLevel: 50\n- Body Slam", RIVAL);
    const sin = danoEntre("Tauros\nLevel: 50\n- Body Slam", RIVAL);
    expect(con / sin).toBeCloseTo(5324 / 4096, 3);
  });

  test("Expert Belt: ×1,2 solo si es súper eficaz", () => {
    const eficaz = danoEntre("Lucario @ Expert Belt\nLevel: 50\n- Close Combat", RIVAL)
      / danoEntre("Lucario\nLevel: 50\n- Close Combat", RIVAL);
    expect(eficaz).toBeCloseTo(4915 / 4096, 3);
    const neutro = danoEntre("Lucario @ Expert Belt\nLevel: 50\n- Flash Cannon", RIVAL)
      / danoEntre("Lucario\nLevel: 50\n- Flash Cannon", RIVAL);
    expect(neutro).toBeCloseTo(1, 5);
  });

  test("Occa Berry: ×0,5 contra Fuego súper eficaz, sin comerse la baya ni escribir en el registro", () => {
    const sin = danoEntre("Charizard\nLevel: 50\n- Flamethrower", "Scizor\nLevel: 50\n- Bullet Punch");
    const battle = combate(9, "Charizard\nLevel: 50\n- Flamethrower", "Scizor @ Occa Berry\nLevel: 50\n- Bullet Punch");
    const lineasAntes = battle.log.length;
    expect(dano(battle, "Flamethrower") / sin).toBeCloseTo(0.5, 3);
    const scizor = battle.p2.active[0]!;
    expect(scizor.item).toBe("occaberry");
    expect(scizor.lastItem).toBe("");
    expect(battle.log.length).toBe(lineasAntes);
    // El contexto del movimiento activo vuelve a como estaba.
    expect(battle.activeMove).toBeNull();
    expect(battle.activePokemon).toBeNull();
    // Y la estimación se puede repetir: la baya sigue ahí.
    expect(dano(battle, "Flamethrower") / sin).toBeCloseTo(0.5, 3);
  });

  test("Multiscale: ×0,5 con los PS al máximo y ×1 si no", () => {
    const sin = danoEntre("Tauros\nLevel: 50\n- Body Slam", "Dragonite\nAbility: Inner Focus\nLevel: 50\n- Outrage");
    const battle = combate(9, "Tauros\nLevel: 50\n- Body Slam", "Dragonite\nAbility: Multiscale\nLevel: 50\n- Outrage");
    expect(dano(battle, "Body Slam") / sin).toBeCloseTo(0.5, 3);
    battle.p2.active[0]!.hp -= 1;
    expect(dano(battle, "Body Slam") / sin).toBeCloseTo(1, 5);
  });

  test("Filter: ×0,75 contra súper eficaces", () => {
    const con = danoEntre("Metagross\nLevel: 50\n- Meteor Mash", "Mr. Mime\nAbility: Filter\nLevel: 50\n- Psychic");
    const sin = danoEntre("Metagross\nLevel: 50\n- Meteor Mash", "Mr. Mime\nAbility: Soundproof\nLevel: 50\n- Psychic");
    expect(con / sin).toBeCloseTo(0.75, 3);
  });

  test("Solid Rock: ×0,75 contra súper eficaces", () => {
    const con = danoEntre("Blastoise\nLevel: 50\n- Surf", "Rhyperior\nAbility: Solid Rock\nLevel: 50\n- Earthquake");
    const sin = danoEntre("Blastoise\nLevel: 50\n- Surf", "Rhyperior\nAbility: Reckless\nLevel: 50\n- Earthquake");
    expect(con / sin).toBeCloseTo(0.75, 3);
  });

  test("Mold Breaker ignora Multiscale, como en Showdown", () => {
    const con = danoEntre("Haxorus\nAbility: Mold Breaker\nLevel: 50\n- Dragon Claw", "Dragonite\nAbility: Multiscale\nLevel: 50\n- Outrage");
    const sin = danoEntre("Haxorus\nAbility: Mold Breaker\nLevel: 50\n- Dragon Claw", "Dragonite\nAbility: Inner Focus\nLevel: 50\n- Outrage");
    expect(con / sin).toBeCloseTo(1, 5);
  });
});

describe("danoEsperado: casos especiales", () => {
  /** Daño del primer movimiento de `jugador` a `rival`. */
  function primero(jugador: string, rival: string, gen = 9): number {
    const battle = combate(gen, jugador, rival);
    return dano(battle, battle.p1.active[0]!.moveSlots[0]!.id);
  }

  test("los movimientos de estado no hacen daño", () => {
    expect(primero("Tauros\nLevel: 50\n- Growl", RIVAL)).toBe(0);
  });

  test("inmunidad por tipo: Normal contra Fantasma, Tierra contra Volador", () => {
    expect(primero("Tauros\nLevel: 50\n- Body Slam", "Gengar\nLevel: 50\n- Lick")).toBe(0);
    expect(primero("Garchomp\nLevel: 50\n- Earthquake", "Pidgeot\nLevel: 50\n- Gust")).toBe(0);
  });

  test("inmunidad por habilidad: Levitate y Flash Fire", () => {
    expect(primero("Garchomp\nLevel: 50\n- Earthquake", "Bronzong\nAbility: Levitate\nLevel: 50\n- Confusion")).toBe(0);
    expect(primero("Charizard\nLevel: 50\n- Flamethrower", "Heatran\nAbility: Flash Fire\nLevel: 50\n- Lava Plume")).toBe(0);
    expect(primero("Charizard\nLevel: 50\n- Flamethrower", "Heatran\nAbility: Flame Body\nLevel: 50\n- Lava Plume")).toBeGreaterThan(0);
  });

  test("Mold Breaker ignora la inmunidad por habilidad", () => {
    expect(primero("Excadrill\nAbility: Mold Breaker\nLevel: 50\n- Earthquake", "Bronzong\nAbility: Levitate\nLevel: 50\n- Confusion")).toBeGreaterThan(0);
  });

  test("Wonder Guard solo deja pasar los súper eficaces", () => {
    const shedinja = "Shedinja\nAbility: Wonder Guard\nLevel: 50\n- Scratch";
    expect(primero("Scizor\nLevel: 50\n- X-Scissor", shedinja)).toBe(0);
    expect(primero("Charizard\nLevel: 50\n- Flamethrower", shedinja)).toBeGreaterThan(0);
  });

  test("Lightning Rod y Storm Drain dan inmunidad solo desde gen 5", () => {
    const pararrayos = "Manectric\nAbility: Lightning Rod\nLevel: 50\n- Spark";
    const colector = "Gastrodon\nAbility: Storm Drain\nLevel: 50\n- Mud Bomb";
    expect(primero("Jolteon\nLevel: 50\n- Thunderbolt", pararrayos, 4)).toBeGreaterThan(0);
    expect(primero("Jolteon\nLevel: 50\n- Thunderbolt", pararrayos, 5)).toBe(0);
    expect(primero("Blastoise\nLevel: 50\n- Surf", colector, 4)).toBeGreaterThan(0);
    expect(primero("Blastoise\nLevel: 50\n- Surf", colector, 5)).toBe(0);
  });

  test("Struggle golpea incluso a los Fantasma", () => {
    expect(primero("Tauros\nLevel: 50\n- Struggle", "Gengar\nLevel: 50\n- Lick")).toBeGreaterThan(0);
  });

  test("OHKO: los PS del rival por la precisión, o 0 si es de más nivel", () => {
    const battle = combate(4, "Rhydon\nLevel: 50\n- Horn Drill", "Snorlax\nLevel: 40\n- Rest");
    expect(dano(battle, "Horn Drill")).toBeCloseTo(battle.p2.active[0]!.hp * 0.3, 5);
    expect(primero("Rhydon\nLevel: 50\n- Horn Drill", "Snorlax\nLevel: 60\n- Rest", 4)).toBe(0);
  });

  test("Super Fang quita la mitad de los PS actuales", () => {
    const battle = combate(9, "Raticate\nLevel: 50\n- Super Fang", RIVAL);
    const hp = battle.p2.active[0]!.hp;
    expect(dano(battle, "Super Fang")).toBeCloseTo(Math.floor(hp / 2) * 0.9, 5);
    battle.p2.active[0]!.hp = 1;
    expect(dano(battle, "Super Fang")).toBeCloseTo(0.9, 5);
  });

  test("daño fijo y por nivel", () => {
    expect(primero("Machamp\nLevel: 37\n- Seismic Toss", RIVAL)).toBe(37);
    expect(primero("Dratini\nLevel: 20\n- Dragon Rage", RIVAL, 4)).toBe(40);
  });

  test("potencia variable (Low Kick) se toma como 60", () => {
    const lowKick = primero("Machamp\nLevel: 50\n- Low Kick", RIVAL);
    const brickBreak = primero("Machamp\nLevel: 50\n- Brick Break", RIVAL);
    expect(lowKick / brickBreak).toBeWithin(0.75, 0.85);
  });

  test("golpes múltiples: 2–5 cuenta 3,1 y Double Kick, 2", () => {
    const bulletSeed = primero("Breloom\nAbility: Poison Heal\nLevel: 50\n- Bullet Seed", RIVAL);
    const seedBomb = primero("Breloom\nAbility: Poison Heal\nLevel: 50\n- Seed Bomb", RIVAL);
    expect(bulletSeed / seedBomb).toBeWithin(0.85, 1.05);
    const doubleKick = primero("Hitmonlee\nAbility: Limber\nLevel: 50\n- Double Kick", RIVAL);
    const karateChop = primero("Hitmonlee\nAbility: Limber\nLevel: 50\n- Karate Chop", RIVAL);
    expect(doubleKick / karateChop).toBeWithin(1.05, 1.35);
  });

  test("Adaptability: STAB ×2 en vez de ×1,5", () => {
    const con = primero("Eevee\nAbility: Adaptability\nLevel: 50\n- Body Slam", RIVAL);
    const sin = primero("Eevee\nAbility: Run Away\nLevel: 50\n- Body Slam", RIVAL);
    expect(con / sin).toBeWithin(1.25, 1.42);
  });

  test("la quemadura no afecta a los movimientos especiales", () => {
    const battle = combate(9, "Charizard\nLevel: 50\n- Flamethrower", RIVAL);
    const sano = dano(battle, "Flamethrower");
    battle.p1.active[0]!.setStatus("brn");
    expect(dano(battle, "Flamethrower")).toBeCloseTo(sano, 5);
  });
});

describe("elegir movimiento", () => {
  let azar: ReturnType<typeof spyOn<Math, "random">> | undefined;
  afterEach(() => {
    azar?.mockRestore();
    azar = undefined;
  });

  const movimientosDe = (battle: Battle) =>
    (battle.p1.activeRequest as { active: { moves: { pp: number; disabled?: boolean }[] }[] }).active[0]!.moves;

  test("elige el que más daño hace", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Tackle\n- Body Slam\n- Growl", RIVAL);
    expect(elegirMejorMovimiento(battle, "p1")).toBe("move 2");
  });

  test("sin movimientos usables → default", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Tackle\n- Body Slam", RIVAL);
    for (const m of movimientosDe(battle)) m.pp = 0;
    expect(elegirMejorMovimiento(battle, "p1")).toBe("default");
  });

  test("descarta los deshabilitados y los que no tienen PP", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Body Slam\n- Double-Edge\n- Tackle", RIVAL);
    const movs = movimientosDe(battle);
    movs[1]!.disabled = true;
    movs[0]!.pp = 0;
    expect(elegirMejorMovimiento(battle, "p1")).toBe("move 3");
  });

  test("si varios debilitan, prefiere la prioridad", () => {
    const battle = combate(9, "Machamp\nLevel: 100\n- Close Combat\n- Mach Punch", "Magikarp\nLevel: 5\n- Splash");
    expect(elegirMejorMovimiento(battle, "p1")).toBe("move 2");
  });

  test("si varios debilitan con la misma prioridad, prefiere el más preciso", () => {
    const battle = combate(9, "Machamp\nLevel: 100\n- Focus Blast\n- Close Combat", "Magikarp\nLevel: 5\n- Splash");
    expect(elegirMejorMovimiento(battle, "p1")).toBe("move 2");
  });

  test("entre empatados elige al azar", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Growl\n- Leer\n- Tail Whip", RIVAL);
    azar = spyOn(Math, "random").mockReturnValue(0);
    expect(elegirMejorMovimiento(battle, "p1")).toBe("move 1");
    azar.mockReturnValue(0.99);
    expect(elegirMejorMovimiento(battle, "p1")).toBe("move 3");
  });

  test("el líder con aleatoriedad 1 usa uno al azar aunque sea malo", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Growl\n- Body Slam", RIVAL);
    azar = spyOn(Math, "random").mockReturnValue(0);
    expect(elegirMovimientoLider(battle, "p1", 1)).toBe("move 1");
  });

  test("el líder con aleatoriedad 0 usa el mejor", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Growl\n- Body Slam", RIVAL);
    azar = spyOn(Math, "random").mockReturnValue(0);
    expect(elegirMovimientoLider(battle, "p1", 0)).toBe("move 2");
  });

  test("el líder al azar sin movimientos usables → default", () => {
    const battle = combate(9, "Tauros\nLevel: 50\n- Growl", RIVAL);
    for (const m of movimientosDe(battle)) m.pp = 0;
    azar = spyOn(Math, "random").mockReturnValue(0);
    expect(elegirMovimientoLider(battle, "p1", 1)).toBe("default");
  });
});

describe("parches de Showdown fuera de la estimación", () => {
  test("eatItem y add funcionan con normalidad", () => {
    const battle = combate(4, "Monferno @ Oran Berry\nLevel: 16\n- Ember", "Cranidos\nLevel: 14\n- Headbutt");
    const monferno = battle.p1.active[0]!;
    const lineas = battle.log.length;
    expect(monferno.eatItem(true)).toBeTruthy();
    expect(monferno.item).toBe("");
    battle.add("message", "hola");
    expect(battle.log.length).toBeGreaterThan(lineas);
  });
});
