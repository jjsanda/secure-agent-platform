# Local, genuinely-applyable stack: stand up a kind cluster and install the
# umbrella Helm chart onto it.
#
#   terraform init && terraform apply
#
# (A full apply is heavy — it pulls kindest/node + every service image — so it is
# intentionally NOT run in CI. init + validate + fmt are.)

resource "kind_cluster" "this" {
  name           = var.cluster_name
  node_image     = var.node_image
  wait_for_ready = true
}

resource "helm_release" "sap" {
  name             = var.release_name
  namespace        = var.namespace
  create_namespace = true

  # The chart lives two levels up in the deploy tree.
  chart = "${path.module}/../../helm/secure-agent-platform"

  # Don't block apply on every pod becoming ready (images pull on first run).
  wait    = false
  timeout = 600

  set {
    name  = "keycloak.enabled"
    value = tostring(var.keycloak_enabled)
  }
  set {
    name  = "observability.enabled"
    value = tostring(var.observability_enabled)
  }
  set {
    name  = "postgres.persistence.enabled"
    value = tostring(var.postgres_persistence)
  }

  depends_on = [kind_cluster.this]
}
