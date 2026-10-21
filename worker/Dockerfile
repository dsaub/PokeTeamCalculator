FROM oven/bun:1.4.3-alpine

WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY src ./src
COPY herramientas ./herramientas

ENV NODE_ENV=production
USER bun

CMD ["bun", "src/index.ts"]
