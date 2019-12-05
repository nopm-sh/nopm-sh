FROM golang as builder

WORKDIR /build/nopm-sh
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

FROM debian

COPY --from=builder /go/bin/nopm-sh /usr/local/bin/nopm-sh
EXPOSE 80
CMD nopm-sh
