FROM golang:1.27.2-alpine AS builder
ENV CGO_ENABLED=0

RUN mkdir -pv /build
WORKDIR /build
COPY . .

WORKDIR /build/pokedata
RUN go build -o pokedata-gen ./cmd/pokedata-gen
WORKDIR /build
RUN go build -o app ./main.go

FROM scratch
WORKDIR /
COPY --from=builder /build/pokedata/pokedata-gen /pokedata
COPY --from=builder /build/app /app