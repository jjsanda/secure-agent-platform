output "cluster_name" {
  description = "Name of the kind cluster."
  value       = kind_cluster.this.name
}

output "kubeconfig_path" {
  description = "Path to the generated kubeconfig for kubectl."
  value       = kind_cluster.this.kubeconfig_path
}

output "cluster_endpoint" {
  description = "Kubernetes API endpoint of the kind cluster."
  value       = kind_cluster.this.endpoint
}

output "next_steps" {
  description = "How to reach the stack once applied."
  value       = <<-EOT
    export KUBECONFIG=${kind_cluster.this.kubeconfig_path}
    kubectl -n ${var.namespace} get pods
    kubectl -n ${var.namespace} port-forward svc/${var.release_name}-secure-agent-platform-frontend 5173:80
  EOT
}
