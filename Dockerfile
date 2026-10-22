FROM ubuntu:26.04 AS security_provider
RUN useradd nonroot

# Importador de los datos: la API lo ejecuta si arranca con la base vacía.
FROM golang:1.27.3-alpine AS builder_pokedata
ARG POKEDATA_REF=3c068c6757baa216025426839e5a3fb2768acdc5
ENV CGO_ENABLED=0
WORKDIR /src
ADD https://github.com/dsaub/pokedata.git#${POKEDATA_REF} .
RUN go build -trimpath -ldflags="-s -w" -o /pokedata-gen ./cmd/pokedata-gen

FROM golang:1.27.3-alpine AS builder_app
ENV CGO_ENABLED=0
RUN mkdir -pv /build
WORKDIR /build
COPY api/go.mod api/go.sum /build/
RUN go mod download

COPY api/ .
RUN go build -o app ./main.go
# pokedata-gen descarga las fuentes en un directorio temporal.
RUN mkdir -m 1777 /tmp-vacio


FROM scratch AS production
COPY --from=security_provider /etc/passwd /etc/passwd
USER nonroot
WORKDIR /
COPY --from=builder_app /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder_app /tmp-vacio /tmp
COPY --from=builder_pokedata /pokedata-gen /pokedata-gen
COPY migraciones/003_v_gym_teams_formas.sql /migraciones/003_v_gym_teams_formas.sql
COPY --from=builder_app /build/app /app
ENTRYPOINT ["/app"]
