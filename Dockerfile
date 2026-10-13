# syntax=docker/dockerfile:1

# Web widget. It is optional: without web/package.json this stage produces an
# empty directory and the image serves the API only. With it, "npm run build"
# must write the static site to web/dist.
FROM node:22-alpine AS web
WORKDIR /src
COPY . .
RUN mkdir -p /out \
 && if [ -f web/package.json ]; then \
      cd web && npm ci && npm run build && cp -r dist/. /out/; \
    fi

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/slotbook ./cmd/slotbook

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/slotbook /app/slotbook
COPY --from=web /out /app/web
COPY seed /app/seed
ENV SLOTBOOK_ADDR=:8080 \
    SLOTBOOK_STATIC_DIR=/app/web
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/slotbook"]
CMD ["serve"]
