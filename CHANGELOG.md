# Changelog

## Unreleased

- Applications and Compose stacks now read `environment` (and, for applications,
  `buildArgs` and `buildSecrets`) back from Dokploy, so an imported resource matches
  the program that declares it. `pulumi refresh` therefore adopts server-side values,
  and `pulumi import --generate-code` writes those secret values into the generated
  program. A value Dokploy reports as empty is treated as "not reported", so clearing
  every variable in the Dokploy UI does not show up as drift.
- Added `deployOnUpdate` to Application and Compose. It defaults to `true`, which is
  the existing behaviour; set it to `false` to save configuration without redeploying,
  for stacks that deploy from their own pipeline. It is a required output, so state
  written by an earlier version gains the field on its next update.
- Initial Pulumi Dokploy provider MVP and Registry release setup.
