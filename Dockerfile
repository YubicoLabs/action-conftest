FROM openpolicyagent/conftest:v0.38.0 as conftest

FROM golang:1.15-alpine as builder
COPY --from=conftest /conftest /usr/local/bin/conftest
COPY main.go .
RUN go build -o /entrypoint main.go
CMD [ "/entrypoint" ]
