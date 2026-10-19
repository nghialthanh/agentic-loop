# Build a runnable agentic-loop worker image.
FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/worker /worker
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/worker"]
