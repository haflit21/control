# Control : une image, pnpm build → binaire Go qui embarque apps/web/dist.
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
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/control ./cmd/control && mkdir -p /out/state
# sops (binaire statique) déchiffre .cloud/secrets/secrets.enc.yaml au démarrage, avec la
# clé age montée en /run/secrets/age.key. Épinglé : une nouvelle version passe par un commit.
ARG SOPS_VERSION=3.13.3
ARG SOPS_SHA256=e5bec3346a873ae91d871550f3e698c1aad962aff462a080e40f25fde17fef6b
RUN curl -fsSL -o /out/sops "https://github.com/getsops/sops/releases/download/v${SOPS_VERSION}/sops-v${SOPS_VERSION}.linux.amd64" \
 && echo "${SOPS_SHA256}  /out/sops" | sha256sum -c - && chmod 755 /out/sops

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/control /app/control
COPY --from=build /out/sops /usr/local/bin/sops
# state dir (push keys + subscriptions, journal) owned by nonroot: a named volume mounted
# there inherits this ownership the first time, so the process can write to it
COPY --from=build --chown=nonroot:nonroot /out/state /data/state
COPY control.yaml /config/control.yaml
# secrets chiffrés (SOPS + age), à côté de la configuration
COPY .cloud/secrets/secrets.enc.yam[l] /config/.cloud/secrets/
COPY media /media
ENV CONTROL_CONFIG=/config/control.yaml CONTROL_LISTEN=:8080 CONTROL_STATE_DIR=/data/state CONTROL_MEDIA=/media TZ=Europe/Paris \
    SOPS_AGE_KEY_FILE=/run/secrets/age.key SOPS_DISABLE_VERSION_CHECK=1
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/app/control"]
CMD ["serve"]
