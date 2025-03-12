FROM openpolicyagent/conftest:v0.58.0 AS conftest

FROM golang:1.24-alpine AS builder
COPY --from=conftest /conftest /usr/local/bin/conftest
COPY main.go .
RUN go build -o /entrypoint main.go
CMD [ "/entrypoint" ]
