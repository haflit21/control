#!/bin/sh
# Client Docker statique pour les jobs de CI (le démon est dind : DOCKER_HOST).
set -eu
command -v docker >/dev/null 2>&1 && { docker version; exit 0; }
V=27.5.1
curl -fsSL "https://download.docker.com/linux/static/stable/x86_64/docker-$V.tgz" | tar xz -C /tmp docker/docker
install /tmp/docker/docker /usr/local/bin/docker
docker version
