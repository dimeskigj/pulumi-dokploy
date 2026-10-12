#!/usr/bin/env bash
# Run independent tiers serially; ordinary failures are aggregated, not gates.
set -euo pipefail

selection="${1:-all}"
case "$selection" in
  all|tier1|tier2|tier3|tier4|smoke) ;;
  *) echo "Unknown acceptance segment; choose all, tier1, tier2, tier3, tier4, or smoke." >&2; exit 1 ;;
esac
if [[ "${DOKPLOY_ACCEPTANCE:-}" != 1 || -z "${DOKPLOY_ENDPOINT:-}" || -z "${DOKPLOY_API_KEY:-}" ]]; then
  echo "Live acceptance requires explicit opt-in and protected credentials." >&2
  exit 1
fi
marker="${DOKPLOY_ACCEPTANCE_STOP_FILE:-}"
if [[ -z "$marker" || ! -d "$(dirname "$marker")" || ! -w "$(dirname "$marker")" ]]; then
  echo "Live acceptance requires a writable stop-marker location." >&2
  exit 1
fi

failed=0
for segment in tier1 tier2 tier3 tier4 smoke; do
  if [[ "$selection" != all && "$selection" != "$segment" ]]; then
    continue
  fi
  # Never clear a previous safety stop, including a dangling marker symlink.
  if [[ -e "$marker" || -L "$marker" ]]; then
    echo "Acceptance safety stop present; investigate cleanup or server health before continuing." >&2
    exit 1
  fi
  package=./provider
  case "$segment" in
    tier1) test_name=TestLiveTier1ControlPlane ;;
    tier2) test_name=TestLiveTier2Workloads ;;
    tier3) test_name=TestLiveTier3Databases ;;
    tier4) test_name=TestLiveTier4Backups ;;
    smoke) test_name=TestAccLifecycleSmoke; package=./tests ;;
  esac
  echo "Running acceptance segment: $segment"
  if go test "$package" -run "^${test_name}$" -parallel=1 -count=1 -timeout=90m -v; then
    echo "Acceptance segment passed: $segment"
  else
    failed=1
    echo "Acceptance segment failed: $segment" >&2
  fi
  if [[ -e "$marker" || -L "$marker" ]]; then
    echo "Acceptance safety stop present; later segments will not run." >&2
    exit 1
  fi
done
exit "$failed"
