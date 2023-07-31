ARG ARCH="amd64"
ARG OS="linux"

FROM golang:1.20-alpine as build

COPY . /app
WORKDIR /app

RUN go get -d -v && go build -o /bin/ovh_exporter

FROM alpine

COPY --from=build /bin/ovh_exporter /bin/ovh_exporter

EXPOSE 9162

ENTRYPOINT ["/bin/ovh_exporter"]