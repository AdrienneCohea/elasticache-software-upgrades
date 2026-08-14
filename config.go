package main

import (
	"os"
	"strconv"
)

// Config holds environment variables and runtime settings.
type Config struct {
	SNSTopicARN     string
	SSMCalendarName string
	AlphaBakeDays   int
	BetaBakeDays    int
	GammaBakeDays   int
	ProdBakeDays    int
	Region          string
}

// LoadConfig reads configuration settings from environment variables.
func LoadConfig() Config {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	return Config{
		SNSTopicARN:     os.Getenv("SNS_TOPIC_ARN"),
		SSMCalendarName: os.Getenv("SSM_CALENDAR_NAME"),
		AlphaBakeDays:   getEnvInt("ALPHA_BAKE_DAYS", 0),
		BetaBakeDays:    getEnvInt("BETA_BAKE_DAYS", 0),
		GammaBakeDays:   getEnvInt("GAMMA_BAKE_DAYS", 3),
		ProdBakeDays:    getEnvInt("PROD_BAKE_DAYS", 7),
		Region:          region,
	}
}

func getEnvInt(key string, defaultVal int) int {
	if raw := os.Getenv(key); raw != "" {
		if val, err := strconv.Atoi(raw); err == nil {
			return val
		}
	}
	return defaultVal
}
