# kustomize

A minimal base + `dev` overlay. **Helm is the primary Kubernetes path** (it owns Keycloak, observability, HPAs, Ingress and NetworkPolicies); this is a lighter kustomize alternative.

```
base/                  five core services (Deployments + Services)
  kustomization.yaml   + configMapGenerator (initdb) + secretGenerator (db creds)
overlays/dev/          namespace, control-plane replicas=2, resource patch, :dev tags
```

```bash
kubectl kustomize deploy/k8s/base          # render the base
kubectl kustomize deploy/k8s/overlays/dev  # render the dev overlay
kubectl apply -k deploy/k8s/overlays/dev   # apply it
```

The `dev` overlay demonstrates the common transformers: `namespace`, `replicas`, a strategic-merge `patch`, the `images` tag override, and `labels`. Generated ConfigMap/Secret names carry content hashes and references are rewritten automatically.

Validated with `kubectl kustomize … | kubeconform -strict` (13/13 resources).
