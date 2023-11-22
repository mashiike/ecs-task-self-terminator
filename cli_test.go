package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/motemen/go-testutil/dataloc"
	"github.com/stretchr/testify/require"
)

func TestCLIParse(t *testing.T) {
	cases := []struct {
		name     string
		envs     map[string]string
		args     []string
		expected CLI
	}{
		{
			name: "default",
			args: []string{"ecs-task-self-terminator"},
			expected: CLI{
				SSMAgentLogLocation:  "/var/log/amazon/ssm/amazon-ssm-agent.log",
				LogFormat:            "text",
				LogLevel:             slog.LevelInfo,
				IdleTimeout:          15 * time.Minute,
				MetricsCheckInterval: 1 * time.Second,
			},
		},
		{
			name: "initial-wait-time",
			args: []string{"ecs-task-self-terminator", "--initial-wait-time", "1m"},
			expected: CLI{
				SSMAgentLogLocation:  "/var/log/amazon/ssm/amazon-ssm-agent.log",
				LogFormat:            "text",
				LogLevel:             slog.LevelInfo,
				InitialWaitTime:      1 * time.Minute,
				IdleTimeout:          15 * time.Minute,
				MetricsCheckInterval: 1 * time.Second,
			},
		},
		{
			name: "as wrapper",
			args: []string{"ecs-task-self-terminator", "--initial-wait-time", "1m", "--", "sleep", "1"},
			expected: CLI{
				SSMAgentLogLocation:  "/var/log/amazon/ssm/amazon-ssm-agent.log",
				LogFormat:            "text",
				LogLevel:             slog.LevelInfo,
				InitialWaitTime:      1 * time.Minute,
				IdleTimeout:          15 * time.Minute,
				Commands:             []string{"sleep", "1"},
				MetricsCheckInterval: 1 * time.Second,
			},
		},
		{
			name: "log-config",
			args: []string{
				"ecs-task-self-terminator",
				"--log-format", "json",
				"--log-level", "debug",
				"--initial-wait-time", "1m",
				"--idle-timeout", "5m",
			},
			expected: CLI{
				SSMAgentLogLocation:  "/var/log/amazon/ssm/amazon-ssm-agent.log",
				LogFormat:            "json",
				LogLevel:             slog.LevelDebug,
				InitialWaitTime:      1 * time.Minute,
				IdleTimeout:          5 * time.Minute,
				MetricsCheckInterval: 1 * time.Second,
			},
		},
		{
			name: "max-life-time",
			args: []string{
				"ecs-task-self-terminator",
				"--max-life-time", "1h",
			},
			expected: CLI{
				SSMAgentLogLocation:  "/var/log/amazon/ssm/amazon-ssm-agent.log",
				LogFormat:            "text",
				LogLevel:             slog.LevelInfo,
				IdleTimeout:          15 * time.Minute,
				MaxLifeTime:          1 * time.Hour,
				MetricsCheckInterval: 1 * time.Second,
			},
		},
		{
			name: "from env",
			envs: map[string]string{
				"ECS_TST_LOG_FORMAT":        "json",
				"ECS_TST_LOG_LEVEL":         "warn",
				"ECS_TST_INITIAL_WAIT_TIME": "1m",
				"ECS_TST_IDLE_TIMEOUT":      "5m",
				"ECS_TST_MAX_LIFE_TIME":     "1h",
			},
			args: []string{
				"ecs-task-self-terminator",
			},
			expected: CLI{
				SSMAgentLogLocation:  "/var/log/amazon/ssm/amazon-ssm-agent.log",
				LogFormat:            "json",
				LogLevel:             slog.LevelWarn,
				InitialWaitTime:      1 * time.Minute,
				IdleTimeout:          5 * time.Minute,
				MaxLifeTime:          1 * time.Hour,
				MetricsCheckInterval: 1 * time.Second,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testLoc := dataloc.L(tc.name)
			for k, v := range tc.envs {
				t.Setenv(k, v)
			}
			var actual CLI
			err := actual.Parse(tc.args[1:])
			require.NoError(t, err, testLoc)
			require.EqualValues(t, tc.expected, actual, testLoc)
		})
	}
}
