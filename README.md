# 👏 klap
![WIP](https://img.shields.io/badge/status-WIP-orange)

Kubernetes operator to declaratively manage LDAP directory entries.

Status: Work in progress

## Overview
`klap` synchronizes Kubernetes custom resources with remote LDAP directories. It provides two primary CRDs:

- `Entry` — declares a single LDAP directory entry to be created/updated/optionally pruned.
- `Server` — centralizes LDAP server connection and TLS configuration so many `Entry` resources can reference one server.

The operator supports TLS (including custom CA bundles), StartTLS, DN validation via webhooks, and records remote identifiers in resource status.

## Quick start

Prerequisites:
- Go (see `go.mod`)
- Docker (or other container runtime)
- `kubectl` and a Kubernetes cluster (webhooks require a cluster that supports admission webhooks)

Install CRDs and deploy (example using Makefile targets):

```sh
make install
make docker-build docker-push IMG=<registry>/klap:tag
make deploy IMG=<registry>/klap:tag
```

Uninstall / cleanup:

```sh
make undeploy
make uninstall
```

## CRDs

### Entry
Resource: `apiVersion: klap.ripolin.github.com/v1alpha1`, `kind: Entry`.

Purpose: declare an LDAP entry to manage.

Key fields:
- `spec.dn` (string) — distinguished name for the entry (validated by webhook).
- `spec.prune` (bool, default: true) — delete remote entry when resource is deleted.
- `spec.force` (bool, default: false) — allow forcing destructive changes.
- `spec.attributes` (map[string][]string) — attributes to reconcile.
- `spec.serverRef` (ResourceRef) — reference to a `Server` resource.

Status highlights:
- `status.guid` — remote entry unique identifier (e.g., `entryUUID` or `objectGUID`).

Example `Entry` (concise):

```yaml
apiVersion: klap.ripolin.github.com/v1alpha1
kind: Entry
metadata:
  name: example-entry
  namespace: default
spec:
  dn: cn=joe,dc=example,dc=org
  prune: true
  attributes:
    objectClass:
      - inetOrgPerson
    sn:
      - Doe
  serverRef:
    name: ldap-server
    namespace: default
```

### Server
Resource: `apiVersion: klap.ripolin.github.com/v1alpha1`, `kind: Server`.

Purpose: store LDAP connection configuration and TLS references for reuse by `Entry` resources.

Key fields:
- `spec.url` (string) — LDAP URL, e.g. `ldap://ldap.svc:389` or `ldaps://ldap.svc:636`.
- `spec.baseDN` (string) — base DN for searches.
- `spec.bindDN` (string) — bind DN used to authenticate.
- `spec.passwordSecretRef` (ResourceRef) — Secret reference containing the bind password.
- `spec.implementation` (enum: `openldap`|`activedirectory`, default: `openldap`).
- `spec.tlsSecretRef` (ResourceRef, optional) — Secret reference with `ca.crt` to trust custom CAs.
- `spec.startTLS` (bool, default: false) — enable StartTLS for plain `ldap://` URLs.

Example `Server`:

```yaml
apiVersion: klap.ripolin.github.com/v1alpha1
kind: Server
metadata:
  name: ldap-server
  namespace: default
spec:
  url: ldap://ldap.openldap.svc:389
  baseDN: dc=example,dc=org
  bindDN: cn=admin,dc=example,dc=org
  passwordSecretRef:
    name: ldap-server-password
    namespace: default
  implementation: openldap
  startTLS: false
  tlsSecretRef:
    name: ldap-tls
    namespace: default
```

## Secrets

By default, TLS Secret (when using `tlsSecretRef`) should include `ca.crt` key containing PEM-encoded CA certificates and server password Secret should include `password` key.

If necessary, secret key could be changed to pinpoint another exiting key.

Example:

```yaml
apiVersion: klap.ripolin.github.com/v1alpha1
kind: Server
metadata:
  name: ldap-server
  namespace: default
spec:
  ...
  tlsSecretRef:
    name: ldap-tls
    namespace: default
    key: myBundle
```

## Samples

See `config/samples/` for example `Entry` manifests. Consider adding a `Server` sample there to simplify getting started.

## Development

Run controller locally with webhooks (recommended via `make` targets). Tests and webhooks are under `internal/`.

## Contributing

Issues and pull requests welcome. Follow project conventions in `PROJECT` and existing module structure.

---
Generated from source: PROJECT and API types under `api/v1alpha1`.
