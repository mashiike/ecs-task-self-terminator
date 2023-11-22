package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Songmu/flextime"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/cenkalti/backoff"
	"github.com/fatih/color"
	"github.com/mashiike/slogutils"
)

type App struct {
	cli        CLI
	logger     *slog.Logger
	startAt    time.Time
	stopReason string
	isActive   int32
	ecsMeta    *ECSMeta
	httpClient *http.Client
	ecsClient  ECSClient
}

type ECSClient interface {
	UpdateService(ctx context.Context, params *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	StopTask(ctx context.Context, params *ecs.StopTaskInput, optFns ...func(*ecs.Options)) (*ecs.StopTaskOutput, error)
}

func New(cli CLI) (*App, error) {
	if cli.InitialWaitTime == 0 {
		cli.InitialWaitTime = cli.IdleTimeout
	}
	newHander := func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(w, opts)
	}
	if cli.LogFormat == "json" {
		newHander = func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
			return slog.NewJSONHandler(w, opts)
		}
	}
	middleware := slogutils.NewMiddleware(
		newHander,
		slogutils.MiddlewareOptions{
			ModifierFuncs: map[slog.Level]slogutils.ModifierFunc{
				slog.LevelDebug: slogutils.Color(color.FgBlack),
				slog.LevelInfo:  nil,
				slog.LevelWarn:  slogutils.Color(color.FgYellow),
				slog.LevelError: slogutils.Color(color.FgRed, color.Bold),
			},
			RecordTransformerFuncs: []slogutils.RecordTransformerFunc{
				slogutils.DefaultAttrs(
					"version", Version,
					"app", "ecs-task-self-terminator",
				),
				slogutils.ConvertLegacyLevel(
					map[string]slog.Level{
						"debug":  slog.LevelDebug,
						"info":   slog.LevelInfo,
						"notice": slog.LevelInfo, // for backward compatibility
						"warn":   slog.LevelWarn,
						"error":  slog.LevelError,
					},
					false, // in-casesensitive
				),
				func(r slog.Record) slog.Record {
					if r.Level >= slog.LevelInfo && r.Level < slog.LevelError {
						return r
					}
					fs := runtime.CallersFrames([]uintptr{r.PC})
					f, _ := fs.Next()
					r.Add(
						slog.SourceKey,
						&slog.Source{
							Function: f.Function,
							File:     f.File,
							Line:     f.Line,
						},
					)
					return r
				},
			},
			Writer: os.Stderr,
			HandlerOptions: &slog.HandlerOptions{
				Level: cli.LogLevel,
			},
		},
	)
	logger := slog.New(middleware)
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}

	return &App{
		cli:        cli,
		logger:     logger,
		startAt:    flextime.Now(),
		httpClient: http.DefaultClient,
		ecsClient:  ecs.NewFromConfig(awsCfg),
	}, nil
}

func (app *App) Run(ctx context.Context) error {
	if err := app.detectECSMeta(ctx); err != nil {
		return fmt.Errorf("failed to detect ecs meta: %w", err)
	}
	atomic.StoreInt32(&app.isActive, 1)
	defer func() {
		atomic.StoreInt32(&app.isActive, 0)
		postCtx, postCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer postCancel()
		if err := app.postProcess(postCtx); err != nil {
			app.logger.ErrorContext(ctx, "post process error", "error", err)
		}
	}()
	if app.cli.MetricsCheckInterval == 0 {
		app.cli.MetricsCheckInterval = 1 * time.Second
	}
	app.logger.DebugContext(ctx, "starting ecs-task-self-terminator", "version", Version)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if app.cli.MaxLifeTime > 0 {
		app.logger.DebugContext(ctx, "setting max lifetime", "maxLifeTime", app.cli.MaxLifeTime)
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, app.cli.MaxLifeTime)
		defer timeoutCancel()
	}
	if len(app.cli.Commands) > 0 {
		app.logger.DebugContext(ctx, "running as wrapper", "commands", app.cli.Commands)
		var execCancel context.CancelCauseFunc
		if app.cli.KeepAliveTask {
			execCancel = func(cause error) {
				if cause != nil {
					app.logger.WarnContext(ctx, "exec command finished", "error", cause)
				}
			}
		} else {
			ctx, execCancel = context.WithCancelCause(ctx)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := app.execCommand(ctx, os.Stdin, app.cli.Commands[0], app.cli.Commands[1:]...)
			execCancel(err)
		}()
	}
	m := NewMonitor(app.cli.SSMAgentLogLocation)
	go func() {
		app.logger.DebugContext(ctx, "starting monitor", "logFilePath", app.cli.SSMAgentLogLocation)
		if err := m.Run(ctx); err != nil {
			app.logger.ErrorContext(ctx, "monitor error", "error", err)
			cancel()
		}
	}()
	if str := app.mainLoop(ctx, cancel, m); app.stopReason == "" {
		app.stopReason = str
	}
	wg.Wait()
	if err := context.Cause(ctx); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if app.stopReason == "" {
			app.stopReason = err.Error()
		}
	}
	return nil
}

func (app *App) StopReason() string {
	return app.stopReason
}

func (app *App) IsActive() bool {
	return atomic.LoadInt32(&app.isActive) == 1
}

func (app *App) mainLoop(ctx context.Context, cancel context.CancelFunc, m *Monitor) string {
	app.logger.DebugContext(ctx, "starting main loop")
	defer func() {
		cancel()
		app.logger.DebugContext(ctx, "main loop finished")
	}()
	for {
		select {
		case <-ctx.Done():
			app.logger.DebugContext(ctx, "context done", "error", ctx.Err())
			return ctx.Err().Error()
		default:
			time.Sleep(app.cli.MetricsCheckInterval)
		}
		metrics := m.Metrics()
		app.logger.DebugContext(ctx, "monitor metrics", "metrics", metrics)
		if metrics.TotalConnections == 0 {
			app.logger.DebugContext(ctx, "no total connections", "startAt", app.startAt, "since", flextime.Since(app.startAt))
			if flextime.Since(app.startAt) > app.cli.InitialWaitTime {
				app.logger.InfoContext(ctx, "no total connections after initial wait time")
				return "no total connections after initial wait time"
			}
			continue
		}
		if metrics.ActiveConnections == 0 {
			app.logger.DebugContext(ctx, "no active connections")
			if flextime.Since(metrics.LastTimestamp) > app.cli.IdleTimeout {
				app.logger.InfoContext(ctx, "no active connections after idle timeout")
				return "no active connections after idle timeout"
			}
			continue
		}
		app.logger.DebugContext(ctx, "has active connections")
	}
}

func (app *App) postProcess(ctx context.Context) error {
	app.logger.DebugContext(ctx, "starting post process")
	if app.cli.StopTaskOnExit {
		if err := app.stopTask(ctx); err != nil {
			return fmt.Errorf("failed to stop task: %w", err)
		}
	}
	if app.cli.SetDesiredCountToZero {
		if err := app.setDesiredCountToZero(ctx); err != nil {
			return fmt.Errorf("failed to set desired count to zero: %w", err)
		}
	}
	return nil
}

func (app *App) stopTask(ctx context.Context) error {
	if app.ecsMeta == nil {
		app.logger.WarnContext(ctx, "ecs meta is not detected, can not stop task")
		return nil
	}
	app.logger.DebugContext(ctx, "stopping task", "taskARN", app.ecsMeta.TaskARN)
	_, err := app.ecsClient.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(app.ecsMeta.Cluster),
		Task:    aws.String(app.ecsMeta.TaskARN),
		Reason:  aws.String("stopped by ecs-task-self-terminator"),
	})
	if err != nil {
		return err
	}
	app.logger.InfoContext(ctx, "stopped task", "taskARN", app.ecsMeta.TaskARN)
	return nil
}

func (app *App) setDesiredCountToZero(ctx context.Context) error {
	if app.ecsMeta == nil {
		return errors.New("ecs meta is not detected, can not set desired count to zero")
	}
	app.logger.DebugContext(ctx, "setting desired count to zero", "serviceName", app.ecsMeta.ServiceName)
	_, err := app.ecsClient.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      aws.String(app.ecsMeta.Cluster),
		Service:      aws.String(app.ecsMeta.ServiceName),
		DesiredCount: aws.Int32(0),
	})
	if err != nil {
		return err
	}
	app.logger.InfoContext(ctx, "set desired count to zero", "serviceName", app.ecsMeta.ServiceName)
	return nil
}

func (app *App) execCommand(ctx context.Context, stdin io.Reader, name string, args ...string) error {
	app.logger.DebugContext(ctx, "executing command", "name", name, "args", args)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = stdin
	execErr := cmd.Run()
	app.logger.DebugContext(ctx, "command finished", "error", execErr)
	if execErr != nil {
		return &ErrorWithExitCode{
			Err:      execErr,
			ExitCode: cmd.ProcessState.ExitCode(),
		}
	}
	return nil
}

type ECSMeta struct {
	Cluster     string `json:"Cluster"`
	TaskARN     string `json:"TaskARN"`
	Family      string `json:"Family"`
	ServiceName string `json:"ServiceName"`
	Revision    string `json:"Revision"`
}

func (ecsMeta ECSMeta) TaskDefinistionARN() string {
	prefix := strings.Split(ecsMeta.TaskARN, "/")[0]
	return fmt.Sprintf("%s-definition/%s:%s", prefix, ecsMeta.Family, ecsMeta.Revision)
}

func (app *App) detectECSMeta(ctx context.Context) error {
	app.logger.DebugContext(ctx, "detecting ecs meta")
	metadataURL := os.Getenv("ECS_CONTAINER_METADATA_URI_V4")
	if metadataURL == "" {
		app.logger.WarnContext(ctx, "ECS_CONTAINER_METADATA_URI_V4 is not set")
		return nil
	}
	u, err := url.Parse(metadataURL)
	if err != nil {
		return err
	}
	u = u.JoinPath("/task")
	b := backoff.WithMaxRetries(
		backoff.NewExponentialBackOff(),
		3,
	)
	var ecsMeta ECSMeta
	operation := func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := app.httpClient.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&ecsMeta); err != nil {
			return err
		}
		return nil
	}
	if err := backoff.Retry(operation, b); err != nil {
		return err
	}
	app.ecsMeta = &ecsMeta
	app.logger.DebugContext(ctx, "detected ecs meta", "ecsMeta", ecsMeta)
	return nil
}
