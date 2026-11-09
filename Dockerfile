# Maison : une image, pnpm build → binaire Go qui embarque apps/web/dist.
FROM node:22-bookworm-slim AS ui
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml .npmrc tsconfig.base.json ./
COPY packages ./packages
COPY apps ./apps
COPY docs/design-system ./docs/design-system
RUN pnpm install --frozen-lockfile && pnpm build
# the build writes into server/internal/server/ui/dist (vite outDir)

FROM golang:1.24-bookworm AS build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
COPY --from=ui /src/server/internal/server/ui/dist ./internal/server/ui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/maison ./cmd/maison && mkdir -p /out/state

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/maison /app/maison
# state dir (push keys + subscriptions, journal) owned by nonroot: a named volume mounted
# there inherits this ownership the first time, so the process can write to it
COPY --from=build --chown=nonroot:nonroot /out/state /data/state
COPY maison.yaml /config/maison.yaml
COPY media /media
ENV MAISON_CONFIG=/config/maison.yaml MAISON_LISTEN=:8080 MAISON_STATE_DIR=/data/state MAISON_MEDIA=/media TZ=Europe/Paris
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/app/maison"]
CMD ["serve"]
