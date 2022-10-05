FROM golang:1 as build-env

WORKDIR /go/src/app
COPY  . ./

RUN CGO_ENABLED=0 go build

FROM gcr.io/distroless/static

COPY --from=build-env /go/src/app/rabbitmq-operator-karmada-resource-interpreter /bin/rabbitmq-operator-karmada-resource-interpreter

ENTRYPOINT [ "/bin/rabbitmq-operator-karmada-resource-interpreter" ]
