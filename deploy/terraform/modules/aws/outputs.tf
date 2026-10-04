output "vpc_id" {
  description = "VPC id."
  value       = aws_vpc.this.id
}

output "private_subnet_ids" {
  description = "Private subnet ids (EKS nodes + RDS)."
  value       = aws_subnet.private[*].id
}

output "public_subnet_ids" {
  description = "Public subnet ids (NAT + load balancers)."
  value       = aws_subnet.public[*].id
}

output "eks_cluster_name" {
  description = "EKS cluster name."
  value       = aws_eks_cluster.this.name
}

output "eks_cluster_endpoint" {
  description = "EKS API server endpoint."
  value       = aws_eks_cluster.this.endpoint
}

output "eks_cluster_ca" {
  description = "Base64 cluster CA (for a kubeconfig)."
  value       = aws_eks_cluster.this.certificate_authority[0].data
}

output "rds_endpoint" {
  description = "RDS Postgres endpoint (host:port)."
  value       = aws_db_instance.postgres.endpoint
}

output "rds_address" {
  description = "RDS Postgres hostname."
  value       = aws_db_instance.postgres.address
}
