FROM oven/bun:1.4.3-alpine

# Parches de seguridad de Alpine (musl, zlib) que aún no trae la imagen base.
RUN apk upgrade --no-cache

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
