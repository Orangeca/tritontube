output "vpc_id" {
  description = "ID of the created VPC."
  value       = module.vpc.vpc_id
}

output "private_subnet_ids" {
  description = "IDs of the private subnets used by worker nodes."
  value       = module.vpc.private_subnets
}

output "public_subnet_ids" {
  description = "IDs of the public subnets."
  value       = module.vpc.public_subnets
}

output "eks_cluster_name" {
  description = "Name of the EKS cluster."
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "Endpoint URL of the EKS control plane."
  value       = module.eks.cluster_endpoint
}

output "eks_cluster_oidc_issuer" {
  description = "OIDC issuer URL for the EKS cluster (used for IRSA)."
  value       = module.eks.cluster_oidc_issuer_url
}

output "eks_node_group_role_name" {
  description = "IAM role name associated with the managed node group."
  value       = module.eks.eks_managed_node_groups["default"].iam_role_name
}

output "ecr_repository_urls" {
  description = "Repository URLs for the service images."
  value       = { for name, repo in aws_ecr_repository.services : name => repo.repository_url }
}

output "s3_bucket_names" {
  description = "Map of logical bucket keys to the actual bucket names."
  value       = local.s3_bucket_names
}

output "storage_access_policy_arn" {
  description = "ARN of the IAM policy that allows the storage service to access S3."
  value       = aws_iam_policy.storage_access.arn
}

output "storage_irsa_role_arn" {
  description = "IAM role ARN assumed by the storage service via IRSA."
  value       = aws_iam_role.storage_irsa.arn
}
