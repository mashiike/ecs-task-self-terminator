package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Songmu/flextime"
	"github.com/stretchr/testify/require"
)

func TestApp(t *testing.T) {
	restore := flextime.Set(time.Date(2023, 11, 17, 7, 00, 00, 0, time.UTC))
	defer restore()
	tmpFile, err := os.CreateTemp("", "amazon-ssm-agent.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	cli := CLI{
		SSMAgentLogLocation: tmpFile.Name(),
		LogFormat:           "text",
		LogLevel:            slog.LevelDebug,
		InitialWaitTime:     1 * time.Minute,
		IdleTimeout:         5 * time.Minute,
		MaxLifeTime:         10 * time.Hour,
	}
	app, err := New(cli)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bs, err := os.ReadFile("testdata/amazon-ssm-agent.log")
	require.NoError(t, err)
	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})
	go func() {
		defer wg.Done()
		close(started)
		err = app.Run(ctx)
	}()
	<-started
	time.Sleep(500 * time.Microsecond)
	require.True(t, app.IsActive())
	tmpFile.Write(bs)
	flextime.Set(time.Date(2023, 11, 17, 7, 45, 35, 0, time.UTC))
	require.True(t, app.IsActive())
	fmt.Fprintln(tmpFile, "2023-11-17 07:45:32 INFO [ssm-session-worker] [ecs-execute-command-02f7755870b50f125] Session worker closed")
	require.True(t, app.IsActive())
	flextime.Set(time.Date(2023, 11, 17, 8, 30, 32, 0, time.UTC))
	wg.Wait()
	require.False(t, app.IsActive())
	require.NoError(t, err)
	require.Equal(t, "no active connections after idle timeout", app.StopReason())
}
