FROM golang:1.24.2 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o whytho cmd/main.go

FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /app/whytho .

EXPOSE 8080

CMD ["/whytho"]
