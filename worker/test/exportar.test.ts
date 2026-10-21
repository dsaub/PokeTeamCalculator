import { describe, expect, test } from "bun:test";
import { aTextoExportacion } from "../src/sim/exportar";
import { ErrorPeticion } from "../src/sim/tipos";

function errorDe(fn: () => unknown): ErrorPeticion {
  try {
    fn();
  } catch (e) {
    expect(e).toBeInstanceOf(ErrorPeticion);
    return e as ErrorPeticion;
  }
  throw new Error("se esperaba un ErrorPeticion");
}

describe("aTextoExportacion", () => {
  test("genera el texto de exportación completo", () => {
    const texto = aTextoExportacion({
      especie: "Monferno",
      nivel: 16,
      habilidad: "Blaze",
      objeto: "Oran Berry",
      naturaleza: "Adamant",
      evs: { atk: 20, spe: 12 },
      ivs: { hp: 20, atk: 25 },
      movimientos: ["Mach Punch", "Ember"],
    }, "Candidato 1");
    expect(texto).toBe([
      "Monferno @ Oran Berry",
      "Ability: Blaze",
      "Level: 16",
      "EVs: 20 Atk / 12 Spe",
      "Adamant Nature",
      "IVs: 20 HP / 25 Atk",
      "- Mach Punch",
      "- Ember",
    ].join("\n"));
  });

  test("canoniza los nombres", () => {
    const texto = aTextoExportacion({
      especie: "mr mime", habilidad: "soundproof", objeto: "oranberry", naturaleza: "ADAMANT",
      movimientos: ["mach punch", "PSYCHIC"],
    }, "Rival");
    expect(texto).toContain("Mr. Mime @ Oran Berry");
    expect(texto).toContain("Ability: Soundproof");
    expect(texto).toContain("Adamant Nature");
    expect(texto).toContain("- Mach Punch");
    expect(texto).toContain("- Psychic");
  });

  test("omite EVs e IVs si no hay nada que poner y usa nivel 100 por defecto", () => {
    const texto = aTextoExportacion({ especie: "Pikachu", movimientos: ["Thunderbolt"], evs: { hp: 0 }, ivs: { atk: 31 } }, "Rival");
    expect(texto).toBe("Pikachu\nLevel: 100\n- Thunderbolt");
  });

  test("rechaza inyecciones con saltos de línea", () => {
    expect(errorDe(() => aTextoExportacion({ especie: "Pikachu\n- Explosion", movimientos: ["Tackle"] }, "Rival")).message)
      .toStartWith("Rival:");
    errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: ["Tackle\n- Explosion"] }, "Rival"));
    errorDe(() => aTextoExportacion({ especie: "Pikachu", objeto: "Leftovers\nAbility: Huge Power", movimientos: ["Tackle"] }, "Rival"));
    errorDe(() => aTextoExportacion({ especie: "Pikachu", habilidad: "Static\n- Explosion", movimientos: ["Tackle"] }, "Rival"));
    errorDe(() => aTextoExportacion({ especie: "Pikachu", naturaleza: "Adamant\nLevel: 100", movimientos: ["Tackle"] }, "Rival"));
  });

  test("rechaza valores fuera de rango con mensajes que dicen quién falla", () => {
    const base = { especie: "Pikachu", movimientos: ["Tackle"] };
    expect(errorDe(() => aTextoExportacion({ ...base, nivel: 0 }, "Candidato 2")).message)
      .toBe("Candidato 2: el nivel debe ser un número entero entre 1 y 100.");
    errorDe(() => aTextoExportacion({ ...base, nivel: 101 }, "Rival"));
    errorDe(() => aTextoExportacion({ ...base, nivel: 5.5 }, "Rival"));
    errorDe(() => aTextoExportacion({ ...base, nivel: "50" }, "Rival"));
    errorDe(() => aTextoExportacion({ ...base, evs: { atk: 253 } }, "Rival"));
    errorDe(() => aTextoExportacion({ ...base, evs: { atk: -1 } }, "Rival"));
    expect(errorDe(() => aTextoExportacion({ ...base, evs: { hp: 252, atk: 252, def: 10 } }, "Rival")).message)
      .toContain("510");
    errorDe(() => aTextoExportacion({ ...base, ivs: { spe: 32 } }, "Rival"));
    errorDe(() => aTextoExportacion({ ...base, ivs: { velocidad: 31 } }, "Rival"));
    errorDe(() => aTextoExportacion({ ...base, evs: [252] }, "Rival"));
  });

  test("valida los movimientos", () => {
    errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: [] }, "Rival"));
    errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: ["Tackle", "Growl", "Thunder", "Surf", "Fly"] }, "Rival"));
    expect(errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: ["Tackle", "tackle"] }, "Rival")).message)
      .toContain("repetido");
    errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: ["Patada Voladora"] }, "Rival"));
    errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: [42] }, "Rival"));
    errorDe(() => aTextoExportacion({ especie: "Pikachu", movimientos: "Tackle" }, "Rival"));
  });

  test("rechaza entradas sin formato de Pokémon", () => {
    errorDe(() => aTextoExportacion(null, "Rival"));
    errorDe(() => aTextoExportacion("Pikachu", "Rival"));
    errorDe(() => aTextoExportacion({ movimientos: ["Tackle"] }, "Rival"));
  });
});
