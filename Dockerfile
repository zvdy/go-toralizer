FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/go-toralizer ./cmd/go-toralizer

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/go-toralizer /usr/local/bin/go-toralizer
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/go-toralizer"]
