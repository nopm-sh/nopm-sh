FROM golang as builder

WORKDIR /build/nopm-sh
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

FROM debian

COPY --from=builder /go/bin/nopm-sh /usr/local/bin/nopm-sh
COPY --from=builder /build/nopm-sh/templates /nopm-sh/templates

EXPOSE 8080

CMD nopm-sh --templates-dir /nopm-sh/templates
