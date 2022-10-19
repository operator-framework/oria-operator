FROM gcr.io/distroless/base:debug-nonroot
WORKDIR /
COPY oria-operator oria-operator

ENTRYPOINT [ "/oria-operator" ]