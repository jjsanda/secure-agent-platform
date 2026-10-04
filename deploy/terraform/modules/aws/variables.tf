variable "name" {
  description = "Name prefix for all resources."
  type        = string
  default     = "secure-agent-platform"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.42.0.0/16"
}

variable "az_count" {
  description = "Number of availability zones (public + private subnet per AZ)."
  type        = number
  default     = 2
}

variable "cluster_version" {
  description = "EKS Kubernetes version."
  type        = string
  default     = "1.31"
}

variable "node_instance_types" {
  description = "Instance types for the managed node group."
  type        = list(string)
  default     = ["t3.large"]
}

variable "node_scaling" {
  description = "Managed node group scaling."
  type = object({
    desired = number
    min     = number
    max     = number
  })
  default = {
    desired = 2
    min     = 2
    max     = 5
  }
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t3.medium"
}

variable "db_allocated_storage" {
  description = "RDS allocated storage (GiB)."
  type        = number
  default     = 20
}

variable "db_engine_version" {
  description = "Postgres engine version."
  type        = string
  default     = "16.4"
}

variable "db_name" {
  description = "Initial database name."
  type        = string
  default     = "sap"
}

variable "db_username" {
  description = "Master username (bootstrap; the app still uses the sap_app role)."
  type        = string
  default     = "postgres"
}

variable "db_password" {
  description = "Master password. Pass via TF_VAR_db_password / a secrets manager — never commit."
  type        = string
  sensitive   = true
  default     = "" # intentionally empty; supply at apply time
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default = {
    "app.kubernetes.io/part-of" = "secure-agent-platform"
    "terraform"                 = "true"
  }
}
