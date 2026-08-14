# ElastiCache Software Updates Automation

An AWS Lambda function written in Go that automates the application of AWS ElastiCache service updates across clusters and replication groups. It supports tag-based policies and configurable bake-in periods for production environments.

## Features

- **Automated Service Update Processing**: Queries and applies available and pending ElastiCache service updates.
- **Resource Tag Policies**:
  - `AutoUpdatePolicy=disabled`: Skips update actions for the resource.
  - `AutoUpdatePolicy=bake` or `Environment=production`: Enforces a configurable bake-in period before applying updates.
- **Configurable Bake-in Window**: Defers production updates until the update has been available for a specified number of days (default: 7 days).
- **SNS Notifications**: Publishes an execution summary to an Amazon SNS topic detailing applied and skipped updates.
- **Optimized Lambda Binary**: Built for `provided.al2023` / `arm64` runtime with `-tags lambda.norpc`.

## Configuration

The Lambda function is configured via environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `AWS_REGION` | AWS region to query and apply updates | `us-east-1` |
| `PROD_BAKE_DAYS` | Minimum days since release before applying updates to production resources | `7` |
| `SNS_TOPIC_ARN` | Optional SNS topic ARN for execution summary notifications | `""` |

## Resource Tagging

Apply the following tags to your ElastiCache clusters or replication groups to control update behavior:

- `AutoUpdatePolicy`: Set to `disabled` to opt out of automated updates, or `bake` to enforce bake-in delay.
- `Environment`: Set to `production` to automatically apply the bake-in period (`PROD_BAKE_DAYS`).

## Build and Packaging

To compile the binary and generate the deployment ZIP package:

```bash
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
