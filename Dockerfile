FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY bin/edi-bridge /app/edi-bridge
EXPOSE 8080
CMD ["/app/edi-bridge"]
