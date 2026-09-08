#!/usr/bin/env bash
# Supply read-only CI credentials only to the exact requested repository.
set +x
set -eu
[[ "${1:-}" == get ]] || exit 0
credential_protocol=''
credential_host=''
credential_path=''
while IFS='=' read -r credential_key credential_value; do
  case "$credential_key" in
    protocol) credential_protocol="$credential_value" ;;
    host) credential_host="$credential_value" ;;
    path) credential_path="$credential_value" ;;
  esac
done
[[ "$credential_protocol" == https && "$credential_host" == github.com ]] || exit 0
credential_repository="${credential_path%.git}"
case "$credential_repository" in
  go-devtools/openapi|go-devtools/gin-swagger) ;;
  *) exit 0 ;;
esac
credential_token=''
if [[ "$credential_repository" == "${CI_REPOSITORY:-}" ]]; then
  credential_token="${CI_REPOSITORY_TOKEN:-}"
elif [[ "$credential_repository" == go-devtools/openapi ]]; then
  credential_token="${OPENAPI_READ_TOKEN:-}"
fi
[[ -n "$credential_token" ]] || exit 0
printf 'username=x-access-token\npassword=%s\n\n' "$credential_token"
