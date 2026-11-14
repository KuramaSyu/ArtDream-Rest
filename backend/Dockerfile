FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# nodynamic uses the embedded WASM libwebp build, so the runtime image needs
# no native image libraries.
RUN CGO_ENABLED=0 GOOS=linux go build -tags nodynamic -trimpath -ldflags="-s -w" -o /out/artdream .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/artdream /app/artdream

ENV ADDR=:8080 \
    DATA_DIR=/data \
    GIN_MODE=release

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/app/artdream"]
