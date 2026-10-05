ARG VERSION

FROM golang:alpine AS builder
# Keep equal to tools/Dockerfile's; CI's lint job checks it.
ARG SQL_MIGRATE_VERSION=v1.8.1
WORKDIR /build
RUN apk add --no-cache --update ca-certificates make git bash less vim yarn vips-dev gcc musl-dev
RUN ls -la
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -buildvcs=false ./cmd/web
RUN cd cmd/web && yarn && yarn production
RUN GOBIN=/usr/local/bin go install github.com/rubenv/sql-migrate/sql-migrate@${SQL_MIGRATE_VERSION}

FROM alpine
RUN apk add --no-cache vips htop curl
COPY --from=builder /build/web /
COPY --from=builder /build/cmd/web/dist /dist
COPY --from=builder /build/cmd/web/client /client
COPY --from=builder /usr/local/bin/sql-migrate /usr/local/bin/
COPY dbconfig.yml /
COPY migrations /migrations
WORKDIR /
ARG VERSION
ENV VERSION $VERSION
ENV PORT 8080
EXPOSE 8080
ENV GIN_MODE=release
CMD ["/web", "serve"]
