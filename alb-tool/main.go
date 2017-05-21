package main

import (
	"fmt"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifttt/alb-tool/alb"
)

func main() {
	var err error

	arn := flag.String("arn", "", "the arn of the load balancer")
	port := flag.Int64("port", 0, "the port to register with the alb")
	maxWait := flag.Int64("maxWait", 30, "how long to wait for the service to become healthy")
	checkHealth := flag.Bool("checkHealth", false, "check health before registering with the alb")

	flag.Parse()

	alb, err := alb.New(*arn, *port)

	if err != nil {
		panic(err)
	}

	if *checkHealth {
		healhy, err := alb.CheckHealth(time.Duration(*maxWait)*time.Second)

		if err != nil {
			panic(err)
		}

		if healhy {
			fmt.Printf("Instance healthy on port %d\n", *port)
		} else {
			fmt.Printf("Instance unhealthy on port %d\n", *port)
			os.Exit(1)
		}
	}

	err = alb.Register()

	if err != nil {
		alb.Deregister()
		panic(err)
	}

	fmt.Printf("Instance registered on port %d\n", *port)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	err = alb.Deregister()

	if err != nil {
		panic(err)
	}

	fmt.Println("Instance draining")
}
