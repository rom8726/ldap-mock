FROM golang:1.23-alpine AS build

ENV GOPROXY="https://proxy.golang.org,direct"
ENV PROJECTDIR=/go/src/ldap-mock

WORKDIR ${PROJECTDIR}
COPY . ${PROJECTDIR}/

RUN go mod download
RUN go build -o ldap-mock-server

FROM alpine:3

COPY --from=build /go/src/ldap-mock/ldap-mock-server /bin/ldap-mock-server

EXPOSE 389
EXPOSE 6006

CMD ["/bin/ldap-mock-server"]
