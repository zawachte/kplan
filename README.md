# kplan

`kplan` is an experimental native Go planner for Kubernetes manifests. It uses
Kubernetes discovery, dynamic clients, and server-side apply dry-run. It never
executes `kubectl`.

## Build and run

```bash
go build ./...
go run . plan \
  --kubeconfig ../playground/fedora-cloud-44.kubeconfig \
  -f ../playground/sample-httpd.yaml
```

Apply after reviewing the plan:

```bash
go run . apply \
  --kubeconfig ../playground/fedora-cloud-44.kubeconfig \
  -f ../playground/sample-httpd.yaml
```

Use `--yes` for non-interactive apply.

## Initial scope

- YAML and JSON files, including multi-document YAML
- Kubeconfig context and namespace selection
- API discovery for built-in resources and CRDs
- Create/update/unchanged summaries
- Unified live-versus-planned diffs for updates and conflicts
- Server-side apply conflict detection (`--force-conflicts` is explicit)
- Server-side apply dry-run for existing resources
- Server-side apply with a dedicated `kplan` field manager

Not implemented yet: deletion/pruning, detailed field diffs, saved plans,
directories, Kustomize/Helm rendering, and rollout waiting.
