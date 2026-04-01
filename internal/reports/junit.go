package reports

import (
	"encoding/xml"
	"io"
	"strconv"
	"time"

	"leil.io/lfstests/internal/utils"
)


type jUnitTestReport struct {
	Name      string              `xml:"name,attr"`
	SuiteName string              `xml:"classname,attr"`
	Time      string              `xml:"time,attr"`
	Failed    string              `xml:"failure,omitempty"`
	Skipped   *xml.Name           `xml:"skipped,omitempty"`

	// The reason why this has two fields of the same type has to do with
	// getting it compatible with Maven's surefire flaky tests, which is
	// used by some plugins on Jenkins.

	// This is empty when there were flakes detected
	NotFlaky  []string `xml:"rerunFailure,omitempty"`
	// This is empty when there were no flakes detected
	Flaky     []string `xml:"flakyFailure,omitempty"`

	// Thanks a lot Maven for your consistency
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
		// Take the last time
		suiteTime += test.Runs[len(test.Runs) - 1].Time
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
			jUnitSuite.TestReports = append(jUnitSuite.TestReports, buildjUnitTestReport(test, suite.SuiteName))
		}
		jUnitReport.TestSuites = append(jUnitReport.TestSuites, jUnitSuite)
	}

	jUnitReport.Time = strconv.FormatFloat(calculateFullTime(report).Seconds(), 'f', -1, 64)
	output, err := xml.MarshalIndent(jUnitReport, " ", "  ")
	utils.PanicIfErr(err)
	return append([]byte(xml.Header), output...)
}

func buildjUnitTestReport(test TestReport, suite string) (jUnitTest jUnitTestReport) {
	flaky := test.IsFlaky()
	jUnitTest.Name = test.Name
	jUnitTest.SuiteName = suite

	if flaky {
		// Take last time
		jUnitTest.Time = strconv.FormatFloat(
			test.Runs[len(test.Runs)-1].Time.Seconds(),
			'f',
			-1,
			64,
		)
	} else {
		// Take first time
		jUnitTest.Time = strconv.FormatFloat(
			test.Runs[0].Time.Seconds(),
			'f',
			-1,
			64,
		)
	}

	if len(test.Runs) > 1 {
		for i, run := range test.Runs {
			if run.Result == TestCancelled {
				jUnitTest.Skipped = new(xml.Name)
				break
			} else if flaky && run.Result == TestFailed {
				jUnitTest.Flaky = append(
					jUnitTest.Flaky,
					string(run.AllOutput),
					)
			} else if !flaky && i == 0 {
				// First failed output must be a normal failure
				jUnitTest.Failed = string(run.AllOutput)
			} else if !flaky && i != 0 {
				jUnitTest.NotFlaky = append(
					jUnitTest.NotFlaky,
					string(run.AllOutput),
				)
			}
		}
	} else {
		run := test.Runs[0]
		if run.Result == TestFailed {
			jUnitTest.Failed = string(run.AllOutput)
		} else if run.Result == TestCancelled {
			jUnitTest.Skipped = new(xml.Name)
		}
	}
	return jUnitTest
}
