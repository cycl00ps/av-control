FROM dhi.io/golang:1.27-debian13-dev AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/integra-web ./cmd/integra-web
RUN mkdir -p /out/data && touch /out/data/.keep

FROM dhi.io/static:20250419-debian13
WORKDIR /app
COPY --from=build /out/integra-web ./integra-web
COPY static/ ./static/
COPY --from=build --chown=65532:65532 /out/data/ ./data/
ENV INTEGRA_DATA_DIR=/app/data
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app/integra-web"]
