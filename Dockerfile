# syntax=docker/dockerfile:1

# --- Build stage -------------------------------------------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

# Cache module downloads when only source changes.
COPY go.mod ./
# go.sum is added once the module gains dependencies; copy if present.
COPY go.su[m] ./
RUN go mod download

COPY . .

# Static, stripped binary suitable for a distroless/scratch base.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/server

# --- Runtime stage -----------------------------------------------------------
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=build /out/server /app/server

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/server"]
