FROM golang:1.27.2-alpine

# Pin a locale Alpine actually has - stops bash whining when the host
# leaks LC_ALL=en_US.UTF-8 in
ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# build-base/pkgconf/vips-dev - cgo for the -tags vips image path (air builds with it)
RUN apk add --no-cache git curl bash build-base pkgconf vips-dev

RUN --mount=type=cache,target=/root/go/pkg/mod \
    go install github.com/air-verse/air@latest && \
    CGO_ENABLED=0 go install github.com/amacneil/dbmate/v2@v2.36.0

# Seed /go/pkg/mod so a fresh container doesn't download everything on first boot - all three modules,
# laid out as in the repo so the relative replaces resolve
COPY apps/backend/sdk/go.mod apps/backend/sdk/go.sum /tmp/deps/sdk/
COPY apps/backend/core/go.mod apps/backend/core/go.sum /tmp/deps/core/
COPY apps/backend/signals/go.mod apps/backend/signals/go.sum /tmp/deps/signals/
RUN cd /tmp/deps/core && go mod download && cd /tmp/deps/signals && go mod download && rm -rf /tmp/deps

# The container runs as your uid - /go has to be writable for any uid
# Dev image, loose perms are fine
RUN chmod -R a+rwX /go /tmp

WORKDIR /var/www/backend/core

CMD ["go", "run", "-tags", "vips", "./cmd/server"]
