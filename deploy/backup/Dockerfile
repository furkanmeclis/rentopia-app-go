# Image of the compose `backup` service (profile "backup"): scripts/backup.sh
# in a daily loop. It drives the stack's containers through the mounted
# Docker socket (docker exec / docker run), so pg_dump and redis-cli are the
# versions of the running postgres / redis images.
# Build context: the repo root (compose.prod.yml); Dockerfile.dockerignore
# keeps the context to scripts/backup*.sh.
FROM alpine:3.22

RUN apk add --no-cache bash coreutils tzdata age docker-cli

COPY scripts/backup.sh scripts/backup-common.sh /app/scripts/
RUN chmod 0755 /app/scripts/backup.sh

ENV TZ=Europe/Istanbul
ENTRYPOINT ["/app/scripts/backup.sh"]
CMD ["schedule"]
