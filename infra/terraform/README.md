# TritonTube Infrastructure

This Terraform configuration provisions the AWS infrastructure that powers the TritonTube stack.

## Resources

The configuration creates:

- A VPC with public and private subnets, NAT gateway, and DNS support.
- Security groups for the EKS control plane.
- An EKS cluster with a managed node group and IAM Roles for Service Accounts (IRSA) enabled.
- ECR repositories for the `web`, `metadata`, and `storage` container images.
- S3 buckets for raw video sources, transcoded outputs, and static assets, plus an IAM policy scoped to those buckets.
- An IRSA role bound to the storage service account so pods can access S3 securely.

## Usage

1. Configure your AWS credentials (for example with `aws configure` or by exporting `AWS_PROFILE`).
2. Adjust the variables in `terraform.tfvars` (create this file) or provide them via the CLI.
3. Initialize and apply:

```bash
terraform init
terraform apply -var="aws_region=us-east-1"
```

Key output values include the ECR repository URLs, S3 bucket names, and the IAM role ARN used by the storage service. Feed those into the CI/CD workflow secrets so deployments can push/pull images and use IRSA. The GitHub Actions pipeline expects the following repository secrets to be set:

- `AWS_REGION`
- `EKS_CLUSTER_NAME`
- `AWS_GITHUB_ROLE_ARN` (IAM role assumed by the workflow)
- `WEB_ECR_REPOSITORY`, `METADATA_ECR_REPOSITORY`, `STORAGE_ECR_REPOSITORY` (short repository names created by Terraform)
- `STORAGE_IAM_ROLE_ARN` (IRSA role ARN output by Terraform)

Optionally, set `K8S_NAMESPACE` to override the default `tritontube` namespace used in the workflow.
