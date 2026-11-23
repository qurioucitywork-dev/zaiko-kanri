# Same runtime and binary as the existing Docker image; build.ps1 builds the app first.
FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY build/zaiko-server /app/zaiko-server
ENV ZAIKO_ADDRESS=0.0.0.0:8080 ZAIKO_ENV=production ZAIKO_COOKIE_SECURE=true
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/zaiko-server"]
