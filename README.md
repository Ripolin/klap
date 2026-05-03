# 👏 klap

[![Status](https://img.shields.io/badge/status-WIP-orange)](https://github.com/Ripolin/klap)
[![Go](https://img.shields.io/badge/Go-1.25-blue?logo=go)](go.mod)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

**klap** is a Kubernetes operator that declaratively manages LDAP directory entries via custom resources.

## Overview

`klap` reconciles Kubernetes custom resources with remote LDAP directories. Define your directory entries as Kubernetes objects and let the operator handle creates, updates and deletes.

Two CRDs are provided:

| CRD      | Purpose                                            |
|----------|----------------------------------------------------|
| `Server` | Centralizes LDAP connection and TLS configuration  |
| `Entry`  | Declares a single LDAP directory entry to manage   |

**Key features:**
- Declarative LDAP entry lifecycle (create, update, prune on delete)
- TLS and StartTLS support with custom CA bundles
- DN validation via admission webhooks
- Supports OpenLDAP and Active Directory *(beta)*
- Remote entry UUID/GUID tracked in resource status
- Helm chart available as OCI artifact

## Installation

### Prerequisites

- Kubernetes cluster with admission webhook support
- [cert-manager](https://cert-manager.io/) (required for webhook certificates)
- Helm 3

### Via Helm (recommended)

```sh
helm install klap oci://ghcr.io/ripolin/helm/klap --version <version> \
  --namespace klap-system \
  --create-namespace
```

### Via Kustomize

```sh
kubectl apply -k config/default
```

## Quick start

### 1. Create a Server

```yaml
apiVersion: klap.ripolin.github.com/v1alpha1
kind: Server
metadata:
  name: ldap-server
  namespace: default
spec:
  url: ldap://ldap.openldap.svc.cluster.local
  baseDN: dc=example,dc=org
  bindDN: cn=admin,dc=example,dc=org
  passwordSecretRef:
    name: ldap-passwd
    namespace: default
  implementation: openldap  # or activedirectory (beta)
  startTLS: false
```

### 2. Create the bind password Secret

```sh
kubectl create secret generic ldap-passwd \
  --from-literal=password=<your-password>
```

### 3. Declare an Entry

```yaml
apiVersion: klap.ripolin.github.com/v1alpha1
kind: Entry
metadata:
  name: joe
  namespace: default
spec:
  dn: cn=joe,dc=example,dc=org
  prune: true       # delete LDAP entry when this resource is deleted
  force: false      # allow destructive attribute changes
  attributes:
    objectClass:
      - inetOrgPerson
    sn:
      - Doe
    mail:
      - joe@example.org
  serverRef:
    name: ldap-server
    namespace: default
```

Once applied, `klap` creates or updates the corresponding entry in the LDAP directory. The remote entry UUID is stored in `status.guid`.

## CRD Reference

### Server

| Field                    | Type        | Default    | Description                                    |
|--------------------------|-------------|------------|------------------------------------------------|
| `spec.url`               | string      | —          | LDAP URL (`ldap://` or `ldaps://`)             |
| `spec.baseDN`            | string      | —          | Base DN for searches                           |
| `spec.bindDN`            | string      | —          | Bind DN for authentication                     |
| `spec.passwordSecretRef` | ResourceRef | —          | Secret containing the `password` key           |
| `spec.implementation`    | enum        | `openldap` | `openldap` or `activedirectory` *(beta)*       |
| `spec.tlsSecretRef`      | ResourceRef | —          | Secret with `ca.crt` for custom CA trust       |
| `spec.startTLS`          | bool        | `false`    | Enable StartTLS on plain `ldap://` connections |

### Entry

| Field             | Type                | Default | Description                                    |
|-------------------|---------------------|---------|------------------------------------------------|
| `spec.dn`         | string              | —       | Distinguished name (validated by webhook)      |
| `spec.prune`      | bool                | `true`  | Delete the LDAP entry when resource is deleted |
| `spec.force`      | bool                | `false` | Allow destructive attribute modifications      |
| `spec.attributes` | map[string][]string | —       | LDAP attributes to reconcile                   |
| `spec.serverRef`  | ResourceRef         | —       | Reference to a `Server` resource               |

#### Secret key override

By default `passwordSecretRef` reads the `password` key and `tlsSecretRef` reads `ca.crt`. Both can be overridden:

```yaml
spec:
  tlsSecretRef:
    name: ldap-tls
    namespace: default
    key: myBundle
```

## Development

### Prerequisites

- Go (see `go.mod`)
- Docker
- `kubectl` and a local cluster (e.g. [kind](https://kind.sigs.k8s.io/))

### Common targets

```sh
make docker-build IMG=<registry>/klap:dev  # build the controller image
make install                               # install CRDs
make deploy IMG=<registry>/klap:dev        # deploy via Kustomize
make helm-deploy IMG=<registry>/klap:dev   # deploy via Helm
make test                                  # run unit tests
make test-e2e                              # run e2e tests (requires a cluster)
make undeploy && make uninstall            # cleanup
```

### Samples

Ready-to-use manifests are available in `config/samples/`.

## Contributing

Issues and pull requests are welcome. Please follow the existing commit convention (used to generate the changelog via [git-cliff](https://git-cliff.org/)).

## License
