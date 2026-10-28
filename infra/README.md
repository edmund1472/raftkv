# Running raftkv on AWS

Terraform builds the AWS side (network, EKS cluster, image registry, deploy
permissions). GitHub Actions then builds, pushes and deploys on every push to
`main`.

**Cost:** about $4–5 per day while it exists (EKS control plane ~$2.40,
NAT gateway ~$1.10, two t3.small nodes ~$1.00, plus disks). Tear it down
when you're not demoing it.

## One-time setup

Needs: an AWS account, the AWS CLI logged in (`aws configure` or SSO), Terraform.

1. **State bucket** (once per AWS account):

   ```bash
   cd infra/bootstrap
   terraform init && terraform apply
   ```

   Copy the `init_command` it prints.

2. **The cluster** (takes ~15 minutes):

   ```bash
   cd ../aws
   terraform init -backend-config="bucket=<state_bucket>" -backend-config="region=us-east-1"
   terraform plan    # read what will be created
   terraform apply
   ```

   If the account already has GitHub's OIDC provider, add
   `-var create_github_oidc_provider=false`.

3. **Turn on deploys:** in GitHub, go to Settings → Secrets and variables →
   Actions → Variables, and add `AWS_DEPLOY_ROLE_ARN` with the
   `github_deploy_role_arn` output. Push to `main` and the deploy job runs.

4. **Use it from your laptop:**

   ```bash
   aws eks update-kubeconfig --region us-east-1 --name raftkv
   kubectl port-forward svc/raftkv-client 8080:8080
   ```

## Tearing down

Order matters. Kubernetes creates EBS disks that Terraform doesn't know about.

```bash
kubectl delete -k deploy/aws
kubectl delete pvc data-raftkv-0
cd infra/aws && terraform destroy
```

The StorageClass keeps disks after their PVC is deleted (`Retain`), so the
WAL survives mistakes. That also means the disk outlives the cluster: delete
any leftover `raftkv` volumes under EC2 → Volumes afterwards. Also delete the
`AWS_DEPLOY_ROLE_ARN` variable so pushes stop trying to deploy.
