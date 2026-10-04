variable "cluster_name" {
  description = "Name of the local kind cluster."
  type        = string
  default     = "sap-local"
}

variable "node_image" {
  description = "kind node image (pins the Kubernetes version)."
  type        = string
  default     = "kindest/node:v1.31.2"
}

variable "namespace" {
  description = "Namespace the chart is installed into."
  type        = string
  default     = "sap"
}

variable "release_name" {
  description = "Helm release name."
  type        = string
  default     = "sap"
}

variable "keycloak_enabled" {
  description = "Install Keycloak instead of the mock OIDC provider."
  type        = bool
  default     = false
}

variable "observability_enabled" {
  description = "Install the in-cluster OTel collector and wire the app to it."
  type        = bool
  default     = false
}

variable "postgres_persistence" {
  description = "Back Postgres with a PVC (false = ephemeral emptyDir, quicker to spin up)."
  type        = bool
  default     = false
}
