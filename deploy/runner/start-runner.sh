#!/bin/bash
# Регистрация и запуск self-hosted runner (pod в k3s).
# ACCESS_TOKEN — токен GitHub с правами repo (k8s Secret), REPO_URL — репозиторий.
set -euo pipefail

: "${ACCESS_TOKEN:?ACCESS_TOKEN обязателен (k8s Secret)}"
: "${REPO_URL:?REPO_URL обязателен}"
API_URL="https://api.github.com/repos/${REPO_URL#https://github.com/}"

echo "Получение registration token..."
reg_token=$(curl -sf -X POST \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Accept: application/vnd.github+json" \
  "${API_URL}/actions/runners/registration-token" | jq -r '.token')
[ -n "${reg_token}" ] && [ "${reg_token}" != "null" ] || { echo "Не удалось получить registration token"; exit 1; }

cleanup() {
  echo "Дерегистрация runner..."
  rm_token=$(curl -sf -X POST \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    "${API_URL}/actions/runners/remove-token" | jq -r '.token') || true
  if [ -n "${rm_token:-}" ] && [ "${rm_token}" != "null" ]; then
    ./config.sh remove --token "${rm_token}" || true
  fi
  [ -n "${child_pid:-}" ] && kill "${child_pid}" 2>/dev/null || true
}
trap cleanup TERM INT

./config.sh --url "${REPO_URL}" --token "${reg_token}" --unattended \
  --name "$(hostname)" --labels "${LABELS:-self-hosted}" \
  --work "${RUNNER_WORKDIR:-/home/runner/_work}"

./run.sh &
child_pid=$!
wait "${child_pid}"
