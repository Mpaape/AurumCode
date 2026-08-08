# AUR-001 Claim Manifest

`claims.yaml` is a deliberately narrow, machine-readable claim list. It is
not a general YAML document and it is not an authorization source. A semantic
`implemented` claim names one tracked entrypoint and one tracked executable
test. A `disposition` record names an observed surface and its disposition but
does not assert that the legacy capability was tested. An `absent` record names
no implementation and carries a reason explaining why the checkout provides
no proof.

The acceptance and integration readers require `claim`, `status`,
`entrypoint`, and `test` for implemented claims; `claim`, `status`,
`disposition`, and `reason` for disposition records; and `claim`, `status`,
and `reason` for absent claims. They resolve paths against the independently
checked inventory and never execute a legacy entrypoint.
