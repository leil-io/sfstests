package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"leil.io/sfstests/internal/runner"
	"leil.io/sfstests/internal/runners/docker"
	"leil.io/sfstests/internal/utils"
)

const (
	corePattern = "/tmp/temp-cores/core-%e-%p-%t"
)

type Test struct {
	Name          string
	TestSuite     string
	Ctx           context.Context
	Runner        runner.Runner
	Success       bool
	TestOutput    string
}

type Config struct {
	containerConfig     container.HostConfig
	testContainerEnvs   []string
	dockerClient        *client.Client
	originalCorePattern string
}

func runTests(ctx context.Context, options utils.TestOptions, runner runner.Runner) int {
	runner.Setup(options, ctx)
	testNames := runner.GetTests(options.TestPattern)
	suite, ok := testNames[options.Suite]
	if !ok {
		fmt.Fprintf(os.Stderr, "Test suite %s not found, these suites are available:\n", options.Suite)
		for key := range testNames {
			fmt.Fprintf(os.Stderr, "%s\n", key)
		}
		return 3
	}

	jobs := make(chan *Test, len(suite))
	tests := make([]*Test, 0, len(suite))

	var wg sync.WaitGroup

	for range options.Workers {
		wg.Add(1)
		go runTest(jobs, &wg, options)
	}

	for _, testName := range suite {
		if options.AuthFile == "" && strings.Contains(testName, "test_upgrade") {
			log.Printf("Skipping test %s", testName)
			continue
		}
		test := &Test{
			Name:          testName,
			TestSuite:     options.Suite,
			Ctx:           ctx,
			Runner:        runner,
		}
		jobs <- test
		tests = append(tests, test)
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return 2
	}
	log.Println("All tests finished")

	return printTestResults(tests, options)
}

// Print results of tests and return 1 if at least one test failed, otherwise 0
func printTestResults(tests []*Test, options utils.TestOptions) int {
	exitCode := 0
	for _, test := range tests {
		if test.Success {
			fmt.Printf("TEST %s: OK\n", test.Name)
			if options.AllOutput {
				fmt.Printf("- %s OUTPUT -\n", test.Name)
				fmt.Printf("%s\n", string(test.TestOutput))
				fmt.Printf("- END OUTPUT -\n")
			}
		}
	}
	// Go through the tests twice, to keep things ordered.
	for _, test := range tests {
		if !test.Success {
			fmt.Printf("TEST %s: FAILED\n", test.Name)
			fmt.Printf("- %s OUTPUT -\n", test.Name)
			fmt.Printf("%s\n", string(test.TestOutput))
			fmt.Printf("- END OUTPUT -\n")
			exitCode = 1
		}
	}
	return exitCode
}

func runTest(jobs <-chan *Test, wg *sync.WaitGroup, options utils.TestOptions) {
	defer wg.Done()

	for job := range jobs {
		success, output := job.Runner.RunTest(job.TestSuite, job.Name)
		if !success {
			log.Printf("Test %s finished: FAILED", job.Name)
			job.Success = false
			job.TestOutput = output
		} else {
			log.Printf("Test %s finished: OK", job.Name)
			job.Success = true
			if options.AllOutput {
				job.TestOutput = output
			}
		}
	}
}

func main() {
	var options utils.TestOptions
	options.SetupFromFlags()
	// Signal handling, so we cleanup properly
	ctx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	go func() {
		<-interrupt
		log.Printf("\nStopping tests, please wait...\n")
		log.SetOutput(io.Discard)
		cancel()
	}()
	var runner docker.DockerRunner
	os.Exit(runTests(ctx, options, runner))
}
