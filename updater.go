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
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// ExecutionResult records the updates applied and skipped during a run.
type ExecutionResult struct {
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
}

// SSMClientAPI defines the subset of SSM operations required by Updater.
type SSMClientAPI interface {
	GetCalendarState(ctx context.Context, params *ssm.GetCalendarStateInput, optFns ...func(*ssm.Options)) (*ssm.GetCalendarStateOutput, error)
}

// Updater encapsulates AWS SDK clients and update processing logic.
type Updater struct {
	ecClient  *elasticache.Client
	snsClient *sns.Client
	ssmClient SSMClientAPI
	accountID string
	cfg       Config
}

// NewUpdater initializes an Updater instance with AWS clients and configuration.
func NewUpdater(ecClient *elasticache.Client, snsClient *sns.Client, ssmClient SSMClientAPI, accountID string, cfg Config) *Updater {
	return &Updater{
		ecClient:  ecClient,
		snsClient: snsClient,
		ssmClient: ssmClient,
		accountID: accountID,
		cfg:       cfg,
	}
}

// IsChangeCalendarOpen checks whether the configured AWS SSM Change Calendar is in OPEN state.
func (u *Updater) IsChangeCalendarOpen(ctx context.Context) (bool, error) {
	if u.cfg.SSMCalendarName == "" || u.ssmClient == nil {
		return true, nil
	}

	out, err := u.ssmClient.GetCalendarState(ctx, &ssm.GetCalendarStateInput{
		CalendarNames: []string{u.cfg.SSMCalendarName},
	})
	if err != nil {
		return false, fmt.Errorf("getting SSM change calendar state for '%s': %w", u.cfg.SSMCalendarName, err)
	}

	return out.State == ssmtypes.CalendarStateOpen, nil
}

// GetRequiredBakeDays determines the required bake-in duration in days based on resource tags.
// Environments 'alpha', 'beta', and 'gamma' are non-production. 'prod' is production.
func (u *Updater) GetRequiredBakeDays(tags map[string]string) (int, string) {
	env := strings.ToLower(strings.TrimSpace(tags["Environment"]))
	switch env {
	case "alpha":
		return u.cfg.AlphaBakeDays, "alpha"
	case "beta":
		return u.cfg.BetaBakeDays, "beta"
	case "gamma":
		return u.cfg.GammaBakeDays, "gamma"
	case "prod", "production":
		return u.cfg.ProdBakeDays, "prod"
	default:
		policy := strings.ToLower(strings.TrimSpace(tags["AutoUpdatePolicy"]))
		if policy == "bake" {
			return u.cfg.ProdBakeDays, "policy-bake"
		}
		return 0, env
	}
}

// ProcessUpdates checks pending updates, evaluates policy tags and bake times, and applies updates.
func (u *Updater) ProcessUpdates(ctx context.Context) (ExecutionResult, error) {
	var result ExecutionResult

	// Check SSM Change Calendar status if configured
	calendarOpen, err := u.IsChangeCalendarOpen(ctx)
	if err != nil {
		slog.Warn("Failed to check SSM Change Calendar, proceeding with cautions",
			"calendar", u.cfg.SSMCalendarName, "error", err)
	} else if !calendarOpen {
		slog.Info("SSM Change Calendar is CLOSED - production updates will be deferred",
			"calendar", u.cfg.SSMCalendarName)
	}

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

			env := strings.ToLower(strings.TrimSpace(tags["Environment"]))
			policy := strings.ToLower(strings.TrimSpace(tags["AutoUpdatePolicy"]))

			// Policy Check 1: AutoUpdatePolicy Tag (Opt-out)
			if policy == "disabled" {
				slog.Info("Skipping resource due to AutoUpdatePolicy=disabled", "resource", resourceID)
				result.Skipped = append(result.Skipped, fmt.Sprintf("%s (Tag policy disabled)", resourceID))
				continue
			}

			// Policy Check 2: SSM Change Calendar (Change Freeze for Production)
			isProd := env == "prod" || env == "production" || policy == "bake"
			hasEmergencyBypass := policy == "emergency-override" || policy == "force"

			if isProd && !calendarOpen && !hasEmergencyBypass {
				slog.Info("Deferring prod update due to active SSM Change Calendar freeze",
					"resource", resourceID, "calendar", u.cfg.SSMCalendarName)
				result.Skipped = append(result.Skipped,
					fmt.Sprintf("%s (Deferred: SSM Change Calendar '%s' is CLOSED)", resourceID, u.cfg.SSMCalendarName))
				continue
			}

			// Policy Check 3: Environment Bake-in Period
			requiredBakeDays, envLabel := u.GetRequiredBakeDays(tags)
			if requiredBakeDays > 0 && action.ServiceUpdateReleaseDate != nil {
				ageDays := int(time.Since(aws.ToTime(action.ServiceUpdateReleaseDate)).Hours() / 24)
				if ageDays < requiredBakeDays {
					label := envLabel
					if label == "" {
						label = "environment"
					}
					msg := fmt.Sprintf("%s (%s baking %dd/%dd)", resourceID, label, ageDays, requiredBakeDays)
					slog.Info("Deferring update due to bake-in window",
						"resource", resourceID,
						"env", label,
						"age_days", ageDays,
						"required_days", requiredBakeDays)
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

	if u.cfg.SSMCalendarName != "" {
		calendarOpen, _ := u.IsChangeCalendarOpen(ctx)
		stateStr := "OPEN"
		if !calendarOpen {
			stateStr = "CLOSED (Freeze Active)"
		}
		sb.WriteString(fmt.Sprintf("SSM Change Calendar: %s [%s]\n\n", u.cfg.SSMCalendarName, stateStr))
	}

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
