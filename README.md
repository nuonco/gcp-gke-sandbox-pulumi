# gcp-gke-sandbox-pulumi

A Pulumi (Go) port of the `gcp-gke-sandbox` Terraform module. It provisions a
GKE Standard cluster sandbox for the Nuon platform and produces the same outputs
as the Terraform module.

## What it provisions

- VPC + subnet (with `pods`/`services` secondary ranges) + Cloud Router/NAT,
  or looks up an existing network when `nuon:network` is set.
- Artifact Registry (Docker) repository.
- Optional Cloud DNS public + internal managed zones.
- GKE Standard cluster + a `main` node pool (Workload Identity, private nodes,
  Gateway API).
- A Kubernetes provider authenticated to the new cluster, the per-install
  namespaces, and (when `enable_linkerd`) cert-manager + Linkerd + the
  `all-egress` EgressNetwork.

## Config

All config is read from the `nuon` namespace. See `Pulumi.yaml` for the full
list of keys and defaults; they map 1:1 to the Terraform module variables.

## Usage

```bash
pulumi config set nuon:nuon_id <id>
pulumi config set nuon:project_id <project>
pulumi config set nuon:region us-central1
pulumi up
```
