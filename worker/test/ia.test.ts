import { describe, expect, test } from "bun:test";
import { Battle, toID } from "pokemon-showdown";
import { danoEsperado } from "../src/sim/ia";
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
