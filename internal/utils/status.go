package utils

type TestResult int

const (
	TestSuccess TestResult = iota
	TestCancelled
	TestFailed
)
