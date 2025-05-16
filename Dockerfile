FROM --platform=$BUILDPLATFORM quay.io/projectquay/golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src/go/app
COPY . .
RUN make build

FROM scratch
WORKDIR /
COPY --from=builder /src/go/app/kbot /
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/kbot"]
