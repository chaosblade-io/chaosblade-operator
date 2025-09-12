FROM --platform=linux/amd64 alpine:latest as builder

ENV OPERATOR=/usr/local/bin/chaosblade-operator
COPY build/_output/bin/chaosblade-operator /usr/local/bin/

# Build platform-specific scripts for AMD64
FROM --platform=linux/amd64 alpine:latest as script_builder
RUN apk add --no-cache bash
COPY build/bin /tmp/scripts
RUN chmod +x /tmp/scripts/*

FROM --platform=linux/amd64 registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/chaosblade-operator \
    CHAOSBLADE_HOME=/opt/chaosblade

COPY --from=builder ${OPERATOR} /usr/local/bin/
COPY --from=script_builder /tmp/scripts /usr/local/bin

RUN chmod 777 /usr/local/bin/user_setup
RUN /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]
