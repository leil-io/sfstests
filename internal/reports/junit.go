package reports

import (
	"encoding/xml"
	"io"
	"strconv"
	"time"

	"leil.io/sfstests/internal/utils"
)

type jUnitFlakyFailure struct {
	StackTrace string `xml:"stackTrace"`
}

type jUnitTestReport struct {
	Name      string            `xml:"name,attr"`
	SuiteName string            `xml:"classname,attr"`
	Time      string            `xml:"time,attr"`
	Failure   string            `xml:"failure,omitempty"`
	Skipped   *xml.Name         `xml:"skipped,omitempty"`
	Flaky     *jUnitFlakyFailure `xml:"flakyFailure,omitempty"`
}
type jUnitSuiteReport struct {
	Name        string            `xml:"name,attr"`
	Time        string            `xml:"time,attr"`
	TestReports []jUnitTestReport `xml:"testcase,omitempty"`
}
type jUnitFullReport struct {
	XMLName    xml.Name           `xml:"testsuites"`
	Time       string             `xml:"time,attr"`
	TestSuites []jUnitSuiteReport `xml:"testsuite"`
}

func calculateSuiteTime(suiteReport SuiteReport) time.Duration {
	var suiteTime time.Duration = 0
	for _, test := range suiteReport.TestReports {
		suiteTime += test.Time
	}
	return suiteTime
}

func calculateFullTime(report RunReport) time.Duration {
	var runTime time.Duration = 0
	for _, suite := range report.SuiteReports {
		runTime += calculateSuiteTime(suite)
	}
	return runTime
}

func WriteXMLReport(writer io.Writer, report RunReport) {
	_, err := writer.Write(generateXMLReport(report))
	utils.PanicIfErr(err)
}

func generateXMLReport(report RunReport) []byte {
	var jUnitReport jUnitFullReport
	for _, suite := range report.SuiteReports {
		var jUnitSuite jUnitSuiteReport
		jUnitSuite.Name = suite.SuiteName
		jUnitSuite.Time = strconv.FormatFloat(calculateSuiteTime(suite).Seconds(), 'f', -1, 64)
		for _, test := range suite.TestReports {
			var jUnitTest jUnitTestReport
			jUnitTest.Name = test.TestName
			jUnitTest.Time = strconv.FormatFloat(test.Time.Seconds(), 'f', -1, 64)
			jUnitTest.SuiteName = suite.SuiteName
			if test.Result == TestFailed {
				jUnitTest.Failure = string(test.AllOutput)
			} else if test.Result == TestFlaky {
				jFlaky := new(jUnitFlakyFailure)
				jFlaky.StackTrace = string(test.LastFailureOutput)
				jUnitTest.Flaky = jFlaky
			} else if test.Result == TestCancelled {
				jUnitTest.Skipped = new(xml.Name)
			}
			jUnitSuite.TestReports = append(jUnitSuite.TestReports, jUnitTest)
		}
		jUnitReport.TestSuites = append(jUnitReport.TestSuites, jUnitSuite)
	}
	jUnitReport.Time = strconv.FormatFloat(calculateFullTime(report).Seconds(), 'f', -1, 64)
	output, err := xml.MarshalIndent(jUnitReport, " ", "  ")
	utils.PanicIfErr(err)
	return append([]byte(xml.Header), output...)
}
