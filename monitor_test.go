package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonitor__HasActiveConnections(t *testing.T) {
	m := NewMonitor("testdata/amazon-ssm-agent.log")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err := m.Run(ctx)
	require.NoError(t, err)
	require.EqualValues(t, Metrics{
		ActiveConnections: 1,
		TotalConnections:  4,
		LastTimestamp:     time.Date(2023, 11, 17, 7, 40, 40, 0, time.UTC),
	}, m.Metrics())
}

func TestMonitor__NoActiveConnections(t *testing.T) {
	file, err := os.Open("testdata/amazon-ssm-agent.log")
	require.NoError(t, err)
	defer file.Close()
	reader := io.MultiReader(
		file,
		strings.NewReader("2023-11-17 07:45:32 INFO [ssm-session-worker] [ecs-execute-command-02f7755870b50f125] Session worker closed\n"),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m := NewMonitor("")
	err = m.RunWithReader(ctx, reader)
	require.NoError(t, err)
	require.EqualValues(t, Metrics{
		ActiveConnections: 0,
		TotalConnections:  4,
		LastTimestamp:     time.Date(2023, 11, 17, 7, 45, 32, 0, time.UTC),
	}, m.Metrics())
}
