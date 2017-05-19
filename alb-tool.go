package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"net/http"
	"time"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elbv2"
)

func register(svc *elbv2.ELBV2, arn string, instanceID string, port int64) (*elbv2.RegisterTargetsOutput, error) {
	params := &elbv2.RegisterTargetsInput{
		TargetGroupArn: aws.String(arn),
		Targets: []*elbv2.TargetDescription{
			{
				Id:   aws.String(instanceID),
				Port: aws.Int64(port),
			},
		},
	}

	return svc.RegisterTargets(params)
}

func deregister(svc *elbv2.ELBV2, arn string, instanceID string, port int64) (*elbv2.DeregisterTargetsOutput, error) {
	params := &elbv2.DeregisterTargetsInput{
		TargetGroupArn: aws.String(arn),
		Targets: []*elbv2.TargetDescription{
			{
				Id:   aws.String(instanceID),
				Port: aws.Int64(port),
			},
		},
	}

	return svc.DeregisterTargets(params)
}

// TODO(nleach): This will only work for ALBs that have a single HttpCode
// configured for a "healthy" check.
func healthStatus(svc *elbv2.ELBV2, arn string, localIP string, port int64, maxWait time.Duration) (bool, error) {
	params := &elbv2.DescribeTargetGroupsInput{
    TargetGroupArns: []*string{
        aws.String(arn),
    },
	}
	resp, err := svc.DescribeTargetGroups(params)

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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d%s", localIP, port, path))

			if err != nil {
				return false, err
			}

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

func main() {
	var err error

	arn := flag.String("arn", "", "the arn of the load balancer")
	port := flag.Int64("port", 0, "the port to register with the alb")
	maxWait := flag.Int64("maxWait", 30, "how long to wait for the service to become healthy")
	checkHealth := flag.Bool("checkHealth", false, "check health before registering with the alb")

	flag.Parse()

	session := session.New(&aws.Config{Region: aws.String("us-east-1")})
	svc := elbv2.New(session)
	metadata := ec2metadata.New(session)

	instanceID, err := metadata.GetMetadata("instance-id")

	if err != nil {
		panic(err)
	}

	localIP, err := metadata.GetMetadata("local-ipv4")

	if err != nil {
		panic(err)
	}

	if *checkHealth {
		healhy, err := healthStatus(svc, *arn, localIP, *port, time.Duration(*maxWait) * time.Second)

		if err != nil {
			panic(err)
		}

		if healhy {
			fmt.Printf("Instance %s healthy on port %d\n", instanceID, *port)
		} else {
			fmt.Printf("Instance %s unhealthy on port %d\n", instanceID, *port)
			os.Exit(1)
		}
	}

	_, err = register(svc, *arn, instanceID, *port)

	if err != nil {
		deregister(svc, *arn, instanceID, *port)
		panic(err)
	}

	fmt.Printf("Instance %s registered on port %d\n", instanceID, *port)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	_, err = deregister(svc, *arn, instanceID, *port)

	if err != nil {
		panic(err)
	}

	fmt.Printf("Instance %s draining\n", instanceID)
}
