// Prueba el simulador a mano, sin hilos ni base de datos.
//
//   bun herramientas/cli.ts herramientas/ejemplo.json            tabla de resultados
//   bun herramientas/cli.ts herramientas/ejemplo.json --json     el Resultado que verá Go
//   bun herramientas/cli.ts herramientas/ejemplo.json --textos   los textos de exportación
//   cat peticion.json | bun herramientas/cli.ts                  lee de stdin

import { prepararPeticion, simularPeticion } from "../src/sim/simular";
import { ErrorPeticion, type Peticion } from "../src/sim/tipos";

const args = process.argv.slice(2);
const modoJson = args.includes("--json");
const modoTextos = args.includes("--textos");
const ruta = args.find((a) => !a.startsWith("--"));

async function leerEntrada(): Promise<string> {
  if (ruta) return Bun.file(ruta).text();
  return Bun.stdin.text();
}

let peticion: Peticion;
try {
  peticion = JSON.parse(await leerEntrada());
} catch (e) {
  console.error(`No se pudo leer la petición: ${(e as Error).message}`);
  process.exit(1);
}

try {
  if (modoTextos) {
    const { textos } = prepararPeticion(peticion);
    console.log(`# Rival\n${textos.rival}\n`);
    textos.candidatos.forEach((t, i) => console.log(`# Candidato ${i + 1}\n${t}\n`));
  } else {
    const resultado = simularPeticion(peticion);
    if (modoJson) {
      console.log(JSON.stringify(resultado, null, 2));
    } else {
      console.log(`${resultado.gen} · rival: ${resultado.rival} · ${resultado.simulaciones} combates por candidato`);
      console.table(resultado.resultados.map((r) => ({
        pokemon: `${r.pokemon} (Nv. ${r.nivel})`,
        winrate: `${(r.winrate * 100).toFixed(1)} %`,
        victorias: r.victorias,
        derrotas: r.derrotas,
        empates: r.empates,
        "PS medios": `${(r.psMedioVictoria * 100).toFixed(1)} %`,
        turnos: Number(r.turnosMedio.toFixed(1)),
      })));
      const combates = resultado.simulaciones * resultado.resultados.length;
      console.log(`${resultado.ms} ms en total · ${(resultado.ms / combates).toFixed(2)} ms por combate`);
    }
  }
} catch (e) {
  if (e instanceof ErrorPeticion) {
    console.error(`Petición no válida: ${e.message}`);
    process.exit(2);
  }
  throw e;
}
