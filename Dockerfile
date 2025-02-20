ARG VERSION

FROM golang:alpine AS builder
WORKDIR /build
RUN apk add --no-cache --update ca-certificates make git bash less vim yarn vips-dev gcc musl-dev
RUN ls -la
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -buildvcs=false ./cmd/web
RUN cd cmd/web && yarn && yarn production

FROM alpine
RUN apk add --no-cache vips htop curl
COPY --from=builder /build/web /
COPY --from=builder /build/cmd/web/dist /dist
COPY --from=builder /build/cmd/web/client /client
ARG VERSION
ENV VERSION $VERSION
ENV PORT 8080
EXPOSE 8080
ENV GIN_MODE=release
CMD ["/web"]
