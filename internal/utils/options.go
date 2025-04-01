package utils

import (
	"flag"
	"log"
	"os"
	"runtime"
	"strings"
)

type TestOptions struct {
	Workers          int
	Suite            string
	Source           string
	TestPattern      string
	MountPoint       string
	CoreMount        string
	AllOutput        bool
	DeleteContainers bool
	Multiplier       int
	CpuLimit         int
	AuthFile         string
	SetCorePattern   bool
	CI               bool
	XMLPath          string
	SkipTestsOnFail  bool
}

var longToShort = map[string]string {
	"workers": "w",
	"test": "t",
	"cpus": "c",
	"all": "a",
	"delete": "d",
	"mount": "m",
}

func reverseMap(oldMap map[string]string) map[string]string {
    newMap := make(map[string]string, len(oldMap))
    for k, v := range oldMap {
        newMap[v] = k
    }
    return newMap
}

var shortToLong = reverseMap(longToShort)

func (options *TestOptions) SetupFromFlags() {
	flag.StringVar(&options.Suite, "suite", "SanityChecks", "Test suite to run")
	flag.StringVar(&options.Suite, "s", "SanityChecks", "shorthand for -suite")

	flag.StringVar(&options.TestPattern, "test", "*", "Test pattern to run")
	flag.StringVar(&options.TestPattern, "t", "*", "shorthand for -test")

	flag.IntVar(&options.Workers, longToShort["workers"], runtime.NumCPU(), "shorthand for -workers")
	flag.IntVar(&options.Workers, "workers", runtime.NumCPU(), "Number of containers to run tests")

	flag.IntVar(&options.CpuLimit, longToShort["cpus"], runtime.NumCPU(), "shorthand for -CPU's")
	flag.IntVar(&options.CpuLimit, "cpus", runtime.NumCPU(), "Limit number of CPU's per container")

	flag.IntVar(&options.Multiplier, "multiplier", 1, "Test timeout multiplier")

	flag.BoolVar(&options.AllOutput, longToShort["all"], false, "shorthand for -all")
	flag.BoolVar(&options.AllOutput, "all", false, "Output all test results, including succeeding")

	flag.BoolVar(&options.AllOutput, "ci", false, "Run in CI mode (disable privileged modes and enhance security)")

	flag.BoolVar(&options.DeleteContainers, longToShort["delete"], false, "shorthand for -d")
	flag.BoolVar(&options.DeleteContainers, "delete", false, "Delete all containers regardless if they failed or not")

	flag.StringVar(&options.MountPoint, longToShort["mount"], "", "shorthand for -mount")
	flag.StringVar(&options.MountPoint, "mount", "", "SaunaFS git repository to mount (must be full path)")

	flag.StringVar(&options.CoreMount, "core-mount", "", "Mount place for cores")
	flag.StringVar(&options.AuthFile, "auth", "", "APT auth full path for upgrade tests, otherwise upgrades are skipped")

	flag.BoolVar(&options.SetCorePattern, "setcore", false, "(EXPERIMENTAL): Manage the core pattern, note if the program is killed or otherwise forced to exit without cleaning up, you need to set it back yourself")
	flag.BoolVar(&options.SkipTestsOnFail, "skip-on-fail", false, "Skip remaining tests on a single fail")

	flag.StringVar(&options.XMLPath, "xml-path", "", "Filename path for JUnit test results from gtest")

	flag.Parse()

	setEnvVariables(flag.CommandLine)
}

// https://stackoverflow.com/a/52916005
func getUnsetFlags(fs *flag.FlagSet) []*flag.Flag {
	var unset []*flag.Flag
	fs.VisitAll(func(f *flag.Flag) {
		unset = append(unset, f)
	})
	fs.Visit(func(f *flag.Flag) {
		for i, h := range unset {
			if f == h {
				unset = append(unset[:i], unset[i+1:]...)
			}
		}
	})
	return unset
}

// Could be better optimized instead of visiting again, but that would require
// a custom structure, and for such few options, it's not worth complicating
// it.
func isShortSet(fs *flag.FlagSet, shortName string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == shortName {
			found = true
		}
	})
	return found
}

func flagNameToEnvVar(flagName string) string {
	envVar := []byte("SFSTESTS_" + strings.ToUpper(flagName))
	for i, c := range envVar {
		if c == '-' {
			envVar[i] = '_'
		}
	}
	return string(envVar)
}

// Set env variables for unset flags
func setEnvVariables(fs *flag.FlagSet) {
	for _, f := range getUnsetFlags(fs) {
		var envVar string
		if shortFlagName, ok := longToShort[f.Name]; ok {
			// Ignore if short option was set
			if isShortSet(fs, shortFlagName) {
				continue
			}
		} else if _, ok := shortToLong[f.Name]; ok {
			// Ignore short option env variables
			continue
		}
		envVar = flagNameToEnvVar(f.Name)

		v := os.Getenv(envVar)
		if v == "" {
			continue
		}
		err := f.Value.Set(v)
		if err != nil {
			log.Printf("Invalid environment variable '%s=%s'\n", envVar, v)
			log.Printf("Using default '%s=%s'\n", envVar, f.DefValue)
			err = f.Value.Set(f.DefValue)
			PanicIfErr(err)
		}
	}
}
