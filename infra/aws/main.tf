data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_caller_identity" "current" {}

locals {
  # Three availability zones (separate data centres), so later a 3-node Raft
  # cluster can survive one of them going down.
  azs = slice(data.aws_availability_zones.available.names, 0, 3)
}

# ---------------------------------------------------------------------------
# Network
# ---------------------------------------------------------------------------

# Worker nodes go in private subnets with no public IPs. They reach the
# internet (to pull images, talk to AWS APIs) through a NAT gateway in the
# public subnets. One NAT gateway instead of one per zone saves ~$65/month;
# the trade-off is that losing its zone cuts outbound traffic for all nodes.
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.21"

  name = var.cluster_name
  cidr = "10.0.0.0/16"
  azs  = local.azs

  private_subnets = ["10.0.0.0/20", "10.0.16.0/20", "10.0.32.0/20"]
  public_subnets  = ["10.0.48.0/24", "10.0.49.0/24", "10.0.50.0/24"]

  enable_nat_gateway = true
  single_nat_gateway = true

  # Tell Kubernetes which subnets to put load balancers in.
  public_subnet_tags = {
    "kubernetes.io/role/elb" = 1
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = 1
  }
}

# ---------------------------------------------------------------------------
# Kubernetes (EKS)
# ---------------------------------------------------------------------------

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.37"

  cluster_name    = var.cluster_name
  cluster_version = var.kubernetes_version

  # Reachable from the internet so kubectl works from a laptop and from
  # GitHub Actions; access still requires valid AWS credentials.
  cluster_endpoint_public_access = true

  # Whoever runs terraform apply becomes a cluster admin.
  enable_cluster_creator_admin_permissions = true

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  cluster_addons = {
    coredns    = {}
    kube-proxy = {}
    vpc-cni = {
      before_compute = true
    }
    # Creates EBS disks for PersistentVolumeClaims. Without it the raftkv
    # PVC stays Pending forever.
    aws-ebs-csi-driver = {
      service_account_role_arn = module.ebs_csi_irsa.iam_role_arn
    }
  }

  eks_managed_node_groups = {
    default = {
      # Amazon Linux 2023: AL2 images aren't published for recent versions.
      ami_type       = "AL2023_x86_64_STANDARD"
      instance_types = [var.node_instance_type]

      min_size     = 1
      max_size     = 3
      desired_size = var.node_count
    }
  }

  # Lets the GitHub Actions deploy role run kubectl. Cluster-wide because the
  # deploy creates a StorageClass, which isn't namespaced.
  access_entries = {
    github_deploy = {
      principal_arn = aws_iam_role.github_deploy.arn
      policy_associations = {
        admin = {
          policy_arn = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"
          access_scope = {
            type = "cluster"
          }
        }
      }
    }
  }
}

# IAM role for the EBS CSI driver's pods only (IRSA: IAM Roles for Service
# Accounts), instead of giving every pod on the node permission to make disks.
module "ebs_csi_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.55"

  role_name             = "${var.cluster_name}-ebs-csi"
  attach_ebs_csi_policy = true

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["kube-system:ebs-csi-controller-sa"]
    }
  }
}

# ---------------------------------------------------------------------------
# Image registry (ECR)
# ---------------------------------------------------------------------------

resource "aws_ecr_repository" "raftkv" {
  name = "raftkv"

  # Images are tagged with the git commit SHA, and a tag must never be
  # reused for different code.
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  # Lets terraform destroy remove the repo even when it still has images.
  force_delete = true
}

resource "aws_ecr_lifecycle_policy" "raftkv" {
  repository = aws_ecr_repository.raftkv.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep only the 20 most recent images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 20
      }
      action = { type = "expire" }
    }]
  })
}

# ---------------------------------------------------------------------------
# GitHub Actions -> AWS, without stored keys
# ---------------------------------------------------------------------------

# GitHub signs a short-lived token for each workflow run. AWS trusts that
# token through this OIDC provider, so no long-lived AWS keys are stored in
# GitHub secrets.
resource "aws_iam_openid_connect_provider" "github" {
  count = var.create_github_oidc_provider ? 1 : 0

  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
}

locals {
  github_oidc_provider_arn = (
    var.create_github_oidc_provider
    ? aws_iam_openid_connect_provider.github[0].arn
    : "arn:aws:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/token.actions.githubusercontent.com"
  )
}

resource "aws_iam_role" "github_deploy" {
  name = "${var.cluster_name}-github-deploy"

  # Only workflow runs on this repo's main branch can assume the role: not
  # forks, not pull requests, not other branches.
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = local.github_oidc_provider_arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
      Condition = {
        StringEquals = {
          "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
          "token.actions.githubusercontent.com:sub" = "repo:${var.github_repository}:ref:refs/heads/main"
        }
      }
    }]
  })
}

# Least privilege: push to this one repository and look up this one cluster.
resource "aws_iam_role_policy" "github_deploy" {
  name = "deploy"
  role = aws_iam_role.github_deploy.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = "ecr:GetAuthorizationToken"
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "ecr:BatchCheckLayerAvailability",
          "ecr:BatchGetImage",
          "ecr:CompleteLayerUpload",
          "ecr:GetDownloadUrlForLayer",
          "ecr:InitiateLayerUpload",
          "ecr:PutImage",
          "ecr:UploadLayerPart",
        ]
        Resource = aws_ecr_repository.raftkv.arn
      },
      {
        Effect   = "Allow"
        Action   = "eks:DescribeCluster"
        Resource = module.eks.cluster_arn
      },
    ]
  })
}
