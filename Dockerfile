FROM ubuntu:26.04 AS security_provider
RUN useradd nonroot

FROM golang:1.27.2-alpine AS builder_app
ENV CGO_ENABLED=0
RUN mkdir -pv /build
WORKDIR /build
COPY go.mod go.sum /build/
RUN go mod download

COPY . .
RUN go build -o app ./main.go


FROM scratch AS production
COPY --from=security_provider /etc/passwd /etc/passwd
USER nonroot
WORKDIR /
COPY --from=builder_app /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder_app /build/app /app
ENTRYPOINT ["/app"]
