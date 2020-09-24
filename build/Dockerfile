FROM alpine:3.8 as builder

ENV OPERATOR=/usr/local/bin/chaosblade-operator
COPY build/_output/bin/chaosblade-operator /usr/local/bin/

RUN wget https://github.com/upx/upx/releases/download/v3.96/upx-3.96-amd64_linux.tar.xz \
    && tar -xvf upx-3.96-amd64_linux.tar.xz \
    && upx-3.96-amd64_linux/upx ${OPERATOR} \
    && rm -rf upx*

FROM alpine:3.8

ENV OPERATOR=/usr/local/bin/chaosblade-operator \
    USER_UID=1001 \
    USER_NAME=chaosblade-operator

# install operator binary
COPY --from=builder ${OPERATOR} /usr/local/bin/
COPY build/bin /usr/local/bin

RUN /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
