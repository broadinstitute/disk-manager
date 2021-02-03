ARG GO_VERSION='1.15'
ARG ALPINE_VERSION='3.13'

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} as build
WORKDIR /build
ENV CGO_ENABLED=0
ENV GO111MODULE=on
ENV GOBIN=/bin
COPY . .
RUN go build -o /bin/terra-disk-manager .

FROM alpine:${ALPINE_VERSION} as runtime
COPY --from=build /bin/terra-disk-manager /bin/terra-disk-manager
ENTRYPOINT [ "/bin/terra-disk-manager" ] 