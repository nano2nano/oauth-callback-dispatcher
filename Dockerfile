FROM scratch
ARG TARGETARCH
COPY linux/${TARGETARCH}/oauth-callback-dispatcher /oauth-callback-dispatcher
USER 65532:65532
ENTRYPOINT ["/oauth-callback-dispatcher"]
