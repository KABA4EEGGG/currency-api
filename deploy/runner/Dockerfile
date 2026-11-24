# Образ self-hosted GitHub Actions runner для запуска в k3s (деплой, smoke,
# генератор нагрузки k6). Собирается пайплайном .github/workflows/runner-image.yml
# и публикуется в GHCR как ghcr.io/kaba4eeggg/actions-runner.
FROM ubuntu:24.04

ENV RUNNER_VERSION=2.338.2 \
    HELM_VERSION=4.3.2 \
    KUBECTL_VERSION=1.36.5 \
    K6_VERSION=1.4.1

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl jq git \
 && rm -rf /var/lib/apt/lists/*

# helm (деплой релиза в кластер)
RUN curl -fsSL "https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" | tar -xz -C /tmp \
 && mv /tmp/linux-amd64/helm /usr/local/bin/helm \
 && helm version

# kubectl (версия под кластер)
RUN curl -fsSL -o /usr/local/bin/kubectl \
      "https://dl.k8s.io/release/v${KUBECTL_VERSION}.bin/linux/amd64/kubectl" \
 && chmod +x /usr/local/bin/kubectl

# k6 (генератор нагрузочных тестов)
RUN curl -fsSL \
      "https://github.com/grafana/k6/releases/download/v${K6_VERSION}/k6-v${K6_VERSION}-linux-amd64.tar.gz" \
    | tar -xz -C /tmp --strip-components=1 "k6-v${K6_VERSION}-linux-amd64/k6" \
 && mv /tmp/k6 /usr/local/bin/k6 \
 && k6 version

# GitHub Actions runner
RUN useradd -m -u 1000 runner \
 && curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" \
    | tar -xz -C /home/runner \
 && mv /home/runner/actions-runner-linux-x64-${RUNNER_VERSION} /home/runner/actions-runner \
 && /home/runner/actions-runner/bin/installdependencies.sh \
 && rm -rf /home/runner/actions-runner/bin/*.tar.gz \
 && chown -R runner:runner /home/runner

COPY --chown=runner:runner start-runner.sh /home/runner/actions-runner/start-runner.sh

USER runner
WORKDIR /home/runner/actions-runner
ENTRYPOINT ["/home/runner/actions-runner/start-runner.sh"]
