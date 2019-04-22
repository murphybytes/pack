package build_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/archive"
	"github.com/buildpack/pack/build"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

var (
	repoName  string
	dockerCli *client.Client
)

func TestLifecycle(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	color.NoColor = true

	h.RequireDocker(t)

	var err error
	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	h.AssertNil(t, err)

	repoName = "lifecycle.test." + h.RandString(10)
	CreateFakeLifecycleImage(t, dockerCli, repoName)
	defer h.DockerRmi(dockerCli, repoName)

	spec.Run(t, "lifecycle", testLifecycle, spec.Report(report.Terminal{}), spec.Parallel())
}

func testLifecycle(t *testing.T, when spec.G, it spec.S) {
	when("Phase", func() {
		var (
			tmpDir         string
			bpFetcher      *buildpack.Fetcher
			lifecycle      *build.Lifecycle
			outBuf, errBuf bytes.Buffer
			logger         *logging.Logger
		)

		it.Before(func() {
			var err error

			logger = logging.NewLogger(&outBuf, &errBuf, true, false)

			tmpDir, err = ioutil.TempDir("", "", )
			h.AssertNil(t, err)

			bpFetcher = buildpack.NewFetcher(logger, tmpDir)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			h.AssertNil(t, lifecycle.Cleanup())
		})

		when("there are no user provided buildpacks", func() {
			it.Before(func() {
				var err error
				lifecycle, err = build.NewLifecycle(
					build.LifecycleConfig{
						BuilderImage: repoName,
						AppDir:       filepath.Join("testdata", "fake-app"),
						Logger:       logger,
						Env: map[string]string{
							"some-key":  "some-val",
							"other-key": "other-val",
						},
						BPFetcher: bpFetcher,
					},
				)
				h.AssertNil(t, err)
			})

			when("#Run", func() {
				it("runs the lifecycle phase on the builder image", func() {
					phase, err := lifecycle.NewPhase("phase")
					h.AssertNil(t, err)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "running some-lifecycle-phase")
				})

				it("prefixes the output with the phase name", func() {
					phase, err := lifecycle.NewPhase("phase")
					h.AssertNil(t, err)
					assertRunSucceeds(t, phase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] running some-lifecycle-phase")
				})

				it("attaches the same layers volume to each phase", func() {
					writePhase, err := lifecycle.NewPhase("phase", build.WithArgs("write", "/layers/test.txt", "test-layers"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, writePhase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] write test")
					readPhase, err := lifecycle.NewPhase("phase", build.WithArgs("read", "/layers/test.txt"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] file contents: test-layers")
				})

				it("attaches the same app volume to each phase", func() {
					writePhase, err := lifecycle.NewPhase("phase", build.WithArgs("write", "/workspace/test.txt", "test-app"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, writePhase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] write test")
					readPhase, err := lifecycle.NewPhase("phase", build.WithArgs("read", "/workspace/test.txt"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] file contents: test-app")
				})

				it("copies the app into the app volume before the first phase", func() {
					readPhase, err := lifecycle.NewPhase("phase", build.WithArgs("read", "/workspace/fake-app-file"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, readPhase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] file contents: fake-app-contents")
					h.AssertContains(t, outBuf.String(), "[phase] file uid/gid 111/222")
					deletePhase, err := lifecycle.NewPhase("phase", build.WithArgs("delete", "/workspace/fake-app-file"))
					h.AssertNil(t, err)
					assertRunSucceeds(t, deletePhase, &outBuf, &errBuf)
					h.AssertContains(t, outBuf.String(), "[phase] delete test")
					readPhase2, err := lifecycle.NewPhase("phase", build.WithArgs("read", "/workspace/fake-app-file"))
					h.AssertNil(t, err)
					err = readPhase2.Run(context.TODO())
					readPhase2.Cleanup()
					h.AssertNotNil(t, err)
					h.AssertContains(t, outBuf.String(), "failed to read file")
				})

				when("#WithArgs", func() {
					it("runs the lifecycle phase with args", func() {
						phase, err := lifecycle.NewPhase("phase", build.WithArgs("some", "args"))
						h.AssertNil(t, err)
						assertRunSucceeds(t, phase, &outBuf, &errBuf)
						h.AssertContains(t, outBuf.String(), `received args [/lifecycle/phase some args]`)
					})
				})

				when("#WithDaemonAccess", func() {
					it("allows daemon access inside the container", func() {
						phase, err := lifecycle.NewPhase(
							"phase",
							build.WithArgs("daemon"),
							build.WithDaemonAccess(),
						)
						h.AssertNil(t, err)
						assertRunSucceeds(t, phase, &outBuf, &errBuf)
						h.AssertContains(t, outBuf.String(), "[phase] daemon test")
					})
				})

				when("#WithRegistryAccess", func() {
					var registry *h.TestRegistryConfig

					it.Before(func() {
						registry = h.RunRegistry(t, true)
					})

					it.After(func() {
						registry.StopRegistry(t)
					})

					it("provides auth for registry in the container", func() {
						phase, err := lifecycle.NewPhase(
							"phase",
							build.WithArgs("registry", registry.RepoName("packs/build:v3alpha2")),
							build.WithRegistryAccess(),
						)
						h.AssertNil(t, err)
						assertRunSucceeds(t, phase, &outBuf, &errBuf)
						h.AssertContains(t, outBuf.String(), "[phase] registry test")
					})
				})
			})
		})

		when("there are user provided buildpacks", func() {
			it.Before(func() {
				if runtime.GOOS == "windows" {
					t.Skip("directory buildpacks are not implemented on windows")
				}
				var err error
				lifecycle, err = build.NewLifecycle(
					build.LifecycleConfig{
						BuilderImage: repoName,
						Logger:       logger,
						Buildpacks: []string{
							filepath.Join("testdata", "fake_buildpack"),
							"just/buildpack.id@1.2.3",
						},
						Env: map[string]string{
							"some-key":  "some-val",
							"other-key": "other-val",
						},
						BPFetcher: bpFetcher,
					},
				)
				h.AssertNil(t, err)
			})

			it("runs the phase with custom buildpacks available", func() {
				phase, err := lifecycle.NewPhase("phase", build.WithArgs("buildpacks"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] buildpacks test")

				h.AssertContains(t, outBuf.String(), "[phase] /buildpacks/test.bp/0.0.1-test 111/222")
				h.AssertContains(t, outBuf.String(), "[phase] /buildpacks/test.bp/0.0.1-test/bin/build 111/222")
				h.AssertContains(t, outBuf.String(), "[phase] /buildpacks/test.bp/0.0.1-test/bin/detect 111/222")
			})

			it("runs the phase with custom order.toml available", func() {
				phase, err := lifecycle.NewPhase("phase", build.WithArgs("read", "/buildpacks/order.toml"))
				h.AssertNil(t, err)
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, outBuf.String(), "[phase] read test")
				assertRunSucceeds(t, phase, &outBuf, &errBuf)
				h.AssertContains(t, strings.Replace(outBuf.String(), "[phase]", "", -1),
					`
   [[groups.buildpacks]]
     id = "test.bp"
     version = "0.0.1-test"
`)
				h.AssertContains(t, strings.Replace(outBuf.String(), "[phase]", "", -1),
					`
   [[groups.buildpacks]]
     id = "just/buildpack.id"
     version = "1.2.3"
`)
			})
		})
	})

	when("#Cleanup", func() {
		var (
			subject        *build.Lifecycle
			outBuf, errBuf bytes.Buffer
		)

		it.Before(func() {
			var err error
			logger := logging.NewLogger(&outBuf, &errBuf, true, false)
			subject, err = build.NewLifecycle(build.LifecycleConfig{
				BuilderImage: repoName,
				AppDir:       filepath.Join("testdata", "fake-app"),
				Logger:       logger,
				Env:          map[string]string{},
			})
			h.AssertNil(t, err)

			phase, err := subject.NewPhase("phase")
			h.AssertNil(t, err)
			assertRunSucceeds(t, phase, &outBuf, &errBuf)
			h.AssertContains(t, outBuf.String(), "running some-lifecycle-phase")

			h.AssertNil(t, subject.Cleanup())
		})

		it("should delete the layers volume", func() {
			body, err := subject.Docker.VolumeList(context.TODO(),
				filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: subject.LayersVolume,
				}))
			h.AssertNil(t, err)
			h.AssertEq(t, len(body.Volumes), 0)
		})

		it("should delete the app volume", func() {
			body, err := subject.Docker.VolumeList(context.TODO(),
				filters.NewArgs(filters.KeyValuePair{
					Key:   "name",
					Value: subject.AppVolume,
				}))
			h.AssertNil(t, err)
			h.AssertEq(t, len(body.Volumes), 0)
		})

		it("should remove the builder image", func() {
			images, err := subject.Docker.ImageList(context.TODO(), dockertypes.ImageListOptions{})
			h.AssertNil(t, err)

			found := false
			for _, image := range images {
				for _, tag := range image.RepoTags {
					if strings.Contains(tag, subject.Builder.Name()) {
						found = true
						break
					}
				}
				if found == true {
					break
				}
			}
			h.AssertEq(t, found, false)
		})
	})
}

func assertRunSucceeds(t *testing.T, phase *build.Phase, outBuf *bytes.Buffer, errBuf *bytes.Buffer) {
	t.Helper()
	if err := phase.Run(context.TODO()); err != nil {
		phase.Cleanup()
		t.Fatalf("Failed to run phase '%s' \n stdout: '%s' \n stderr '%s'", err, outBuf.String(), errBuf.String())
	}
	phase.Cleanup()
}

func CreateFakeLifecycleImage(t *testing.T, dockerCli *client.Client, repoName string) {
	ctx := context.Background()

	wd, err := os.Getwd()
	h.AssertNil(t, err)
	buildContext, _ := archive.CreateTarReader(filepath.Join(wd, "testdata", "fake-lifecycle"), "/", 0, 0)

	res, err := dockerCli.ImageBuild(ctx, buildContext, dockertypes.ImageBuildOptions{
		Tags:        []string{repoName},
		Remove:      true,
		ForceRemove: true,
	})
	h.AssertNil(t, err)

	io.Copy(ioutil.Discard, res.Body)
	res.Body.Close()
}
