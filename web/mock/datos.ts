// Catálogo de ejemplo del mock. Datos reducidos y no siempre fieles a los juegos: sirven para
// probar la interfaz, no para jugar. Los ids van en inglés y formato Showdown, como en la API.

import type { Juego, Lider, Movimiento } from '../src/api/tipos.ts'

type MovimientoMock = Movimiento & { gen: number }

const mv = (id: string, nombre: string, tipo: string, categoria: Movimiento['categoria'], potencia: number, gen: number): MovimientoMock => ({
  id,
  nombre,
  tipo,
  categoria,
  potencia,
  gen,
})

export const MOVIMIENTOS: Record<string, MovimientoMock> = Object.fromEntries(
  [
    mv('Tackle', 'Placaje', 'Normal', 'physical', 35, 1),
    mv('Scratch', 'Arañazo', 'Normal', 'physical', 40, 1),
    mv('Pound', 'Destructor', 'Normal', 'physical', 40, 1),
    mv('Headbutt', 'Golpe Cabeza', 'Normal', 'physical', 70, 1),
    mv('Quick Attack', 'Ataque Rápido', 'Normal', 'physical', 40, 1),
    mv('Fury Swipes', 'Golpes Furia', 'Normal', 'physical', 18, 1),
    mv('Bind', 'Atadura', 'Normal', 'physical', 15, 1),
    mv('Swift', 'Rapidez', 'Normal', 'special', 60, 1),
    mv('Growl', 'Gruñido', 'Normal', 'status', 0, 1),
    mv('Leer', 'Malicioso', 'Normal', 'status', 0, 1),
    mv('Tail Whip', 'Látigo', 'Normal', 'status', 0, 1),
    mv('Defense Curl', 'Rizo Defensa', 'Normal', 'status', 0, 1),
    mv('Harden', 'Fortaleza', 'Normal', 'status', 0, 1),
    mv('Screech', 'Chirrido', 'Normal', 'status', 0, 1),
    mv('Growth', 'Desarrollo', 'Normal', 'status', 0, 1),
    mv('Bite', 'Mordisco', 'Dark', 'physical', 60, 1),
    mv('Pursuit', 'Persecución', 'Dark', 'physical', 40, 2),
    mv('Ember', 'Ascuas', 'Fire', 'special', 40, 1),
    mv('Flame Wheel', 'Rueda Fuego', 'Fire', 'physical', 60, 2),
    mv('Sunny Day', 'Día Soleado', 'Fire', 'status', 0, 2),
    mv('Karate Chop', 'Golpe Kárate', 'Fighting', 'physical', 50, 1),
    mv('Mach Punch', 'Ultrapuño', 'Fighting', 'physical', 40, 2),
    mv('Rock Smash', 'Golpe Roca', 'Fighting', 'physical', 40, 2),
    mv('Bubble', 'Burbuja', 'Water', 'special', 20, 1),
    mv('Water Gun', 'Pistola Agua', 'Water', 'special', 40, 1),
    mv('Bubble Beam', 'Rayo Burbuja', 'Water', 'special', 65, 1),
    mv('Water Pulse', 'Hidropulso', 'Water', 'special', 60, 3),
    mv('Peck', 'Picotazo', 'Flying', 'physical', 35, 1),
    mv('Gust', 'Tornado', 'Flying', 'special', 40, 1),
    mv('Wing Attack', 'Ataque Ala', 'Flying', 'physical', 60, 1),
    mv('Absorb', 'Absorber', 'Grass', 'special', 20, 1),
    mv('Mega Drain', 'Megaagotar', 'Grass', 'special', 40, 1),
    mv('Razor Leaf', 'Hoja Afilada', 'Grass', 'physical', 55, 1),
    mv('Vine Whip', 'Látigo Cepa', 'Grass', 'physical', 35, 1),
    mv('Magical Leaf', 'Hoja Mágica', 'Grass', 'special', 60, 3),
    mv('Leech Seed', 'Drenadoras', 'Grass', 'status', 0, 1),
    mv('Stun Spore', 'Paralizador', 'Grass', 'status', 0, 1),
    mv('Thunder Shock', 'Impactrueno', 'Electric', 'special', 40, 1),
    mv('Spark', 'Chispa', 'Electric', 'physical', 65, 2),
    mv('Thunder Wave', 'Onda Trueno', 'Electric', 'status', 0, 1),
    mv('Confusion', 'Confusión', 'Psychic', 'special', 50, 1),
    mv('Teleport', 'Teletransporte', 'Psychic', 'status', 0, 1),
    mv('Rock Throw', 'Lanzarrocas', 'Rock', 'physical', 50, 1),
    mv('Rock Tomb', 'Tumba Rocas', 'Rock', 'physical', 50, 3),
    mv('Mud-Slap', 'Bofetón Lodo', 'Ground', 'special', 20, 2),
    mv('Sand Attack', 'Ataque Arena', 'Ground', 'status', 0, 1),
    mv('Poison Sting', 'Picotazo Veneno', 'Poison', 'physical', 15, 1),
    mv('Metal Claw', 'Garra Metal', 'Steel', 'physical', 50, 3),
  ].map((m) => [m.id, m]),
)

interface PokemonMock {
  especie: string
  nombre: string
  gen: number
  tipos: string[]
  habilidades: [string, string][]
  movimientos: string[]
}

const pk = (
  especie: string,
  nombre: string,
  gen: number,
  tipos: string[],
  habilidades: [string, string][],
  movimientos: string[],
): PokemonMock => ({ especie, nombre, gen, tipos, habilidades, movimientos })

const ESPESURA: [string, string] = ['Overgrow', 'Espesura']
const MAR_LLAMAS: [string, string] = ['Blaze', 'Mar Llamas']
const TORRENTE: [string, string] = ['Torrent', 'Torrente']

export const POKEMON: PokemonMock[] = [
  pk('Bulbasaur', 'Bulbasaur', 1, ['Grass', 'Poison'], [ESPESURA], ['Tackle', 'Growl', 'Vine Whip', 'Leech Seed', 'Razor Leaf', 'Stun Spore', 'Growth']),
  pk('Charmander', 'Charmander', 1, ['Fire'], [MAR_LLAMAS], ['Scratch', 'Growl', 'Ember', 'Leer', 'Metal Claw', 'Flame Wheel']),
  pk('Squirtle', 'Squirtle', 1, ['Water'], [TORRENTE], ['Tackle', 'Tail Whip', 'Bubble', 'Water Gun', 'Bite', 'Water Pulse']),
  pk('Pikachu', 'Pikachu', 1, ['Electric'], [['Static', 'Electricidad Estática']], ['Thunder Shock', 'Growl', 'Tail Whip', 'Quick Attack', 'Thunder Wave', 'Spark', 'Swift']),
  pk('Pidgey', 'Pidgey', 1, ['Normal', 'Flying'], [['Keen Eye', 'Vista Lince']], ['Tackle', 'Sand Attack', 'Gust', 'Quick Attack', 'Wing Attack']),
  pk('Geodude', 'Geodude', 1, ['Rock', 'Ground'], [['Rock Head', 'Cabeza Roca'], ['Sturdy', 'Robustez']], ['Tackle', 'Defense Curl', 'Rock Throw', 'Rock Tomb', 'Mud-Slap']),
  pk('Onix', 'Onix', 1, ['Rock', 'Ground'], [['Rock Head', 'Cabeza Roca'], ['Sturdy', 'Robustez']], ['Tackle', 'Harden', 'Bind', 'Rock Throw', 'Screech', 'Rock Tomb']),
  pk('Machop', 'Machop', 1, ['Fighting'], [['Guts', 'Agallas']], ['Karate Chop', 'Leer', 'Rock Smash', 'Mach Punch']),
  pk('Abra', 'Abra', 1, ['Psychic'], [['Synchronize', 'Sincronía'], ['Inner Focus', 'Fuerza Mental']], ['Teleport', 'Confusion']),
  pk('Psyduck', 'Psyduck', 1, ['Water'], [['Damp', 'Humedad'], ['Cloud Nine', 'Aclimatación']], ['Scratch', 'Tail Whip', 'Water Gun', 'Confusion', 'Fury Swipes', 'Water Pulse']),
  pk('Staryu', 'Staryu', 1, ['Water'], [['Illuminate', 'Iluminación'], ['Natural Cure', 'Cura Natural']], ['Tackle', 'Harden', 'Water Gun', 'Swift', 'Bubble Beam']),
  pk('Starmie', 'Starmie', 1, ['Water', 'Psychic'], [['Illuminate', 'Iluminación'], ['Natural Cure', 'Cura Natural']], ['Tackle', 'Water Gun', 'Swift', 'Confusion', 'Bubble Beam']),
  pk('Mr. Mime', 'Mr. Mime', 1, ['Psychic'], [['Soundproof', 'Insonorizar'], ['Filter', 'Filtro']], ['Pound', 'Confusion', 'Swift']),
  pk('Turtwig', 'Turtwig', 4, ['Grass'], [ESPESURA], ['Tackle', 'Absorb', 'Razor Leaf', 'Bite', 'Mega Drain']),
  pk('Grotle', 'Grotle', 4, ['Grass'], [ESPESURA], ['Tackle', 'Absorb', 'Razor Leaf', 'Bite', 'Mega Drain', 'Leech Seed']),
  pk('Chimchar', 'Chimchar', 4, ['Fire'], [MAR_LLAMAS], ['Scratch', 'Leer', 'Ember', 'Fury Swipes']),
  pk('Monferno', 'Monferno', 4, ['Fire', 'Fighting'], [MAR_LLAMAS, ['Iron Fist', 'Puño Férreo']], ['Scratch', 'Leer', 'Ember', 'Mach Punch', 'Fury Swipes', 'Flame Wheel']),
  pk('Piplup', 'Piplup', 4, ['Water'], [TORRENTE], ['Pound', 'Growl', 'Bubble', 'Peck', 'Bubble Beam']),
  pk('Prinplup', 'Prinplup', 4, ['Water'], [TORRENTE], ['Pound', 'Growl', 'Bubble', 'Peck', 'Bubble Beam', 'Metal Claw']),
  pk('Starly', 'Starly', 4, ['Normal', 'Flying'], [['Keen Eye', 'Vista Lince']], ['Tackle', 'Growl', 'Quick Attack', 'Wing Attack']),
  pk('Staravia', 'Staravia', 4, ['Normal', 'Flying'], [['Intimidate', 'Intimidación']], ['Tackle', 'Growl', 'Quick Attack', 'Wing Attack']),
  pk('Bidoof', 'Bidoof', 4, ['Normal'], [['Simple', 'Simple'], ['Unaware', 'Ignorante']], ['Tackle', 'Growl', 'Defense Curl', 'Headbutt']),
  pk('Shinx', 'Shinx', 4, ['Electric'], [['Rivalry', 'Rivalidad'], ['Intimidate', 'Intimidación']], ['Tackle', 'Leer', 'Bite', 'Spark', 'Thunder Wave']),
  pk('Luxio', 'Luxio', 4, ['Electric'], [['Rivalry', 'Rivalidad'], ['Intimidate', 'Intimidación']], ['Tackle', 'Leer', 'Bite', 'Spark']),
  pk('Budew', 'Budew', 4, ['Grass', 'Poison'], [['Natural Cure', 'Cura Natural'], ['Poison Point', 'Punto Tóxico']], ['Absorb', 'Growth', 'Stun Spore', 'Mega Drain', 'Poison Sting']),
  pk('Cherubi', 'Cherubi', 4, ['Grass'], [['Chlorophyll', 'Clorofila']], ['Tackle', 'Growth', 'Leech Seed', 'Magical Leaf', 'Sunny Day']),
  pk('Roserade', 'Roserade', 4, ['Grass', 'Poison'], [['Natural Cure', 'Cura Natural'], ['Poison Point', 'Punto Tóxico']], ['Magical Leaf', 'Mega Drain', 'Stun Spore', 'Poison Sting']),
  pk('Cranidos', 'Cranidos', 4, ['Rock'], [['Mold Breaker', 'Rompemoldes']], ['Headbutt', 'Pursuit', 'Leer', 'Rock Tomb']),
]

export const JUEGOS: Juego[] = [
  { id: 'platino', nombre: 'Pokémon Platino', gen: 'gen4' },
  { id: 'rojo', nombre: 'Pokémon Rojo', gen: 'gen1' },
]

const delLider = (especie: string, nivel: number, habilidad: string | null, movimientos: string[]) => {
  const p = POKEMON.find((x) => x.especie === especie)!
  return { especie, nombre: p.nombre, nivel, habilidad, objeto: null, movimientos, tipos: p.tipos }
}

export const LIDERES: Record<string, Lider[]> = {
  platino: [
    {
      id: 1,
      nombre: 'Roco',
      ciudad: 'Ciudad Pirita',
      tipo: 'Rock',
      orden: 1,
      equipo: [
        delLider('Geodude', 12, 'Sturdy', ['Tackle', 'Defense Curl', 'Rock Throw']),
        delLider('Onix', 12, 'Rock Head', ['Bind', 'Rock Throw', 'Screech']),
        delLider('Cranidos', 14, 'Mold Breaker', ['Headbutt', 'Pursuit', 'Leer']),
      ],
    },
    {
      id: 2,
      nombre: 'Gardenia',
      ciudad: 'Ciudad Vetusta',
      tipo: 'Grass',
      orden: 2,
      equipo: [
        delLider('Cherubi', 20, 'Chlorophyll', ['Leech Seed', 'Magical Leaf', 'Growth']),
        delLider('Turtwig', 20, 'Overgrow', ['Razor Leaf', 'Bite', 'Absorb']),
        delLider('Roserade', 22, 'Natural Cure', ['Magical Leaf', 'Stun Spore', 'Poison Sting', 'Mega Drain']),
      ],
    },
  ],
  rojo: [
    {
      id: 3,
      nombre: 'Brock',
      ciudad: 'Ciudad Plateada',
      tipo: 'Rock',
      orden: 1,
      equipo: [delLider('Geodude', 12, null, ['Tackle', 'Defense Curl']), delLider('Onix', 14, null, ['Tackle', 'Screech', 'Bind'])],
    },
    {
      id: 4,
      nombre: 'Misty',
      ciudad: 'Ciudad Celeste',
      tipo: 'Water',
      orden: 2,
      equipo: [delLider('Staryu', 18, null, ['Tackle', 'Water Gun']), delLider('Starmie', 21, null, ['Tackle', 'Water Gun', 'Bubble Beam'])],
    },
  ],
}
