# ElastiCache Software Updates Automation

An AWS Lambda function written in Go that automates the application of AWS ElastiCache service updates across clusters and replication groups. It supports tag-based environment policies with progressive bake-in periods across promotion tiers.

## Environment Bake-in Policies

Updates are rolled out progressively across environments using the `Environment` resource tag:

| Environment | Tag (`Environment`) | Bake-in Period | Description |
| :--- | :--- | :--- | :--- |
| **Alpha** | `alpha` | **0 days** | Applied as soon as update is available |
| **Beta** | `beta` | **0 days** | Applied as soon as update is available |
| **Gamma** | `gamma` | **3 days** | Applied once update is at least 3 days old |
| **Prod** | `prod` / `production` | **7 days** | Applied once update is at least 7 days old |

## Features

- **Progressive Multi-Environment Rollouts**: Automatically calculates service update age and enforces bake-in delay per environment tier.
- **Resource Tag Policies**:
  - `Environment`: Sets the environment tier (`alpha`, `beta`, `gamma`, `prod` / `production`).
  - `AutoUpdatePolicy=disabled`: Skips update actions for the resource.
  - `AutoUpdatePolicy=bake`: Manually enforces the production bake-in window regardless of environment name.
- **SNS Notifications**: Publishes an execution summary to an Amazon SNS topic detailing applied and skipped updates.
- **Optimized Lambda Binary**: Built for `provided.al2023` / `arm64` runtime with `-tags lambda.norpc`.

## Configuration

The Lambda function is configured via environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `AWS_REGION` | AWS region to query and apply updates | `us-east-1` |
| `ALPHA_BAKE_DAYS` | Bake-in delay for alpha resources (days) | `0` |
| `BETA_BAKE_DAYS` | Bake-in delay for beta resources (days) | `0` |
| `GAMMA_BAKE_DAYS` | Bake-in delay for gamma resources (days) | `3` |
| `PROD_BAKE_DAYS` | Bake-in delay for prod resources (days) | `7` |
| `SNS_TOPIC_ARN` | Optional SNS topic ARN for execution summary notifications | `""` |

## Resource Tagging

Apply tags to your ElastiCache clusters or replication groups to control update behavior:

- `Environment`: `alpha`, `beta`, `gamma`, or `prod`
- `AutoUpdatePolicy`: Set to `disabled` to opt out of automated updates

## Build and Packaging

To compile the binary and generate the deployment ZIP package:

```bash
# Run unit tests
make test

# Build ARM64 binary (bootstrap) and zip for AWS Lambda
make zip
```

## IAM Permissions

The Lambda execution role requires permissions to interact with ElastiCache, STS, and SNS:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "elasticache:DescribeUpdateActions",
        "elasticache:BatchApplyUpdateAction",
        "elasticache:ListTagsForResource",
        "sts:GetCallerIdentity",
        "sns:Publish"
      ],
      "Resource": "*"
    }
  ]
}
```
