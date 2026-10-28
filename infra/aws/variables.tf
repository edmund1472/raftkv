variable "region" {
  description = "AWS region for every resource."
  type        = string
  default     = "us-east-1"
}

variable "cluster_name" {
  description = "Name of the EKS cluster; also used to name related resources."
  type        = string
  default     = "raftkv"
}

variable "kubernetes_version" {
  description = "EKS Kubernetes version. Versions past standard support are billed at the much higher extended-support rate, so keep this current."
  type        = string
  default     = "1.36"
}

variable "node_instance_type" {
  description = "EC2 instance type for the worker nodes."
  type        = string
  default     = "t3.small"
}

variable "node_count" {
  description = "Number of worker nodes. Use 3 once Raft runs 3 replicas, so each lands on its own machine."
  type        = number
  default     = 2
}

variable "github_repository" {
  description = "GitHub repo (owner/name) whose main branch may deploy."
  type        = string
  default     = "edmund1472/raftkv"
}

variable "create_github_oidc_provider" {
  description = "Set to false if this AWS account already has GitHub's OIDC provider (only one is allowed per account)."
  type        = bool
  default     = true
}
