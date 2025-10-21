variable "aws_region" {
  type        = string
  description = "AWS region to deploy all resources in."
}

variable "project_name" {
  type        = string
  description = "Project name used for tagging and naming resources."
  default     = "tritontube"
}

variable "environment" {
  type        = string
  description = "Environment name (e.g. dev, staging, prod)."
  default     = "dev"
}

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC."
  default     = "10.0.0.0/16"
}

variable "public_subnet_cidrs" {
  type        = list(string)
  description = "List of CIDR blocks for public subnets."
  default     = ["10.0.0.0/24", "10.0.1.0/24"]
}

variable "private_subnet_cidrs" {
  type        = list(string)
  description = "List of CIDR blocks for private subnets."
  default     = ["10.0.10.0/24", "10.0.11.0/24"]
}

variable "eks_cluster_version" {
  type        = string
  description = "Kubernetes version for the EKS control plane."
  default     = "1.29"
}

variable "eks_node_instance_types" {
  type        = list(string)
  description = "Instance types for the managed node group."
  default     = ["t3.medium"]
}

variable "eks_node_desired_size" {
  type        = number
  description = "Desired size of the managed node group."
  default     = 2
}

variable "eks_node_min_size" {
  type        = number
  description = "Minimum size of the managed node group."
  default     = 1
}

variable "eks_node_max_size" {
  type        = number
  description = "Maximum size of the managed node group."
  default     = 4
}

variable "ecr_repository_names" {
  type        = list(string)
  description = "List of short names for ECR repositories that will host container images."
  default     = ["web", "metadata", "storage"]
}

variable "video_source_bucket_name" {
  type        = string
  description = "Optional explicit name for the S3 bucket that stores raw video sources."
  default     = null
  nullable    = true
}

variable "transcoded_output_bucket_name" {
  type        = string
  description = "Optional explicit name for the S3 bucket that stores transcoded outputs."
  default     = null
  nullable    = true
}

variable "static_assets_bucket_name" {
  type        = string
  description = "Optional explicit name for the S3 bucket that stores static site assets."
  default     = null
  nullable    = true
}

variable "bucket_force_destroy" {
  type        = bool
  description = "Whether to allow Terraform to delete non-empty S3 buckets."
  default     = false
}

variable "storage_service_namespace" {
  type        = string
  description = "Namespace of the Kubernetes service account used by the storage service."
  default     = "storage"
}

variable "storage_service_account_name" {
  type        = string
  description = "Name of the Kubernetes service account used by the storage service."
  default     = "storage-service"
}

variable "tags" {
  type        = map(string)
  description = "Additional tags to apply to provisioned resources."
  default     = {}
}
