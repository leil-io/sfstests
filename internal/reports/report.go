package reports

import (
	"fmt"
	"time"
)

type TestResult int

const (
	TestSuccess TestResult = iota
	TestCancelled
	TestFailed
)

type TestRunReport struct {
	Result    TestResult
	TestName  string
	Time      time.Duration
	AllOutput []byte
	// This may be null, if no LastFailure
	// Maybe...
	// StdOuput []byte
	// StdErr []byte
}

type TestReport struct {
	Name string
	Runs []TestRunReport
}

type SuiteReport struct {
	SuiteName string
	// Each test may have additional reports, if flakes is set
	TestReports []TestReport
}

type RunReport struct {
	SuiteReports []SuiteReport
}

// Returns true if there is at least one failure and one success in the runs
func (testReport *TestReport) IsFlaky() bool {
	failedOnce := false
	passedOnce := false
	for _, test := range testReport.Runs {
		if test.Result == TestSuccess {
			passedOnce = true
		} else if test.Result == TestFailed {
			failedOnce = true
		}
	}
	return failedOnce && passedOnce
}

func (testReport *TestReport) OKResults(printAll bool) string {
	strOutput := ""

	for i, test := range testReport.Runs {
		if test.Result == TestSuccess {
			if len(testReport.Runs) == 1 {
				strOutput += fmt.Sprintf("TEST %s: OK (%s)\n", test.TestName, test.Time.Round(time.Millisecond))
			} else {
				strOutput += fmt.Sprintf("TEST %s (%v/%v): OK (FLAKY) (%s)\n",
					test.TestName,
					i+1,
					len(testReport.Runs),
					test.Time.Round(time.Millisecond),
				)
			}
			if printAll {
				strOutput += fmt.Sprintf("- %s OUTPUT -\n", test.TestName)
				strOutput += fmt.Sprintf("%s\n", string(test.AllOutput))
				strOutput += fmt.Sprintf("- END OUTPUT -\n")
			}
		}
	}
	return strOutput
}

// Returns true if any test failed
func (testReport *TestReport) FailedResults(printAll bool) (string, bool) {
	strOutput := ""
	anyFailed := false

	for i, test := range testReport.Runs {
		if test.Result == TestFailed {
			if len(testReport.Runs) == 1 {
				strOutput += fmt.Sprintf("TEST %s: FAILED (%s)\n", test.TestName, test.Time.Round(time.Millisecond))
			} else {
				strOutput += fmt.Sprintf("TEST %s (%v/%v): FAILED (%s)\n",
					test.TestName,
					i+1,
					len(testReport.Runs),
					test.Time.Round(time.Millisecond),
				)
			}
			if printAll {
				strOutput += fmt.Sprintf("TEST %s: FAILED\n", test.TestName)
				strOutput += fmt.Sprintf("- %s OUTPUT -\n", test.TestName)
				strOutput += fmt.Sprintf("%s\n", string(test.AllOutput))
				strOutput += fmt.Sprintf("- END OUTPUT -\n")
			}
			anyFailed = true
		}
	}
	return strOutput, anyFailed
}
