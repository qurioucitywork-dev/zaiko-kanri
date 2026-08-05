FROM node:24-alpine AS frontend
WORKDIR /src/frontend
RUN corepack enable && corepack prepare pnpm@11.9.0 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

FROM golang:1.26 AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/web/react-dist ./internal/web/react-dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/zaiko-server ./cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=backend /out/zaiko-server /app/zaiko-server
ENV ZAIKO_ADDRESS=0.0.0.0:8080 \
    ZAIKO_ENV=production \
    ZAIKO_COOKIE_SECURE=true
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/zaiko-server"]
