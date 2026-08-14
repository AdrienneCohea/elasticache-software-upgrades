package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type mockSSMClient struct {
	state ssmtypes.CalendarState
	err   error
}

func (m *mockSSMClient) GetCalendarState(ctx context.Context, params *ssm.GetCalendarStateInput, optFns ...func(*ssm.Options)) (*ssm.GetCalendarStateOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &ssm.GetCalendarStateOutput{
		State: m.state,
	}, nil
}

func TestIsChangeCalendarOpen(t *testing.T) {
	ctx := context.Background()

	t.Run("no calendar configured", func(t *testing.T) {
		updater := NewUpdater(nil, nil, nil, "123456789012", Config{SSMCalendarName: ""})
		open, err := updater.IsChangeCalendarOpen(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !open {
			t.Errorf("expected calendar to be considered open when unconfigured")
		}
	})

	t.Run("calendar is open", func(t *testing.T) {
		mock := &mockSSMClient{state: ssmtypes.CalendarStateOpen}
		updater := NewUpdater(nil, nil, mock, "123456789012", Config{SSMCalendarName: "ProdChangeCalendar"})
		open, err := updater.IsChangeCalendarOpen(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !open {
			t.Errorf("expected calendar to be open")
		}
	})

	t.Run("calendar is closed", func(t *testing.T) {
		mock := &mockSSMClient{state: ssmtypes.CalendarStateClosed}
		updater := NewUpdater(nil, nil, mock, "123456789012", Config{SSMCalendarName: "ProdChangeCalendar"})
		open, err := updater.IsChangeCalendarOpen(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if open {
			t.Errorf("expected calendar to be closed")
		}
	})

	t.Run("calendar returns error", func(t *testing.T) {
		mock := &mockSSMClient{err: errors.New("ssm api error")}
		updater := NewUpdater(nil, nil, mock, "123456789012", Config{SSMCalendarName: "ProdChangeCalendar"})
		_, err := updater.IsChangeCalendarOpen(ctx)
		if err == nil {
			t.Errorf("expected error when SSM API fails")
		}
	})
}

func TestGetRequiredBakeDays(t *testing.T) {
	cfg := Config{
		AlphaBakeDays: 0,
		BetaBakeDays:  0,
		GammaBakeDays: 3,
		ProdBakeDays:  7,
	}
	updater := NewUpdater(nil, nil, nil, "123456789012", cfg)

	tests := []struct {
		name         string
		tags         map[string]string
		wantBakeDays int
		wantEnv      string
	}{
		{
			name:         "alpha environment - 0 days",
			tags:         map[string]string{"Environment": "alpha"},
			wantBakeDays: 0,
			wantEnv:      "alpha",
		},
		{
			name:         "alpha environment uppercase",
			tags:         map[string]string{"Environment": "ALPHA"},
			wantBakeDays: 0,
			wantEnv:      "alpha",
		},
		{
			name:         "beta environment - 0 days",
			tags:         map[string]string{"Environment": "beta"},
			wantBakeDays: 0,
			wantEnv:      "beta",
		},
		{
			name:         "gamma environment - 3 days",
			tags:         map[string]string{"Environment": "gamma"},
			wantBakeDays: 3,
			wantEnv:      "gamma",
		},
		{
			name:         "gamma environment mixed case",
			tags:         map[string]string{"Environment": "Gamma"},
			wantBakeDays: 3,
			wantEnv:      "gamma",
		},
		{
			name:         "prod environment - 7 days",
			tags:         map[string]string{"Environment": "prod"},
			wantBakeDays: 7,
			wantEnv:      "prod",
		},
		{
			name:         "production environment - 7 days",
			tags:         map[string]string{"Environment": "production"},
			wantBakeDays: 7,
			wantEnv:      "prod",
		},
		{
			name:         "policy bake tag override",
			tags:         map[string]string{"AutoUpdatePolicy": "bake"},
			wantBakeDays: 7,
			wantEnv:      "policy-bake",
		},
		{
			name:         "unspecified environment defaults to 0",
			tags:         map[string]string{},
			wantBakeDays: 0,
			wantEnv:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBakeDays, gotEnv := updater.GetRequiredBakeDays(tt.tags)
			if gotBakeDays != tt.wantBakeDays {
				t.Errorf("GetRequiredBakeDays() gotBakeDays = %v, want %v", gotBakeDays, tt.wantBakeDays)
			}
			if gotEnv != tt.wantEnv {
				t.Errorf("GetRequiredBakeDays() gotEnv = %v, want %v", gotEnv, tt.wantEnv)
			}
		})
	}
}

func TestLoadConfigDefaultsAndOverrides(t *testing.T) {
	// Test defaults
	os.Unsetenv("ALPHA_BAKE_DAYS")
	os.Unsetenv("BETA_BAKE_DAYS")
	os.Unsetenv("GAMMA_BAKE_DAYS")
	os.Unsetenv("PROD_BAKE_DAYS")
	os.Unsetenv("SSM_CALENDAR_NAME")
	os.Unsetenv("AWS_REGION")

	cfg := LoadConfig()
	if cfg.AlphaBakeDays != 0 {
		t.Errorf("expected AlphaBakeDays=0, got %d", cfg.AlphaBakeDays)
	}
	if cfg.BetaBakeDays != 0 {
		t.Errorf("expected BetaBakeDays=0, got %d", cfg.BetaBakeDays)
	}
	if cfg.GammaBakeDays != 3 {
		t.Errorf("expected GammaBakeDays=3, got %d", cfg.GammaBakeDays)
	}
	if cfg.ProdBakeDays != 7 {
		t.Errorf("expected ProdBakeDays=7, got %d", cfg.ProdBakeDays)
	}
	if cfg.SSMCalendarName != "" {
		t.Errorf("expected empty SSMCalendarName, got %s", cfg.SSMCalendarName)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("expected Region=us-east-1, got %s", cfg.Region)
	}

	// Test overrides
	os.Setenv("ALPHA_BAKE_DAYS", "1")
	os.Setenv("BETA_BAKE_DAYS", "2")
	os.Setenv("GAMMA_BAKE_DAYS", "5")
	os.Setenv("PROD_BAKE_DAYS", "14")
	os.Setenv("SSM_CALENDAR_NAME", "EnterpriseChangeFreeze")
	os.Setenv("AWS_REGION", "us-west-2")
	defer func() {
		os.Unsetenv("ALPHA_BAKE_DAYS")
		os.Unsetenv("BETA_BAKE_DAYS")
		os.Unsetenv("GAMMA_BAKE_DAYS")
		os.Unsetenv("PROD_BAKE_DAYS")
		os.Unsetenv("SSM_CALENDAR_NAME")
		os.Unsetenv("AWS_REGION")
	}()

	cfgOverride := LoadConfig()
	if cfgOverride.AlphaBakeDays != 1 {
		t.Errorf("expected AlphaBakeDays=1, got %d", cfgOverride.AlphaBakeDays)
	}
	if cfgOverride.BetaBakeDays != 2 {
		t.Errorf("expected BetaBakeDays=2, got %d", cfgOverride.BetaBakeDays)
	}
	if cfgOverride.GammaBakeDays != 5 {
		t.Errorf("expected GammaBakeDays=5, got %d", cfgOverride.GammaBakeDays)
	}
	if cfgOverride.ProdBakeDays != 14 {
		t.Errorf("expected ProdBakeDays=14, got %d", cfgOverride.ProdBakeDays)
	}
	if cfgOverride.SSMCalendarName != "EnterpriseChangeFreeze" {
		t.Errorf("expected SSMCalendarName=EnterpriseChangeFreeze, got %s", cfgOverride.SSMCalendarName)
	}
	if cfgOverride.Region != "us-west-2" {
		t.Errorf("expected Region=us-west-2, got %s", cfgOverride.Region)
	}
}
