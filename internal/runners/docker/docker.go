package docker

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-units"
	"leil.io/sfstests/internal/utils"
)

const (
	imageName   = "saunafs-test"
	corePattern = "/tmp/temp-cores/core-%e-%p-%t"
)

type DockerRunner struct {
	config DockerConfig
	ctx context.Context
	options utils.TestOptions
}

func (runner DockerRunner) Setup(options utils.TestOptions, ctx context.Context) {
	runner.config = setupDockerConfig(options)
	runner.ctx = ctx

	if options.SetCorePattern {
	originalBytes, err := os.ReadFile("/proc/sys/kernel/core_pattern")
	utils.PanicIfErr(err)
	runner.config.originalCorePattern = string(originalBytes)
	setCorePattern(corePattern, ctx, runner.config.dockerClient)
	// defer setCorePattern(runner.config.originalCorePattern, ctx, runner.config.dockerClient)
	}
	
}

func (runner DockerRunner) RunTest(suite string, name string) (bool, string) {
	var output bytes.Buffer

	containerName := fmt.Sprintf("saunafs-%s-%s", suite, name)
	setupContainer(containerName, runner.config.dockerClient, runner.ctx)
	log.Println("Running test: " + suite + "/" + name + " in container " + containerName)

	filter := fmt.Sprintf("%s.%s", suite, name)
	gtestFilter := fmt.Sprintf("--gtest_filter=%s", filter)
	cmd := "touch /var/log/syslog; chown syslog:syslog /var/log/syslog; rsyslogd; saunafs-tests " + gtestFilter

	execConfig := types.ExecConfig{
		Cmd:          []string{"bash", "-c", cmd},
		AttachStdout: true,
		AttachStderr: true,
		Env:          runner.config.testContainerEnvs,
	}
	resp, err := runner.config.dockerClient.ContainerExecCreate(runner.ctx, containerName, execConfig)
	if errdefs.IsCancelled(err) {
		return false, ""
	}
	utils.PanicIfErr(err)

	attachResp, err := runner.config.dockerClient.ContainerExecAttach(runner.ctx, resp.ID, types.ExecStartCheck{})
	if errdefs.IsCancelled(err) {
		return false, ""
	}
	utils.PanicIfErr(err)
	_, err = stdcopy.StdCopy(&output, &output, attachResp.Reader)
	if errdefs.IsCancelled(err) {
		return false, ""
	}
	utils.PanicIfErr(err)

	var success bool = false;
	if bytes.Contains(output.Bytes(), []byte("1 FAILED TEST")) {
		log.Printf("Test %s finished: FAILED", name)
		success = false
		if runner.options.DeleteContainers {
			cleanContainer(runner.ctx, runner.config.dockerClient, containerName)
		}
	} else {
		log.Printf("Test %s finished: OK", name)
		success = true
		cleanContainer(runner.ctx, runner.config.dockerClient, containerName)
	}
	return success, output.String()

}
func (runner DockerRunner) GetTests(name string) (tests map[string][]string) {

}

func getDefaultHostConfig(options utils.TestOptions) container.HostConfig {
	return container.HostConfig{
		Resources: container.Resources{
			Devices: []container.DeviceMapping{
				{
					PathOnHost:        "/dev/fuse",
					PathInContainer:   "/dev/fuse",
					CgroupPermissions: "rwm",
				},
			},
			CpusetCpus: os.Getenv("NUMA_CPUSET"),
			CpusetMems: os.Getenv("NUMA_MEM_NODE"),
			Ulimits: []*units.Ulimit{
				{
					Name: "core",
					Soft: -1,
					Hard: -1,
				},
			},
			CPUCount: int64(options.CpuLimit),
		},
		Tmpfs: map[string]string{
			"/mnt/ramdisk": "rw,mode=1777,size=2g",
		},
		Privileged: true,
		CapAdd:     []string{"SYS_ADMIN", "SYS_PTRACE", "NET_ADMIN"},
	}

}

type Test struct {
	Name          string
	TestSuite     string
	Ctx           context.Context
	Config        DockerConfig
	ContainerName string
	Success       bool
	TestOutput    []byte
}

type DockerConfig struct {
	containerConfig     container.HostConfig
	testContainerEnvs   []string
	dockerClient        *client.Client
	originalCorePattern string
}

func (cfg *DockerConfig) cleanUp() {
	cfg.dockerClient.Close()

}

func setupDockerConfig(options utils.TestOptions) DockerConfig {
	var config DockerConfig
	var err error

	if options.CI {
		config.containerConfig.Privileged = false
	}
	config.containerConfig = setupMounts(options, config.containerConfig)
	config.dockerClient, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	utils.PanicIfErr(err)
	config.testContainerEnvs = setupTestEnvVariables(options)

	return config
}

func setupMounts(options utils.TestOptions, config container.HostConfig) container.HostConfig {
	if options.MountPoint != "" {
		config.Mounts = append(config.Mounts, mount.Mount{
			Type: mount.TypeBind, Source: options.MountPoint, Target: "/saunafs",
		})
	}
	if options.CoreMount != "" {
		config.Mounts = append(config.Mounts, mount.Mount{
			Type: mount.TypeBind, Source: options.CoreMount, Target: filepath.Dir(corePattern),
		})
	}
	if options.AuthFile != "" {
		config.Mounts = append(config.Mounts, mount.Mount{
			Type: mount.TypeBind, Source: options.AuthFile, Target: "/etc/apt/auth.conf.d/",
		})
	}
	return config
}

// Setup environment variables to use in the container
func setupTestEnvVariables(options utils.TestOptions) []string {
	var envVariables []string
	if (options.SetCorePattern) {
		envVariables = append(envVariables, "COREDUMP_WATCH=1")
	}
	multiplerVar := fmt.Sprintf("SAUNAFS_TEST_TIMEOUT_MULTIPLIER=%d", options.Multiplier)
	envVariables = append(envVariables, multiplerVar)
	return envVariables
}

// Print results of tests and return 1 if at least one test failed, otherwise 0
func printTestResults(tests []*Test, options utils.TestOptions) int {
	exitCode := 0
	for _, test := range tests {
		if test.Success {
			fmt.Printf("TEST %s: OK\n", test.Name)
			if options.AllOutput {
				fmt.Printf("- %s OUTPUT -\n", test.Name)
				fmt.Printf("%s\n", string(test.TestOutput))
				fmt.Printf("- END OUTPUT -\n")
			}
		}
	}
	// Go through the tests twice, to keep things ordered.
	for _, test := range tests {
		if !test.Success {
			fmt.Printf("TEST %s: FAILED\n", test.Name)
			fmt.Printf("FAILED IN CONTAINER %s\n", test.ContainerName)
			fmt.Printf("- %s OUTPUT -\n", test.Name)
			fmt.Printf("%s\n", string(test.TestOutput))
			fmt.Printf("- END OUTPUT -\n")
			exitCode = 1
		}
	}
	return exitCode
}

func setupContainer(name string, client *client.Client, ctx context.Context) {
	cont, err := client.ContainerInspect(ctx, name)
	if err == nil {
		log.Printf("Container %s already exists, removing it...\n", name)
		cleanContainer(ctx, client, cont.Name)
	} else if errdefs.IsNotFound(err) {
		log.Printf("Container %s does not exist, creating it...\n", name)
	} else if errdefs.IsCancelled(err) {
		return
	} else {
		utils.PanicIfErr(err)
	}

	resp, err := client.ContainerCreate(
		ctx,
		&container.Config{
			Image: imageName,
			Cmd:   []string{"sleep", "infinity"},
			Tty:   true,
			Env:   []string{"TERM=xterm" /* , "DEBUG=1", "DEBUG_LEVEL=5" */},
		},
		&container.HostConfig{},
		nil,
		nil,
		name,
	)
	if errdefs.IsCancelled(err) {
		return
	}
	utils.PanicIfErr(err)

	err = client.ContainerStart(ctx, resp.ID, container.StartOptions{})
	if errdefs.IsCancelled(err) {
		return
	}
	utils.PanicIfErr(err)

	for {
		container, err := client.ContainerInspect(ctx, resp.ID)
		if errdefs.IsCancelled(err) {
			return
		}
		utils.PanicIfErr(err)

		if container.State.Running {
			break
		}

		log.Printf("Waiting for container %s to start...\n", name)
		time.Sleep(1 * time.Second)
	}
}

func cleanContainer(ctx context.Context, client *client.Client, name string) {
	err := client.ContainerStop(ctx, name, container.StopOptions{Signal: "SIGKILL"})
	if errdefs.IsCancelled(err) {
		return
	}
	utils.PanicIfErr(err)

	err = client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if errdefs.IsCancelled(err) {
		return
	}
	utils.PanicIfErr(err)
}

// Funny how this works on a """Recommended""" docker setup without requiring
// root privileges
func setCorePattern(original string, ctx context.Context, client *client.Client) {
	const name string = "sfs-tmp"
	setupContainer(name, client, ctx)
	defer cleanContainer(ctx, client, "sfs-tmp")

	execConfig := types.ExecConfig{
		Cmd:          []string{"bash", "-c", "echo \"" + original + "\" > /proc/sys/kernel/core_pattern"},
		AttachStdout: true,
		AttachStderr: true,
	}
	resp, err := client.ContainerExecCreate(ctx, name, execConfig)
	utils.PanicIfErr(err)
	respAttach, err := client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{})

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, respAttach.Reader)
	utils.PanicIfErr(err)
}

func getSuites(ctx context.Context, client *client.Client, containerName string) map[string][]string {
	suites := map[string][]string{}
	execConfig := types.ExecConfig{
		Cmd:          []string{"bash", "-c", "ls -1 /saunafs/tests/test_suites/"},
		AttachStdout: true,
		AttachStderr: true,
	}
	resp, err := client.ContainerExecCreate(ctx, containerName, execConfig)
	utils.PanicIfErr(err)
	respAttach, err := client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{})

	var stdout bytes.Buffer

	_, err = stdcopy.StdCopy(&stdout, os.Stderr, respAttach.Reader)
	utils.PanicIfErr(err)
	filesLines := bytes.Split(stdout.Bytes(), []byte("\n"))
	for _, bytesLine := range filesLines {
		line := filepath.Base(string(bytesLine))
		if len(line) < 2 {
			continue
		}
		suite, _ := strings.CutSuffix(line, filepath.Ext(line))
		suites[suite] = make([]string, 0, 200)
	}
	return suites
}

func getTestGlobs(ctx context.Context, client *client.Client, options utils.TestOptions) map[string][]string {
	const name string = "sfs-tmp"
	setupContainer(name, client, ctx)
	defer cleanContainer(ctx, client, name)

	suites := getSuites(ctx, client, name)
	for suite := range suites {
		execConfig := types.ExecConfig{
			Cmd:          []string{"bash", "-c", "ls -1 /saunafs/tests/test_suites/" + suite + "/" + options.TestPattern + ".sh"},
			AttachStdout: true,
			AttachStderr: false,
		}
		resp, err := client.ContainerExecCreate(ctx, name, execConfig)
		utils.PanicIfErr(err)
		respAttach, err := client.ContainerExecAttach(ctx, resp.ID, types.ExecStartCheck{})

		var stdout bytes.Buffer

		_, err = stdcopy.StdCopy(&stdout, os.Stderr, respAttach.Reader)
		utils.PanicIfErr(err)

		filesLines := bytes.Split(stdout.Bytes(), []byte("\n"))
		for _, bytesLine := range filesLines {
			line := filepath.Base(string(bytesLine))
			if len(line) < 2 {
				continue
			}
			test, _ := strings.CutSuffix(line, filepath.Ext(line))
			suites[suite] = append(suites[suite], test)
		}
	}
	return suites
}
