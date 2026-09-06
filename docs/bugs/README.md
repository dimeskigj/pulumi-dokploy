# Live acceptance bug reports

Use this guide for failures from the protected `run-acceptance-tests` workflow.
Reports must be sanitized: never paste API keys, registry credentials, `.env`
files, or raw environment dumps.

## Required report fields

- **Severity:** blocker, high, medium, or low; explain user impact.
- **Resource/revision:** resource or feature, provider revision, and workflow run.
- **Sanitized environment:** OS, Go/Pulumi versions, endpoint hostname only,
  and relevant feature flags. Redact tokens, passwords, IDs, and private URLs.
- **Reproduction:** exact safe command and the smallest input/configuration.
- **Expected / actual:** state the contract and observed result.
- **Safe evidence:** sanitized logs, error text, request IDs, and timestamps;
  remove credentials and sensitive payloads.
- **Likely code location:** package, resource, or harness function to inspect.
- **Workaround:** safe temporary mitigation, or `none known`.
- **Cleanup status:** whether created resources were removed; record any
  remaining resource IDs only in the protected incident system.

## Triage rules

1. Confirm the run used `DOKPLOY_ACCEPTANCE=1` and protected workflow secrets.
2. Record skip reasons separately from failures (for example, an intentionally
   unavailable replica feature is a skip, not a provider regression).
3. Preserve the first failure and cleanup outcome before retrying.
4. Link the sanitized report to the workflow run and update the run summary.
