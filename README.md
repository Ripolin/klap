# 👏 klap
![WIP](https://img.shields.io/badge/status-WIP-orange)

Status: Work in progress — This project is under active development.

## Description
`klap` is a Kubernetes operator that declaratively manages LDAP directory entries using a custom resource (`Entry`). It synchronizes the desired state defined in Kubernetes with a remote LDAP server: creating and updating entries, optionally pruning them on deletion, and recording the remote `GUID` in status. The operator supports TLS (including custom CA bundles), StartTLS, and enforces DN validation and sensible defaults via webhooks.

## Getting Started

### Prerequisites
- Go version v1.25.5+ (see `go.mod`)
- Docker (or another container runtime) installed for image builds
- `kubectl` compatible with your cluster (recommended v1.25+)
- Access to a Kubernetes cluster with support for CRDs and webhooks (v1.25+ recommended)

### To Deploy on the cluster

This document explains how to install and use the `klap` Kubernetes operator to manage LDAP entries using the `Entry` custom resource.

**Quick links**
- CRD sample: [config/samples/klap_v1alpha1_entry.yaml](config/samples/klap_v1alpha1_entry.yaml)
- API: [api/v1alpha1/entry_types.go](api/v1alpha1/entry_types.go)

### Overview

The `klap` operator synchronizes LDAP entries from Kubernetes `Entry` custom resources to a remote LDAP server. It can create, update and (optionally) delete entries on the LDAP server.

Key behavior:
- On create/update the operator will add or modify the remote LDAP entry to match the `Entry` spec.
- On delete the operator will remove the remote entry only if `spec.prune` is true.
- A validating + mutating webhook provides defaults and validates the `Entry` resource (DN validation + finalizer and secret namespace defaults).

### Requirements

- Kubernetes cluster (the project was scaffolded with kubebuilder; use a modern cluster for webhooks — v1.25+ recommended).
- `kubectl`, `make`, and a container runtime (Docker or alternative) for building and pushing images.

### Installation

Build and push the operator image (set `IMG`):

```sh
make docker-build docker-push IMG=<registry>/klap:tag
```

Install the CRDs:

```sh
make install
```

Deploy the manager (controller + webhooks) using your published image:

```sh
make deploy IMG=<registry>/klap:tag
```

To uninstall:

```sh
make undeploy
make uninstall
```

You can also use the generated installer bundle in `dist/install.yaml`.

### About the `Entry` CRD

Resource: `apiVersion: klap.ripolin.github.com/v1alpha1`, `kind: Entry`.

Purpose: declare a single LDAP directory entry to be managed by the operator.

Top-level fields (summary):

- `spec.dn` (string, required): the LDAP Distinguished Name for the entry. The webhook validates DN syntax.
- `spec.prune` (bool, default: true): when true the operator will delete the remote LDAP entry when the `Entry` resource is deleted.
- `spec.force` (bool, default: false): when true the operator will force modifications to the remote entry even if they may lead to data loss (for example overriding or removing attributes). Use with caution.
- `spec.attributes` (map[string][]string, optional): attributes reconciled on each update. Keys are LDAP attribute names; values are lists of strings.
- `spec.serverSecretRef` (SecretRef, required): reference to a `Secret` containing LDAP server connection details (see below). The webhook defaults the `namespace` to the Entry's namespace when omitted.
- `spec.tlsSecretRef` (SecretRef, optional): reference to a `Secret` containing TLS CA material (`ca.crt`). Like `spec.serverSecretRef`, the webhook defaults the `namespace` to the Entry's namespace when omitted.


`SecretRef` structure:

- `name` (string, required)
- `namespace` (string, optional)

Status and metadata:

- `status.guid` (string): the remote LDAP Global Unique IDentifier recorded after successful creation/search. This value is sourced from the LDAP server and may be called `entryUUID` (OpenLDAP) or `objectGUID` (Active Directory).
- `status.implementation` (string): the detected LDAP implementation where the entry was found. Possible values: `openldap` or `activedirectory`.
- `status.conditions`: standard Kubernetes conditions are used (e.g., `Available` = True/False) to indicate synchronization state.
- A finalizer `klap.ripolin.github.com/finalizer` is added by the defaulter to ensure prune behavior is executed before Kubernetes removes the resource.

Example `Entry` resource (concise):

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
        description:
            - "Imported by klap"
    serverSecretRef:
        name: ldap-server
        namespace: default
    tlsSecretRef:
        name: ldap-tls
        namespace: default
```

Controller behavior summary:

- On create: the operator attempts to add the entry to the remote LDAP server. If the entry already exists the controller will search the directory and record the remote identifier in `status.guid` and set `status.implementation` to either `openldap` or `activedirectory` depending on which attribute was present.
- On update: if `status.guid` exists the operator will locate the remote entry by that identifier (using the attribute appropriate to `status.implementation`) and reconcile attributes from the `spec`.

Important: Do not set operational attributes (for example: createTimestamp, modifyTimestamp, entryUUID) in `attributes`. These attributes are managed by the LDAP server and may be rejected, ignored or overwritten. Use non-operational attributes (for example `description`, `location`, `title`) for initial metadata.

#### Server secret expected keys

`spec.serverSecretRef` must reference a `v1` Secret containing the following keys in `data` or `stringData` (all values are strings):

- `url` — LDAP URL, e.g. `ldap://ldap.example.svc:389` or `ldaps://ldap.example.svc:636`.
- `bind_dn` — Bind DN used to authenticate to the LDAP server.
- `password` — Password for the `bind_dn`.
- `base_dn` — Base DN to use when searching for entries.
- `start_tls` — Optional boolean as string (`"true"` or `"false"`). If present and the server URL scheme is `ldap`, `start_tls` will trigger StartTLS.

Example server secret (stringData form for readability):

```yaml
apiVersion: v1
kind: Secret
metadata:
    name: ldap-server
    namespace: default
type: Opaque
stringData:
    url: ldap://ldap.openldap.svc:389
    bind_dn: cn=admin,dc=example,dc=org
    password: supersecret
    base_dn: dc=example,dc=org
    start_tls: "false"
```

#### TLS secret expected keys

If `spec.tlsSecretRef` is provided, the referenced secret should include a PEM-encoded CA certificate under the key `ca.crt` (the controller will append those certs to the TLS root pool).

Example TLS secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
    name: ldap-tls
    namespace: default
type: Opaque
data:
    ca.crt: |+ # base64-encoded PEM content (example placeholder)
        LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCg==
```

### Example `Entry` resource

See the sample under [config/samples/klap_v1alpha1_entry.yaml](config/samples/klap_v1alpha1_entry.yaml).

A minimal example with server/tls references:

```yaml
apiVersion: klap.ripolin.github.com/v1alpha1
kind: Entry
metadata:
    name: example-entry
    namespace: default
spec:
    dn: cn=joe,dc=example,dc=org
    prune: true
    force: false
    attributes:
        objectClass:
            - inetOrgPerson
        sn:
            - Doe
    serverSecretRef:
        name: ldap-server
        namespace: default
    tlsSecretRef:
        name: ldap-tls
        namespace: default
```

Behavior notes:
- If an `Entry` is created and the remote entry does not exist, the operator will create it and populate `status.guid` with the remote identifier and set `status.implementation` to the detected server type.
- If `status.guid` is set the operator will attempt to update the corresponding LDAP entry (matching by the implementation-specific attribute: `entryUUID` for OpenLDAP, `objectGUID` for Active Directory).
- If the `Entry` is deleted and `spec.prune` is true, the operator will delete the remote LDAP entry.

### Webhooks and defaults

The operator registers both a mutating (defaulter) and validating webhook for `Entry`:
- The defaulter adds a finalizer (`klap.ripolin.github.com/finalizer`) on new `Entry` objects and will default `serverSecretRef.namespace` and `tlsSecretRef.namespace` to the resource namespace when omitted.
- The validator checks that `spec.dn` is a syntactically valid LDAP DN.

Note: webhooks require the manager to be deployed with webhook configuration (this is handled by `make deploy` which applies the `config/webhook` kustomize manifests).

### RBAC

The controller requires permission to read secrets and manage `entries` resources. The repository includes RBAC manifests under `config/rbac/` that will be applied by the installer.

### Observability & troubleshooting

- Check `Entry` status and events:
### Development / Testing

Run the sample locally (kind/minikube) with the default make targets:

```sh
# install CRDs
make install
# build and deploy controller (use local registry or update IMG)
make docker-build docker-push IMG=<registry>/klap:dev
make deploy IMG=<registry>/klap:dev
# apply example
kubectl apply -k config/samples/
```

### Next steps / Recommendations

- Add a documented example `Secret` in `config/samples/` that contains realistic keys and values (redact sensitive values).
- Consider publishing documentation as a site (mkdocs or GitHub Pages).

---
Generated from source: [PROJECT](PROJECT), [api/v1alpha1/entry_types.go](api/v1alpha1/entry_types.go), [internal/controller/entry_controller.go](internal/controller/entry_controller.go), and webhook defaults in [internal/webhook/v1alpha1/entry_webhook.go](internal/webhook/v1alpha1/entry_webhook.go).

