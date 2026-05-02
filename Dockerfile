FROM scratch
ARG TARGETARCH
COPY linux/${TARGETARCH}/oauth-callback-dispatcher /oauth-callback-dispatcher
ENTRYPOINT ["/oauth-callback-dispatcher"]
