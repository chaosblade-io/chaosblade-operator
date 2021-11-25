FROM alpine:3.8 as builder

ENV OPERATOR=/usr/local/bin/chaosblade-operator
COPY build/_output/bin/chaosblade-operator /usr/local/bin/

FROM centos:latest

ENV OPERATOR=/usr/local/bin/chaosblade-operator \
    CHAOSBLADE_HOME=/opt/chaosblade

COPY --from=builder ${OPERATOR} /usr/local/bin/
COPY build/cache/chaosblade ${CHAOSBLADE_HOME}
COPY build/bin /usr/local/bin

RUN yum install net-tools -y
RUN /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]
