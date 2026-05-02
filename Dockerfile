FROM scratch
COPY oauth-callback-dispatcher /oauth-callback-dispatcher
ENTRYPOINT ["/oauth-callback-dispatcher"]
