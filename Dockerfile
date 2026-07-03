FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/docket ./cmd/docket

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/docket /docket
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/docket"]
