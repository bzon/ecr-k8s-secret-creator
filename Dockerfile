FROM alpine:3.6

RUN apk add --no-cache ca-certificates tini

COPY ./ecr-k8s-secret-creator /ecr-k8s-secret-creator

RUN chmod +x /ecr-k8s-secret-creator

ENTRYPOINT ["/sbin/tini", "--", "/ecr-k8s-secret-creator"]

