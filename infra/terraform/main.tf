provider "aws" {
  region = var.aws_region
}

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  name_prefix   = lower("${var.project_name}-${var.environment}")
  azs           = slice(data.aws_availability_zones.available.names, 0, length(var.public_subnet_cidrs))
  default_tags  = merge({
    Project     = var.project_name,
    Environment = var.environment,
  }, var.tags)
  ecr_repo_names = { for name in var.ecr_repository_names : name => "${local.name_prefix}-${name}" }
  s3_bucket_names = {
    video_source      = coalesce(var.video_source_bucket_name, "${local.name_prefix}-video-source")
    transcoded_output = coalesce(var.transcoded_output_bucket_name, "${local.name_prefix}-transcoded")
    static_assets     = coalesce(var.static_assets_bucket_name, "${local.name_prefix}-assets")
  }
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.1"

  name = "${local.name_prefix}-vpc"
  cidr = var.vpc_cidr

  azs             = local.azs
  public_subnets  = var.public_subnet_cidrs
  private_subnets = var.private_subnet_cidrs

  enable_dns_hostnames = true
  enable_dns_support   = true

  enable_nat_gateway   = true
  single_nat_gateway   = true
  one_nat_gateway_per_az = false

  public_subnet_tags = {
    "kubernetes.io/role/elb" = "1"
  }

  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = "1"
  }

  tags = local.default_tags
}

resource "aws_security_group" "eks_additional" {
  name        = "${local.name_prefix}-eks-additional"
  description = "Additional access rules for the EKS control plane"
  vpc_id      = module.vpc.vpc_id

  ingress {
    description = "Allow API access from VPC"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = [module.vpc.vpc_cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.default_tags
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.8"

  cluster_name                   = "${local.name_prefix}-eks"
  cluster_version                = var.eks_cluster_version
  vpc_id                         = module.vpc.vpc_id
  subnet_ids                     = module.vpc.private_subnets
  cluster_endpoint_public_access = true
  enable_irsa                    = true

  cluster_additional_security_group_ids = [aws_security_group.eks_additional.id]

  eks_managed_node_groups = {
    default = {
      name           = "${local.name_prefix}-nodes"
      instance_types = var.eks_node_instance_types
      min_size       = var.eks_node_min_size
      max_size       = var.eks_node_max_size
      desired_size   = var.eks_node_desired_size
      subnet_ids     = module.vpc.private_subnets
      ami_type       = "AL2_x86_64"
      capacity_type  = "ON_DEMAND"
    }
  }

  tags = local.default_tags
}

resource "aws_ecr_repository" "services" {
  for_each = local.ecr_repo_names

  name                 = each.value
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = merge(local.default_tags, { Service = each.key })
}

resource "aws_s3_bucket" "media" {
  for_each = local.s3_bucket_names

  bucket        = each.value
  force_destroy = var.bucket_force_destroy

  tags = merge(local.default_tags, { Purpose = "${each.key}-bucket" })
}

resource "aws_s3_bucket_versioning" "media" {
  for_each = aws_s3_bucket.media

  bucket = each.value.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_public_access_block" "media" {
  for_each = aws_s3_bucket.media

  bucket = each.value.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

data "aws_iam_policy_document" "storage_access" {
  statement {
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:AbortMultipartUpload"
    ]
    resources = flatten([
      for name in values(local.s3_bucket_names) : [
        "arn:aws:s3:::${name}/*"
      ]
    ])
  }

  statement {
    actions   = ["s3:ListBucket"]
    resources = [for name in values(local.s3_bucket_names) : "arn:aws:s3:::${name}"]
  }
}

resource "aws_iam_policy" "storage_access" {
  name        = "${local.name_prefix}-storage-access"
  description = "Allow the storage service to access media buckets"
  policy      = data.aws_iam_policy_document.storage_access.json

  tags = local.default_tags
}

data "aws_iam_policy_document" "storage_assume_role" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    effect  = "Allow"

    principals {
      type        = "Federated"
      identifiers = [module.eks.oidc_provider_arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${replace(module.eks.cluster_oidc_issuer_url, "https://", "")}:sub"
      values   = ["system:serviceaccount:${var.storage_service_namespace}:${var.storage_service_account_name}"]
    }
  }
}

resource "aws_iam_role" "storage_irsa" {
  name               = "${local.name_prefix}-storage-irsa"
  assume_role_policy = data.aws_iam_policy_document.storage_assume_role.json

  tags = local.default_tags
}

resource "aws_iam_role_policy_attachment" "storage_irsa" {
  role       = aws_iam_role.storage_irsa.name
  policy_arn = aws_iam_policy.storage_access.arn
}
