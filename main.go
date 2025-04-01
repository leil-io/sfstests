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
	"leil.io/sfstests/internal/reports"
	"leil.io/sfstests/internal/runners/docker"
	"leil.io/sfstests/internal/utils"
)

type Test struct {
	Name      string
	TestSuite string
	Ctx       context.Context
	CancelRun context.CancelFunc
	Runner    Runner
	Report    reports.TestReport
}

type Config struct {
	containerConfig     container.HostConfig
	testContainerEnvs   []string
	dockerClient        *client.Client
	originalCorePattern string
}

type Runner interface {
	Setup(options utils.TestOptions, ctx context.Context)
	RunTest(suite string, name string, ctx context.Context) reports.TestReport
	// 'name' must handle wildcard (*) pattern
	GetTests(name string, ctx context.Context) (tests map[string][]string)
	Cleanup(ctx context.Context)
}

func runTests(ctx context.Context, options utils.TestOptions, runner Runner, cancel context.CancelFunc) int {
	runner.Setup(options, ctx)
	testNames := runner.GetTests(options.TestPattern, ctx)
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

	log.Printf("Using %v workers\n", options.Workers)
	for range options.Workers {
		wg.Add(1)
		go testWorker(jobs, &wg, options)
	}

	for _, testName := range suite {
		if options.AuthFile == "" && strings.Contains(testName, "test_upgrade") {
			log.Printf("Skipping test %s", testName)
			continue
		}
		test := &Test{
			Name:      testName,
			TestSuite: options.Suite,
			Ctx:       ctx,
			CancelRun: cancel,
			Runner:    runner,
		}
		jobs <- test
		tests = append(tests, test)
	}
	close(jobs)
	wg.Wait()
	log.Println("All tests finished")
	runner.Cleanup(ctx)
	runRep := compileSuiteReport(tests, options)

	// TODO(Urmas): Currently only one suite can be run at a time.
	exitCode := printTestResults(runRep.SuiteReports[0], options)
	if options.XMLPath != "" {
		err := writeXMLReportToFile(options.XMLPath, runRep)
		if err != nil {
			exitCode = 3
		} else {
			// Jenkins will determine whether to fail based on the
			// XML file
			exitCode = 0
		}
	}


	if ctx.Err() != nil && options.SkipTestsOnFail {
		return 2
	} else {
		return exitCode
	}
}

func writeXMLReportToFile(path string, report reports.RunReport) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open file %s: %e", path, err)
		return err
	}
	defer file.Close()
	reports.WriteXMLReport(file, report)
	return nil
}

func compileSuiteReport(tests []*Test, options utils.TestOptions) reports.RunReport {
	runRep := reports.RunReport{}
	suiteRep := reports.SuiteReport{}
	suiteRep.SuiteName = options.Suite
	for _, test := range tests {
		suiteRep.TestReports = append(suiteRep.TestReports, test.Report)
	}
	runRep.SuiteReports = append(runRep.SuiteReports, suiteRep)
	return runRep
}

func testWorker(jobs <-chan *Test, wg *sync.WaitGroup, options utils.TestOptions) {
	defer wg.Done()

	for job := range jobs {
		job.Report = job.Runner.RunTest(job.TestSuite, job.Name, job.Ctx)
		if job.Report.Result == reports.TestFailed {
			log.Printf("Test %s finished: FAILED", job.Name)
			if options.SkipTestsOnFail {
				log.SetOutput(io.Discard)
				job.CancelRun()
			}
		} else if job.Report.Result == reports.TestSuccess {
			log.Printf("Test %s finished: OK", job.Name)
		}
	}
}

// Print results of tests and return 1 if at least one test failed, otherwise 0
func printTestResults(report reports.SuiteReport, options utils.TestOptions) int {
	exitCode := 0
	for _, test := range report.TestReports {
		if test.Result == reports.TestSuccess {
			fmt.Printf("TEST %s: OK\n", test.TestName)
			if options.Workers > 1 && options.AllOutput {
				fmt.Printf("- %s OUTPUT -\n", test.TestName)
				fmt.Printf("%s\n", string(test.AllOutput))
				fmt.Printf("- END OUTPUT -\n")
			}
		}
	}
	// Go through the tests twice, to keep things ordered.
	for _, test := range report.TestReports {
		if test.Result == reports.TestFailed {
			fmt.Printf("TEST %s: FAILED\n", test.TestName)
			if !options.AllOutput || options.Workers > 1 {
				fmt.Printf("- %s OUTPUT -\n", test.TestName)
				fmt.Printf("%s\n", string(test.AllOutput))
				fmt.Printf("- END OUTPUT -\n")
			}
			exitCode = 1
		}
	}
	return exitCode
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
	os.Exit(runTests(ctx, options, &runner, cancel))
}
