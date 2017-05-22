package alb

import (
  "fmt"
  "net/http"
  "strconv"
  "time"

  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/aws/ec2metadata"
  "github.com/aws/aws-sdk-go/aws/session"
  "github.com/aws/aws-sdk-go/service/elbv2"
)

type Alb struct {
  arn string
  instanceID string
  localIP string
  port int64
  svc *elbv2.ELBV2
}

func New(arn string, port int64) (*Alb, error) {
  session := session.Must(session.NewSession())

  svc := elbv2.New(session)
  metadata := ec2metadata.New(session)

  instanceID, err := metadata.GetMetadata("instance-id")

  if err != nil {
    return nil, err
  }

  localIP, err := metadata.GetMetadata("local-ipv4")

  if err != nil {
    return nil, err
  }

  return &Alb {
    arn: arn,
    instanceID: instanceID,
    localIP: localIP,
    port: port,
    svc: svc,
  }, nil
}

func (a Alb) Register() error {
  params := &elbv2.RegisterTargetsInput{
    TargetGroupArn: aws.String(a.arn),
    Targets: []*elbv2.TargetDescription{
      {
        Id:   aws.String(a.instanceID),
        Port: aws.Int64(a.port),
      },
    },
  }

  _, err := a.svc.RegisterTargets(params)

  return err
}

func (a Alb) Deregister() error {
  params := &elbv2.DeregisterTargetsInput{
    TargetGroupArn: aws.String(a.arn),
    Targets: []*elbv2.TargetDescription{
      {
        Id:   aws.String(a.instanceID),
        Port: aws.Int64(a.port),
      },
    },
  }

  _, err := a.svc.DeregisterTargets(params)

  return err
}

func (a Alb) CheckHealth(maxWait time.Duration) (healthy bool, err error) {
  params := &elbv2.DescribeTargetGroupsInput{
    TargetGroupArns: []*string{
      aws.String(a.arn),
    },
  }
  resp, err := a.svc.DescribeTargetGroups(params)

  if err != nil {
    return false, err
  }

  // TODO(nleach): This could be handled better asyncronously
  for _, targetGroup := range resp.TargetGroups {
    path := *targetGroup.HealthCheckPath
    statusCode, err := strconv.Atoi(*targetGroup.Matcher.HttpCode)

    if err != nil {
      return false, err
    }

    start := time.Now()

    for {
      resp, _ := http.Get(fmt.Sprintf("http://%s:%d%s", a.localIP, a.port, path))

      if resp.StatusCode == statusCode {
        break
      }

      if time.Since(start) > maxWait {
        return false, nil
      }

      time.Sleep(100 * time.Millisecond)
    }
  }

  return true, nil
}
