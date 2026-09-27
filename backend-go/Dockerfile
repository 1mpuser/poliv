# Multi-stage: соберём статический бинарник, в образ — только он, без Python и шелла.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/poliv .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/poliv /poliv
EXPOSE 8000
USER 65532:65532
ENTRYPOINT ["/poliv"]
CMD ["serve"]
