package reports

import "time"

type TestResult int

const (
	TestSuccess TestResult = iota
	TestCancelled
	TestFailed
	TestFlaky
)

type TestReport struct {
	Result    TestResult
	TestName  string
	Time      time.Duration
	AllOutput []byte
	// This may be null, if no LastFailure
	LastFailureOutput []byte
	// Maybe...
	// StdOuput []byte
	// StdErr []byte
}

type SuiteReport struct {
	SuiteName   string
	TestReports []TestReport
}

type RunReport struct {
	SuiteReports []SuiteReport
}
