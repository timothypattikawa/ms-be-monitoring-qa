FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/api /out/worker ./
COPY errorContract.json .env ./
# .env is baked in (loaded from cwd; real env vars override it) — keep this image private.
ENV APP_HOST=0.0.0.0
EXPOSE 8080
# worker: override with `command: ./worker`
CMD ["./api"]
