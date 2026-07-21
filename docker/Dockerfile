# harness-runner: sandboxed environment for ALL project-touching agent runs.
# Rebuild after changing: docker build -t harness-runner:latest docker/
FROM node:20-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
      git jq ripgrep ca-certificates curl coreutils \
    && rm -rf /var/lib/apt/lists/*

# EDIT_ME: your project's toolchain (default: Go)
RUN curl -fsSL https://go.dev/dl/go1.23.4.linux-amd64.tar.gz | tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:${PATH}"

RUN npm install -g @anthropic-ai/claude-code

# Non-root: the agent inside has no root even in its sandbox
RUN useradd -m runner
USER runner
WORKDIR /work

# git identity for agent commits inside throwaway clones
RUN git config --global user.email "harness-runner@local" \
 && git config --global user.name  "harness-runner" \
 && git config --global --add safe.directory /work
