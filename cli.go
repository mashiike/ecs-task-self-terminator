package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/alecthomas/kong"
)

type CLI struct {
	SSMAgentLogLocation   string        `help:"SSM Agent Log Location" default:"/var/log/amazon/ssm/amazon-ssm-agent.log" env:"ECS_TST_SSM_AGENT_LOG_LOCATION" type:"path"`
	LogFormat             string        `help:"Log format" enum:"json,text" default:"text" env:"ECS_TST_LOG_FORMAT"`
	LogLevel              slog.Level    `help:"Log level" default:"info" env:"ECS_TST_LOG_LEVEL"`
	InitialWaitTime       time.Duration `help:"Initial wait time before starting the first ECS Exec or Portforward session" env:"ECS_TST_INITIAL_WAIT_TIME"`
	IdleTimeout           time.Duration `help:"If no ECS Exec sessions occur within the specified time duration, the application will automatically terminate the ECS Task" default:"15m" env:"ECS_TST_IDLE_TIMEOUT"`
	MaxLifeTime           time.Duration `help:"Maximum time duration for ECS Task" env:"ECS_TST_MAX_LIFE_TIME"`
	SetDesiredCountToZero bool          `help:"Set desired count to zero when stopping task" env:"ECS_TST_SET_DESIRED_COUNT_TO_ZERO"`
	StopTaskOnExit        bool          `help:"Stop task when stopping task" env:"ECS_TST_STOP_TASK"`
	KeepAliveTask         bool          `help:"Keep alive task when finished command" env:"ECS_TST_KEEP_ALIVE_TASK"`
	Commands              []string      `arg:"" optional:"" help:"Command to run, if set run as wrapper"`
}

func (cli *CLI) Parse(args []string) error {
	parsed, err := kong.New(
		cli,
		kong.Name("ecs-task-self-terminator"),
		kong.Description("ECS Task Self Terminator "+Version),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Summary: true,
			Compact: true,
		}),
		kong.Vars{
			"version": Version,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to parse CLI: %w", err)
	}
	_, err = parsed.Parse(args)
	if err != nil {
		return fmt.Errorf("failed to parse Args: %w", err)
	}
	return nil
}
