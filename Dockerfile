FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/wheel ./cmd/wheel

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 wheel
USER wheel

COPY --from=build /out/wheel /usr/local/bin/wheel

ENV ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/wheel"]
