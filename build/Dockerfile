FROM registry.access.redhat.com/ubi7/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/chaosblade-operator \
    CHAOSBLADE_HOME=/opt/chaosblade

# install operator binary
COPY build/_output/bin/chaosblade-operator ${OPERATOR}
COPY build/cache/chaosblade ${CHAOSBLADE_HOME}
COPY build/bin /usr/local/bin

ENTRYPOINT ["/usr/local/bin/entrypoint"]
