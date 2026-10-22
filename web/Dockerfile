FROM oven/bun:1.4.3-alpine AS build

WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .
RUN bunx vite build

# Debian y no Alpine: el cortafuegos del sandbox de Daytona donde se construye
# la imagen amd64 bloquea los mirrors de Alpine (ver worker/Dockerfile).
FROM nginx:1.29

# Parches de seguridad que aún no trae la imagen base.
RUN apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80
