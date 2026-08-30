FROM golang:1.27.0-alpine

# Pin a locale Alpine actually has - stops bash whining when the host
# leaks LC_ALL=en_US.UTF-8 in
ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# postgresql18-client - pg_dump for internal/backup (client major >= server major)
# build-base/pkgconf/vips-dev - cgo for the -tags vips image path (air builds with it)
RUN apk add --no-cache git curl bash postgresql18-client build-base pkgconf vips-dev

RUN --mount=type=cache,target=/root/go/pkg/mod \
    go install github.com/air-verse/air@latest && \
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# Seed /go/pkg/mod so a fresh container doesn't download everything on first boot
COPY apps/backend/go.mod apps/backend/go.sum /tmp/deps/
RUN cd /tmp/deps && go mod download && rm -rf /tmp/deps

# The container runs as your uid - /go has to be writable for any uid
# Dev image, loose perms are fine
RUN chmod -R a+rwX /go /tmp

WORKDIR /var/www/app

CMD ["go", "run", "-tags", "vips", "./cmd/server"]
