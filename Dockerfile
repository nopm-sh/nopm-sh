FROM golang as builder

WORKDIR /build/nopm-sh
COPY . .

RUN go get -d -v ./...
RUN make build

FROM debian

COPY --from=builder /build/nopm-sh/nopm-sh /usr/local/bin/nopm-sh
COPY --from=builder /build/nopm-sh/templates /nopm-sh/templates

EXPOSE 8080

CMD nopm-sh --templates-dir /nopm-sh/templates
