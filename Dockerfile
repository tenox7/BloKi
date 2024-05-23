FROM scratch
ARG TARGETARCH
ADD bloki-${TARGETARCH}-linux /bloki
ENTRYPOINT ["/bloki"]
LABEL maintainer="as@tenoware.com"
