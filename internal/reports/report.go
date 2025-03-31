package reports

import "time"

type TestResult int

const (
	TestSuccess TestResult = iota
	TestCancelled
	TestFailed
)

type TestReport struct {
	Result    TestResult
	TestName  string
	Time      time.Duration
	AllOutput []byte
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
