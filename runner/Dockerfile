# checkov:skip=CKV_DOCKER_3:runner executes user commands and requires root
# checkov:skip=CKV_DOCKER_2:health check is managed by compose/orchestrator
# The nix closure is pinned to x86_64-linux.
# A build for another platform would mix architectures in the final image.
FROM --platform=linux/amd64 golang:1.26.6-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS build
WORKDIR /app
COPY runner/go.mod runner/go.sum ./
RUN go mod download
COPY runner/*.go ./

# test stage
FROM build AS test
RUN apk add --no-cache bash~=5.3
SHELL ["/bin/ash", "-o", "pipefail", "-c"]
RUN CGO_ENABLED=0 go test -v -coverprofile=cover.out -timeout=30s ./... \
    && total=$(go tool cover -func=cover.out | grep ^total: | awk '{print $NF}') \
    && echo "Total coverage: ${total}" \
    && test "${total}" = "100.0%" || { echo "FAIL: coverage ${total} is below 100.0%"; exit 1; }

# race detection stage
FROM build AS test-race
RUN apk add --no-cache bash~=5.3 gcc musl-dev
RUN CGO_ENABLED=1 go test -race -v -count=1 ./...

# binary build
FROM build AS binary
RUN CGO_ENABLED=0 go build -o /server .

FROM --platform=linux/amd64 nixos/nix:2.35.2@sha256:7a007c766426c1877758ddc5cb87a965ac131fc78c582ce0083d922d51ae945c AS nix-runtime

ENV PATH="/usr/local/bin:${PATH}"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

RUN mkdir -p /etc/nix
COPY runner/nix/build.conf /etc/nix/nix.conf

COPY runner/nix/nixpkgs-pin.nix runner/nix/home-manager-pin.nix /tmp/nix/
COPY runner/app/flake.lock /tmp/app/flake.lock

# The base image exports NIX_PATH, which wins over nix.conf's nix-path.
# Every step below unsets it to fall through to the pins.
RUN nixpkgs_src="$(nix eval --raw --file /tmp/nix/nixpkgs-pin.nix)" && \
    hm_src="$(nix eval --raw --file /tmp/nix/home-manager-pin.nix)" && \
    printf 'nix-path = nixpkgs=%s home-manager=%s\n' "$nixpkgs_src" "$hm_src" > /etc/nix/nix-path.conf && \
    cat /etc/nix/nix-path.conf >> /etc/nix/nix.conf && \
    mkdir -p /nix/var/nix/gcroots && \
    nix-store --add-root /nix/var/nix/gcroots/home-manager-source --indirect -r "$hm_src"

RUN nix-env -e man-db

RUN unset NIX_PATH && nix-shell '<home-manager>' -A install

RUN command -v home-manager && \
    HM_REAL="$(readlink -f "$(command -v home-manager)")" && \
    mkdir -p /usr/local/bin && \
    printf '%s\n' '#!/bin/sh' "exec \"$HM_REAL\" \"\$@\"" > /usr/local/bin/home-manager && \
    chmod +x /usr/local/bin/home-manager

RUN nix-env -e bash-interactive

RUN mkdir -p /root/.config/home-manager
COPY runner/nix/home.nix /root/.config/home-manager/home.nix

RUN unset NIX_PATH && home-manager switch

RUN unset NIX_PATH && nix shell nixpkgs#gnused --command sh -c '\
    sed -i '\''s|TZ = "Asia/Tokyo";|TZ = "Pacific/Honolulu";|g'\'' /root/.config/home-manager/home.nix && \
    sed -i '\''s|LANG = "ja_JP.UTF-8";|LANG = "en_US.UTF-8";|g'\'' /root/.config/home-manager/home.nix && \
    sed -i '\''s|LC_ALL = "ja_JP.UTF-8";|LC_ALL = "en_US.UTF-8";|g'\'' /root/.config/home-manager/home.nix && \
    home-manager switch'

COPY runner/nix/prebuilt-tools.nix /tmp/nix/prebuilt-tools.nix
RUN nix build --no-link --print-out-paths --file /tmp/nix/prebuilt-tools.nix > /tmp/prebuilt-tools-paths.txt && \
    prebuilt_tools="$(cat /tmp/prebuilt-tools-paths.txt)" && \
    mkdir -p /nix/var/nix/gcroots/prebuilt-tools /usr/local/bin && \
    nix-store --add-root /nix/var/nix/gcroots/prebuilt-tools/runner-prebuilt-tools --indirect -r "$prebuilt_tools" && \
    for bin in ip awk; do \
      ln -sf "$prebuilt_tools/bin/$bin" "/usr/local/bin/$bin"; \
    done && \
    nixpkgs_src="$(nix eval --raw --file /tmp/nix/nixpkgs-pin.nix)" && \
    nix-store --add-root /nix/var/nix/gcroots/nixpkgs-source --indirect -r "$nixpkgs_src" && \
    printf '{"version":2,"flakes":[{"from":{"type":"indirect","id":"nixpkgs"},"to":{"type":"path","path":"%s"}}]}\n' "$nixpkgs_src" > /etc/nix/registry.json

# Only the flake files.
# The Perl sources change often and would rebuild the closure on every edit.
COPY runner/app/flake.nix runner/app/flake.lock /app/
WORKDIR /app

COPY runner/nix/collect-flake-paths.nix /tmp/collect-flake-paths.nix
RUN nix flake archive --json > /tmp/app-flake-archive.json && \
    mkdir -p /nix/var/nix/gcroots/app-flake && \
    nix eval --raw --file /tmp/collect-flake-paths.nix > /tmp/app-flake-paths.txt && \
    path_index=0 && \
    while IFS= read -r flake_path || [ -n "$flake_path" ]; do \
      path_index=$((path_index + 1)); \
      nix-store --add-root "/nix/var/nix/gcroots/app-flake/path-${path_index}" --indirect -r "$flake_path"; \
    done < /tmp/app-flake-paths.txt && \
    nix develop --profile /nix/var/nix/profiles/app-devshell --command true && \
    mkdir -p /nix/var/nix/gcroots/app-devshell-shell && \
    nix build --no-link --print-out-paths --impure --expr '(builtins.getFlake (toString /app)).inputs.nixpkgs.legacyPackages.x86_64-linux.bashInteractive' > /tmp/app-devshell-shell-paths.txt && \
    shell_index=0 && \
    while IFS= read -r shell_path || [ -n "$shell_path" ]; do \
      shell_index=$((shell_index + 1)); \
      nix-store --add-root "/nix/var/nix/gcroots/app-devshell-shell/path-${shell_index}" --indirect -r "$shell_path"; \
    done < /tmp/app-devshell-shell-paths.txt

# The news file is realised apart from the activation package.
# Rooting only the generation leaves it to rebuild from source.
# That rebuild fails offline.
RUN unset NIX_PATH && HM_OUT="$(nix-build '<home-manager>' -A home-manager --no-out-link)" && \
    HM_REAL="$HM_OUT/bin/home-manager" && \
    NIX_REAL="$(readlink -f "$(command -v nix)")" && \
    BASH_REAL="$(readlink -f /root/.nix-profile/bin/bash)" && \
    LOCALE_ARCHIVE_PATH="$(awk -F'"' '/^export LOCALE_ARCHIVE="/ { print $2; exit }' /root/.nix-profile/etc/profile.d/hm-session-vars.sh)" && \
    test -n "$LOCALE_ARCHIVE_PATH" && \
    LOCALE_STORE="${LOCALE_ARCHIVE_PATH#/nix/store/}" && \
    LOCALE_STORE="/nix/store/${LOCALE_STORE%%/*}" && \
    NEWS_OUT="$(nix-build '<home-manager/home-manager/home-manager.nix>' --no-out-link \
      --arg check false -A config.news.json.output \
      --argstr confPath /root/.config/home-manager/home.nix --argstr confAttr '')" && \
    printf '%s\n' '#!/bin/sh' "exec \"$HM_REAL\" \"\$@\"" > /usr/local/bin/home-manager && \
    chmod +x /usr/local/bin/home-manager && \
    ln -sf "$BASH_REAL" /usr/local/bin/bash && \
    mkdir -p /usr/local/share && \
    ln -sf "$LOCALE_ARCHIVE_PATH" /usr/local/share/locale-archive && \
    mkdir -p /nix/var/nix/gcroots/runtime && \
    nix-store --add-root /nix/var/nix/gcroots/runtime/home-manager --indirect -r "$HM_OUT" && \
    nix-store --add-root /nix/var/nix/gcroots/runtime/nix --indirect -r "$NIX_REAL" && \
    nix-store --add-root /nix/var/nix/gcroots/runtime/bash --indirect -r "$BASH_REAL" && \
    nix-store --add-root /nix/var/nix/gcroots/runtime/glibc-locales --indirect -r "$LOCALE_STORE" && \
    nix-store --add-root /nix/var/nix/gcroots/runtime/hm-news --indirect -r "$NEWS_OUT" && \
    rm -rf /root/.cache /tmp/* && \
    rm -f /root/.nix-defexpr/channels \
      /nix/var/nix/profiles/per-user/root/channels \
      /nix/var/nix/profiles/per-user/root/channels-*-link \
      /nix/var/nix/gcroots/docker/* && \
    nix-env --delete-generations old && \
    nix-collect-garbage
# Not `-d`: it drops the home-manager generations that `--rollback` needs.

# production image
FROM --platform=linux/amd64 alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

ENV PATH="/usr/local/bin:/root/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin" \
    USER="root" \
    LOCALE_ARCHIVE="/usr/local/share/locale-archive" \
    NIX_SSL_CERT_FILE="/etc/ssl/certs/ca-certificates.crt" \
    SSL_CERT_FILE="/etc/ssl/certs/ca-certificates.crt"

# Pin the exact versions instead of a floating `apk upgrade` so the image stays
# reproducible; a version dropped from the repo fails the build loudly and
# forces a base image bump rather than silently changing what ships.
# bash is absent here because it comes from the nix profile.
RUN apk add --no-cache libcrypto3=3.5.8-r0 libssl3=3.5.8-r0 perl=5.42.2-r0 perl-html-parser=3.85-r0 perl-module-refresh=0.18-r2

RUN addgroup -S nixbld && \
    mkdir -p /etc/nix && \
    adduser -S -D -H -h /var/empty -s /sbin/nologin -G nixbld nixbld1

COPY runner/nix/runtime.conf /etc/nix/nix.conf
COPY --from=nix-runtime /etc/nix/nix-path.conf /etc/nix/nix-path.conf
COPY --from=nix-runtime /etc/nix/registry.json /etc/nix/registry.json
COPY --from=nix-runtime /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=nix-runtime /nix /nix
COPY --from=nix-runtime /root /root
COPY --from=nix-runtime /usr/local/bin /usr/local/bin
COPY --from=nix-runtime /usr/local/share /usr/local/share

COPY runner/app/ /app/
WORKDIR /app

COPY --from=binary /server /server
EXPOSE 3000 5000
CMD ["/server"]
