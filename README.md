# ElastiCache Software Updates Automation

An AWS Lambda function written in Go that automates the application of AWS ElastiCache service updates across clusters and replication groups. It reads the mandatory `Environment` tag from each ElastiCache resource, enforces progressive bake-in periods, and integrates with **AWS Systems Manager (SSM) Change Calendar** to respect corporate change freezes.

## Environment Bake-in Policies

The `Environment` tag is present on all ElastiCache clusters/replication groups and dictates the rollout policy:

| Type | Environment | Tag (`Environment`) | Bake-in Period | Behavior |
| :--- | :--- | :--- | :--- | :--- |
| **Non-Production** | **Alpha** | `alpha` | **0 days** | Applied as soon as available |
| **Non-Production** | **Beta** | `beta` | **0 days** | Applied as soon as available |
| **Non-Production** | **Gamma** | `gamma` | **3 days** | Applied once update is ≥ 3 days old |
| **Production** | **Prod** | `prod` / `production` | **7 days** | Applied once update is ≥ 7 days old (subject to Change Calendar) |

## Features

- **Progressive Multi-Environment Rollouts**: Automatically calculates service update age and enforces bake-in delay per environment tier.
- **AWS SSM Change Calendar Integration**: Checks enterprise change calendar state (`ssm:GetCalendarState`). When the calendar is `CLOSED` (e.g., during Q4 holiday change freezes), production updates are deferred automatically.
- **Emergency Bypass Override**: Allows critical zero-day / CVE patches during change freezes when explicitly tagged with `AutoUpdatePolicy=emergency-override` or `AutoUpdatePolicy=force`.
- **Resource Tag Policies**:
  - `Environment`: Always-present tag defining the environment tier (`alpha`, `beta`, `gamma`, `prod` / `production`).
  - `AutoUpdatePolicy=disabled`: Skips update actions for the resource.
  - `AutoUpdatePolicy=bake`: Enforces the production bake-in window regardless of environment name.
  - `AutoUpdatePolicy=emergency-override` / `force`: Bypasses SSM Change Calendar freeze for the tagged resource.
- **SNS Notifications**: Publishes an execution summary to an Amazon SNS topic detailing applied and skipped updates, including calendar state.
- **Optimized Lambda Binary & Container Support**: Deployable as a native ARM64 ZIP package or as an AWS Lambda container image (`provided.al2023`).

## Configuration

The Lambda function is configured via environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `AWS_REGION` | AWS region to query and apply updates | `us-east-1` |
| `SSM_CALENDAR_NAME` | Optional AWS SSM Change Calendar name / ARN to check for change freezes | `""` |
| `ALPHA_BAKE_DAYS` | Bake-in delay for alpha non-prod resources (days) | `0` |
| `BETA_BAKE_DAYS` | Bake-in delay for beta non-prod resources (days) | `0` |
| `GAMMA_BAKE_DAYS` | Bake-in delay for gamma non-prod resources (days) | `3` |
| `PROD_BAKE_DAYS` | Bake-in delay for prod resources (days) | `7` |
| `SNS_TOPIC_ARN` | Optional SNS topic ARN for execution summary notifications | `""` |

## Resource Tagging

Each ElastiCache resource includes:
- `Environment`: `alpha`, `beta`, `gamma`, or `prod`
- `AutoUpdatePolicy`: Optional tag (`disabled`, `bake`, or `emergency-override`)

## Build and Packaging

### Option 1: ZIP Archive (for standard Lambda deployment)
```bash
# Run unit tests
make test

# Build ARM64 binary (bootstrap) and package into function.zip
make zip
```

### Option 2: Container Image (for Lambda Container Image deployment)
```bash
# Build Docker image using multi-stage provided.al2023 base
make docker-build
```

## IAM Permissions

The Lambda execution role requires permissions to interact with ElastiCache, STS, SNS, and SSM:

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
        "sns:Publish",
        "ssm:GetCalendarState"
      ],
      "Resource": "*"
    }
  ]
}
```
