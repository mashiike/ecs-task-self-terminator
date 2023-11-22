# ecs-task-self-terminator
ECS SSM Connection Monitor and Auto-Terminator

```
Usage: ecs-task-self-terminator [<commands> ...]

ECS Task Self Terminator v0.1.0

Arguments:
  [<commands> ...]    Command to run, if set run as wrapper

Flags:
  -h, --help                                                                 Show context-sensitive help.
      --ssm-agent-log-location="/var/log/amazon/ssm/amazon-ssm-agent.log"    SSM Agent Log Location ($ECS_TST_SSM_AGENT_LOG_LOCATION)
      --log-format="text"                                                    Log format ($ECS_TST_LOG_FORMAT)
      --log-level=info                                                       Log level ($ECS_TST_LOG_LEVEL)
      --initial-wait-time=DURATION                                           Initial wait time before starting the first ECS Exec or Portforward session ($ECS_TST_INITIAL_WAIT_TIME)
      --idle-timeout=15m                                                     If no ECS Exec sessions occur within the specified time duration, the application will automatically terminate the ECS Task ($ECS_TST_IDLE_TIMEOUT)
      --max-life-time=DURATION                                               Maximum time duration for ECS Task ($ECS_TST_MAX_LIFE_TIME)
      --set-desired-count-to-zero                                            Set desired count to zero when stopping task ($ECS_TST_SET_DESIRED_COUNT_TO_ZERO)
      --stop-task-on-exit                                                    Stop task when stopping task ($ECS_TST_STOP_TASK)
      --keep-alive-task                                                      Keep alive task when finished command ($ECS_TST_KEEP_ALIVE_TASK)
      --metrics-check-interval=1s                                            Metrics check interval ($ECS_TST_METRICS_CHECK_INTERVAL)
      --vervose                                                              log output verbose output ($ECS_TST_VERBOSE)
      --ecs-service-name=STRING                                              ECS Service Name ($ECS_TST_ECS_SERVICE_NAME)
```

## QuickStart

This application is designed for simple operation containers. Here is a use case:

1. Create an IAM Role for the ECS Task with the following policy:

```json
{
   "Version": "2012-10-17",
   "Statement": [
       {
       "Effect": "Allow",
       "Action": [
            "ecs:StopTask",
            "ssmmessages:CreateControlChannel",
            "ssmmessages:CreateDataChannel",
            "ssmmessages:OpenControlChannel",
            "ssmmessages:OpenDataChannel"
       ],
      "Resource": "*"
      }
   ]
}
```

2. Define the ECS Task as follows:

```json
{
  "containerDefinitions": [
    {
      "cpu": 0,
      "essential": true,
      "image": "ghcr.io/mashiike/ecs-task-self-terminator",
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/gate",
          "awslogs-region": "ap-northeast-1",
          "awslogs-stream-prefix": "ecs-task-self-terminator"
        }
      },
      "environment": [
        {
          "name": "ECS_TST_LOG_FORMAT",
          "value": "json"
        },
        {
          "name": "ECS_TST_INITIAL_WAIT_TIME",
          "value": "30m"
        },
        {
          "name": "ECS_TST_IDLE_TIMEOUT",
          "value": "5m"
        },
        {
          "name": "ECS_TST_MAX_LIFE_TIME",
          "value": "24h"
        },
        {
          "name": "ECS_TST_STOP_TASK",
          "value": "true"
        }
      ],
      "name": "ecs-task-self-terminator"
    }
  ],
  "executionRoleArn": "arn:aws:iam::123456789012:role/ecsTaskExecutionRole",
  "family": "gate",
  "cpu": "256",
  "memory": "512",
  "networkMode": "awsvpc",
  "requiresCompatibilities": [
    "FARGATE"
  ],
  "taskRoleArn": "arn:aws:iam::123456789012:role/ecsTaskRole"
}
```

3. Start the ECS Task with the ecs-task-self-terminator application.

The application will wait for the first connection for 30 minutes after the task starts. If there is no connection for 5 minutes, the application will automatically terminate the ECS Task. The application will automatically terminate the ECS Task after a maximum of 24 hours.
Please adjust these settings according to your use case.

## Custom Container Image

```Dockerfile
FROM ghcr.io/mashiike/ecs-task-self-terminator:latest AS ecs-task-self-terminator

FROM alpine:3.14.2
COPY --from=ecs-task-self-terminator /usr/local/bin/ecs-task-self-terminator /usr/local/bin/ecs-task-self-terminator
# Your application
ENTRYPOINT ["/usr/local/bin/ecs-task-self-terminator"]
```

## License

MIT License
