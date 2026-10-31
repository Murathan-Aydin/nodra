# Image de Nodra : front React (s'il existe) embarqué dans un binaire Go statique.
# docker build --build-arg VERSION=v1.0.0 -t ghcr.io/murathan-aydin/nodra:v1.0.0 .

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/ ./
RUN if [ -f package.json ]; then \
      (npm ci || npm install) && npm run build; \
    fi

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/Murathan-Aydin/nodra/internal/version.Version=${VERSION}" \
      -o /out/nodra ./cmd/nodra

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nodra /nodra
ENV NODRA_ADDR=:8000
EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=6s --start-period=20s --retries=5 CMD ["/nodra", "healthcheck"]
ENTRYPOINT ["/nodra"]
CMD ["serve"]
