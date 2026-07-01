package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"leil.io/leil-tests/internal/reports"
	"leil.io/leil-tests/internal/runners/docker"
	"leil.io/leil-tests/internal/utils"
)

type Test struct {
	Name      string
	TestSuite string
	Ctx       context.Context
	CancelRun context.CancelFunc
	Runner    Runner
	Runs   []reports.TestRunReport
}

type Config struct {
	containerConfig     container.HostConfig
	testContainerEnvs   []string
	dockerClient        *client.Client
	originalCorePattern string
}

type Runner interface {
	Setup(options utils.TestOptions, ctx context.Context)
	RunTest(suite string, name string, ctx context.Context) reports.TestRunReport
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

	if (options.SkipTestsOnFail && options.XMLPath == "") && errors.Is(ctx.Err(), context.Canceled) {
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
		testReport := reports.TestReport{}
		testReport.Name = test.Name
		if len(test.Runs) == 0 {
			testReport.Runs = append(
				testReport.Runs,
				reports.TestRunReport{
					TestName: test.Name,
					Result:   reports.TestCancelled,
				},
			)
		} else {
			for _, run := range test.Runs {
				testReport.Runs = append(testReport.Runs, run)
			}
		}
		suiteRep.TestReports = append(suiteRep.TestReports, testReport)
	}
	runRep.SuiteReports = append(runRep.SuiteReports, suiteRep)
	return runRep
}

// Returns true on first success, and final amount of tries
// Should be called after a failure and if flakes is set
func testFlakiness(amountOfRetries int, job *Test) (bool, int) {
	for testTries := 2; testTries <= amountOfRetries; testTries++ {
		log.Printf("Rerunning test %s again (%v/%v)",
			job.Name,
			testTries,
			amountOfRetries,
			)
		report := job.Runner.RunTest(job.TestSuite, job.Name, job.Ctx)
		if report.Result == reports.TestSuccess {
			job.Runs = append(job.Runs, report)
			return true, testTries
		} else if report.Result == reports.TestFailed {
			log.Printf("Test %s FAILED AGAIN (%v/%v)",
				job.Name,
				testTries,
				amountOfRetries,
			)
		}
		job.Runs = append(job.Runs, report)
	}
	return false, amountOfRetries
}

func testWorker(jobs <-chan *Test, wg *sync.WaitGroup, options utils.TestOptions) {
	defer wg.Done()

	for job := range jobs {
		report := job.Runner.RunTest(job.TestSuite, job.Name, job.Ctx)
		job.Runs = append(job.Runs, report)
		if report.Result == reports.TestFailed {
			// TODO(Urmas): Clean this mess up
			if options.Flakes > 1 {
				log.Printf("Test %s failed initially, retrying to see if it's flaky",
					job.Name,
				)
				if flaky, tries := testFlakiness(options.Flakes, job); flaky {
					log.Printf("Test %s passed when it failed before, considered FLAKY (%v/%v)",
						job.Name,
						tries,
						options.Flakes,
					)
					continue
				}
			} else {
				log.Printf("Test %s finished: FAILED (%s)", job.Name, report.Time.Round(time.Millisecond))
			}
			if options.SkipTestsOnFail {
				log.SetOutput(io.Discard)
				job.CancelRun()
			}
		} else if report.Result == reports.TestSuccess {
			log.Printf("Test %s finished: OK (%s)", job.Name, report.Time.Round(time.Millisecond))
		}
	}
}

// Print results of tests and return 1 if at least one test failed, otherwise 0
func printTestResults(report reports.SuiteReport, options utils.TestOptions) int {
	exitCode := 0
	for _, test := range report.TestReports {
		// First OK tests
		output := test.OKResults(options.AllOutput)
		if options.AllOutput && options.Workers < 2 {
			continue
		}
		fmt.Println(output)
	}
	for _, test := range report.TestReports {
		// Then failed tests
		output, failed := test.FailedResults(!options.AllOutput || options.Workers > 2)
		if failed {
			exitCode = 2
		}
		if output != "" {
			fmt.Println(output)
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
