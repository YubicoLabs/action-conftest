FROM instrumenta/conftest:latest as conftest

FROM golang:1.14-alpine as builder
COPY main.go .
RUN go build -o /entrypoint

FROM alpine:3
COPY --from=conftest /conftest /usr/local/bin/conftest
COPY --from=builder /entrypoint /usr/local/bin/entrypoint
CMD [ "/usr/local/bin/entrypoint" ]
