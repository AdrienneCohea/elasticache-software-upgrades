package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) (ExecutionResult, error) {
	slog.Info("Starting ElastiCache Service Update check...")

	// 1. Load runtime configuration
	cfg := LoadConfig()

	// 2. Load AWS SDK config
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		slog.Error("Failed to load AWS SDK config", "error", err)
		return ExecutionResult{}, fmt.Errorf("loading aws config: %w", err)
	}

	// 3. Obtain Account ID via STS for ARN construction
	stsClient := sts.NewFromConfig(awsCfg)
	callerID, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		slog.Error("Failed to get caller identity", "error", err)
		return ExecutionResult{}, fmt.Errorf("getting caller identity: %w", err)
	}
	accountID := aws.ToString(callerID.Account)

	// 4. Initialize updater dependencies
	ecClient := elasticache.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)
	ssmClient := ssm.NewFromConfig(awsCfg)
	updater := NewUpdater(ecClient, snsClient, ssmClient, accountID, cfg)

	// 5. Execute updates check and processing
	result, err := updater.ProcessUpdates(ctx)
	if err != nil {
		slog.Error("Update processing encountered an error", "error", err)
		return result, err
	}

	// 6. Send SNS summary notification if topic is configured
	if err := updater.SendSummaryNotification(ctx, result); err != nil {
		slog.Warn("Failed to send SNS summary notification", "error", err)
	}

	slog.Info("Completed ElastiCache update process",
		"applied_count", len(result.Applied),
		"skipped_count", len(result.Skipped))

	return result, nil
}
