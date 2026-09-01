# Project handoff: kplan

## Purpose

`kplan` is a native Go Kubernetes plan/apply CLI. It must use Kubernetes APIs
directly and must not invoke `kubectl`. The product idea is a readable planner
that can eventually save and apply reviewed plans safely.

## Current state

- Commands: `kplan plan -f FILE` and `kplan apply -f FILE`.
- Uses Cobra, `client-go` discovery, a dynamic client, and unstructured objects.
- Loads YAML or JSON, including multi-document YAML.
- Supports kubeconfig, context, namespace, field-manager, and
  `--force-conflicts` flags.
- Existing resources are predicted using server-side apply dry-run.
- Results are classified as create, update, unchanged, or conflict.
- Updates include normalized unified live-versus-planned YAML diffs.
- On an ownership conflict, a second **forced dry-run** is used only to render
  the predicted diff. The result remains `CONFLICT`; this does not mutate the
  cluster.
- Apply is interactive unless `--yes` is supplied. Namespace objects are
  ordered before namespaced resources.

The live development fixture is outside this project:

```bash
go run . plan \
  --kubeconfig ../playground/fedora-cloud-44.kubeconfig \
  -f ../playground/sample-httpd.yaml
```

The last verified live result showed a Deployment replica conflict owned by
`kubectl-client-side-apply`, with a unified diff from two to three replicas.
No apply was performed during that verification.

## Architecture

- `internal/cli`: command definitions, flags, confirmation, and rendering.
- `internal/manifest`: manifest decoding.
- `internal/kube`: kubeconfig, discovery mapper, and dynamic client setup.
- `internal/engine`: resource mapping, dry-run planning, normalization, diffs,
  summaries, and apply.

## Verification

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o kubectl-kplan .
```

Cluster-backed plan tests require network access to the Kubernetes API. Treat
`plan` as read-only. Never run `apply` against the real cluster unless the user
explicitly asks for it.

## Important limitations

- There is no saved plan artifact yet.
- `apply` replans, then reloads manifests before applying. That leaves an input
  time-of-check/time-of-use gap and should be fixed before claiming safe plans.
- Cluster state is not locked or revalidated between plan and apply.
- A missing resource is classified as create without server-side dry-run
  validation.
- No delete/prune behavior, Kustomize, Helm, directories, rollout waiting,
  Secret-specific redaction, or structured JSON output.
- Diff normalization deliberately removes status and common server-managed
  metadata, plus kubectl's last-applied annotation.

## Good next steps

1. Introduce a versioned saved-plan format containing normalized desired
   objects, input hashes, target cluster identity, and observed preconditions.
2. Apply the reviewed desired objects from that artifact rather than reloading
   source manifests.
3. Revalidate resource versions or other suitable preconditions before apply
   and reject stale plans.
4. Server-side dry-run creates and render their complete planned representation.
5. Add JSON output and Secret redaction tests.

Keep the native API design and explicit conflict ownership behavior intact.
