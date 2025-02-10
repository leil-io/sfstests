package utils

import (
	"flag"
	"runtime"
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
}

func (options *TestOptions) SetupFromFlags() {
	flag.StringVar(&options.Suite, "suite", "SanityChecks", "Test suite to run")
	flag.StringVar(&options.Suite, "s", "SanityChecks", "shorthand for -suite")

	flag.StringVar(&options.TestPattern, "test", "*", "Test pattern to run")
	flag.StringVar(&options.TestPattern, "t", "*", "shorthand for -test")

	flag.IntVar(&options.Workers, "w", runtime.NumCPU(), "shorthand for -workers")
	flag.IntVar(&options.Workers, "workers", runtime.NumCPU(), "Number of containers to run tests")

	flag.IntVar(&options.CpuLimit, "c", runtime.NumCPU(), "shorthand for -CPU's")
	flag.IntVar(&options.CpuLimit, "cpus", runtime.NumCPU(), "Limit number of CPU's per container")

	flag.IntVar(&options.Multiplier, "multiplier", 1, "Test timeout multiplier")

	flag.BoolVar(&options.AllOutput, "a", false, "shorthand for -all")
	flag.BoolVar(&options.AllOutput, "all", false, "Output all test results, including succeeding")

	flag.BoolVar(&options.AllOutput, "ci", false, "Run in CI mode (disable privileged modes and enhance security")

	flag.BoolVar(&options.DeleteContainers, "d", false, "shorthand for -d")
	flag.BoolVar(&options.DeleteContainers, "delete", false, "Delete all containers regardless if they failed or not")

	flag.StringVar(&options.MountPoint, "mount", "", "SaunaFS git repository to mount")
	flag.StringVar(&options.MountPoint, "m", "", "shorthand for -mount")

	flag.StringVar(&options.CoreMount, "core-mount", "", "Mount place for cores")
	flag.StringVar(&options.AuthFile, "auth", "", "APT auth full path for upgrade tests, otherwise upgrades are skipped")

	flag.BoolVar(&options.SetCorePattern, "setcore", false, "(EXPERIMENTAL): Manage the core pattern, note if the program is killed or otherwise forced to exit without cleaning up, you need to set it back yourself")

	flag.Parse()
}
