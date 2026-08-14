package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

// ExecutionResult records the updates applied and skipped during a run.
type ExecutionResult struct {
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
}

// Updater encapsulates AWS SDK clients and update processing logic.
type Updater struct {
	ecClient  *elasticache.Client
	snsClient *sns.Client
	accountID string
	cfg       Config
}

// NewUpdater initializes an Updater instance with AWS clients and configuration.
func NewUpdater(ecClient *elasticache.Client, snsClient *sns.Client, accountID string, cfg Config) *Updater {
	return &Updater{
		ecClient:  ecClient,
		snsClient: snsClient,
		accountID: accountID,
		cfg:       cfg,
	}
}

// ProcessUpdates checks pending updates, evaluates policy tags and bake times, and applies updates.
func (u *Updater) ProcessUpdates(ctx context.Context) (ExecutionResult, error) {
	var result ExecutionResult

	// Use AWS SDK Paginator for DescribeUpdateActions
	paginator := elasticache.NewDescribeUpdateActionsPaginator(u.ecClient, &elasticache.DescribeUpdateActionsInput{
		ServiceUpdateStatus: []types.ServiceUpdateStatus{types.ServiceUpdateStatusAvailable},
		UpdateActionStatus:  []types.UpdateActionStatus{types.UpdateActionStatusNotApplied},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return result, fmt.Errorf("fetching update actions page: %w", err)
		}

		for _, action := range page.UpdateActions {
			serviceUpdateName := aws.ToString(action.ServiceUpdateName)
			repGroupID := aws.ToString(action.ReplicationGroupId)
			cacheClusterID := aws.ToString(action.CacheClusterId)

			var resourceID, resourceType string
			if repGroupID != "" {
				resourceID = repGroupID
				resourceType = "replicationgroup"
			} else if cacheClusterID != "" {
				resourceID = cacheClusterID
				resourceType = "cluster"
			} else {
				continue
			}

			// Construct ARN for tag inspection
			arn := fmt.Sprintf("arn:aws:elasticache:%s:%s:%s:%s",
				u.cfg.Region, u.accountID, resourceType, resourceID)

			tags, err := u.getResourceTags(ctx, arn)
			if err != nil {
				slog.Warn("Failed to fetch tags for resource, assuming default policy",
					"resource", resourceID, "error", err)
			}

			// Policy Check 1: AutoUpdatePolicy Tag
			policy := strings.ToLower(tags["AutoUpdatePolicy"])
			if policy == "disabled" {
				slog.Info("Skipping resource due to AutoUpdatePolicy=disabled", "resource", resourceID)
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s (Tag policy disabled)", resourceID))
				continue
			}

			// Policy Check 2: Bake-in Period for Production or Policy=bake
			isProd := strings.ToLower(tags["Environment"]) == "production" || policy == "bake"
			if isProd && action.ServiceUpdateReleaseDate != nil {
				ageDays := int(time.Since(aws.ToTime(action.ServiceUpdateReleaseDate)).Hours() / 24)
				if ageDays < u.cfg.ProdBakeDays {
					msg := fmt.Sprintf("%s (Baking %dd/%dd)", resourceID, ageDays, u.cfg.ProdBakeDays)
					slog.Info("Deferring update due to bake-in window", "resource", resourceID, "age_days", ageDays)
					result.Skipped = append(result.Skipped, msg)
					continue
				}
			}

			// Apply Update Action
			input := &elasticache.BatchApplyUpdateActionInput{
				ServiceUpdateName: aws.String(serviceUpdateName),
			}
			if resourceType == "replicationgroup" {
				input.ReplicationGroupIds = []string{resourceID}
			} else {
				input.CacheClusterIds = []string{resourceID}
			}

			_, err = u.ecClient.BatchApplyUpdateAction(ctx, input)
			if err != nil {
				errMsg := fmt.Sprintf("%s (Error: %v)", resourceID, err)
				slog.Error("Failed to apply service update", "resource", resourceID, "error", err)
				result.Skipped = append(result.Skipped, errMsg)
				continue
			}

			successMsg := fmt.Sprintf("%s -> %s", resourceID, serviceUpdateName)
			slog.Info("Successfully triggered service update", "resource", resourceID, "update", serviceUpdateName)
			result.Applied = append(result.Applied, successMsg)
		}
	}

	return result, nil
}

func (u *Updater) getResourceTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := u.ecClient.ListTagsForResource(ctx, &elasticache.ListTagsForResourceInput{
		ResourceName: aws.String(arn),
	})
	if err != nil {
		return map[string]string{}, err
	}

	tags := make(map[string]string)
	for _, tag := range out.TagList {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return tags, nil
}

// SendSummaryNotification publishes execution results to the configured SNS topic.
func (u *Updater) SendSummaryNotification(ctx context.Context, res ExecutionResult) error {
	if u.cfg.SNSTopicARN == "" || (len(res.Applied) == 0 && len(res.Skipped) == 0) {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("ElastiCache Service Update Automation Execution Summary:\n\n")

	sb.WriteString(fmt.Sprintf("Applied Updates (%d):\n", len(res.Applied)))
	if len(res.Applied) > 0 {
		for _, item := range res.Applied {
			sb.WriteString(fmt.Sprintf(" - %s\n", item))
		}
	} else {
		sb.WriteString(" None\n")
	}

	sb.WriteString(fmt.Sprintf("\nSkipped / Deferred Updates (%d):\n", len(res.Skipped)))
	if len(res.Skipped) > 0 {
		for _, item := range res.Skipped {
			sb.WriteString(fmt.Sprintf(" - %s\n", item))
		}
	} else {
		sb.WriteString(" None\n")
	}

	subject := fmt.Sprintf("[ElastiCache Update] Processed %d Updates", len(res.Applied))
	_, err := u.snsClient.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(u.cfg.SNSTopicARN),
		Subject:  aws.String(subject),
		Message:  aws.String(sb.String()),
	})

	return err
}
