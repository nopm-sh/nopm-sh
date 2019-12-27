FROM golang as builder

WORKDIR /build/nopm-sh
COPY . .

RUN go get -d -v ./...
RUN make build

FROM debian

RUN apt-get update && apt-get install -y ca-certificates

COPY --from=builder /build/nopm-sh/nopm-sh /usr/local/bin/nopm-sh
COPY --from=builder /build/nopm-sh/templates /nopm-sh/templates

EXPOSE 8080

CMD nopm-sh --templates-dir /nopm-sh/templates
