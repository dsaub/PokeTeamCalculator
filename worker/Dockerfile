# Debian y no Alpine: la imagen amd64 se construye en un sandbox de Daytona cuyo
# cortafuegos bloquea los mirrors de Alpine pero deja pasar deb.debian.org.
FROM oven/bun:1.4.3-slim

# Parches de seguridad que aún no trae la imagen base.
RUN apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY package.json bun.lock ./
# Sin devDependencies ni opcionales de Showdown (sqlite3, nodemailer): el
# worker solo usa el simulador. esbuild solo lo usan las herramientas de build
# de Showdown, que vienen precompiladas en dist/.
RUN bun install --frozen-lockfile --production --omit=optional \
    && rm -rf node_modules/esbuild node_modules/@esbuild

COPY src ./src
COPY herramientas ./herramientas

ENV NODE_ENV=production
USER bun

CMD ["bun", "src/index.ts"]
