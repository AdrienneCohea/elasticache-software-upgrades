package main

import (
	"os"
	"strconv"
)

// Config holds environment variables and runtime settings.
type Config struct {
	SNSTopicARN  string
	ProdBakeDays int
	Region       string
}

// LoadConfig reads configuration settings from environment variables.
func LoadConfig() Config {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	bakeDays := 7
	if rawDays := os.Getenv("PROD_BAKE_DAYS"); rawDays != "" {
		if parsed, err := strconv.Atoi(rawDays); err == nil {
			bakeDays = parsed
		}
	}

	return Config{
		SNSTopicARN:  os.Getenv("SNS_TOPIC_ARN"),
		ProdBakeDays: bakeDays,
		Region:       region,
	}
}
