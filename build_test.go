package pack

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/buildpack"

	"github.com/buildpack/pack/builder"

	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/logging"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuild(t *testing.T) {
	color.NoColor = true
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "build", testBuild, spec.Report(report.Terminal{}))
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var (
		subject               *Client
		logOut, logErr        *bytes.Buffer
		clientConfig          *config.Config
		fakeImageFetcher      *h.FakeImageFetcher
		fakeLifecycle         *h.FakeLifecycle
		defaultBuilderStackID string
		defaultBuilderImage   *fakes.Image
		tmpDir                string
	)
	it.Before(func() {
		fakeImageFetcher = h.NewFakeImageFetcher()
		fakeLifecycle = &h.FakeLifecycle{}

		logOut, logErr = &bytes.Buffer{}, &bytes.Buffer{}
		clientConfig = &config.Config{
			DefaultBuilder: "example.com/default/builder:tag",
		}
		defaultBuilderStackID = "default.stack"
		defaultBuilderImage = h.NewFakeBuilderImage(t, clientConfig.DefaultBuilder, builder.Config{
			Stack: builder.StackConfig{
				ID:       defaultBuilderStackID,
				RunImage: "default/run",
				RunImageMirrors: []string{
					"registry1.example.com/run/mirror",
					"registry2.example.com/run/mirror",
				},
			},
		})
		fakeImageFetcher.LocalImages[defaultBuilderImage.Name()] = defaultBuilderImage

		fakeDefaultRunImage := fakes.NewImage(t, "index.docker.io/default/run:latest", "", "")
		h.AssertNil(t, fakeDefaultRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		fakeImageFetcher.LocalImages[fakeDefaultRunImage.Name()] = fakeDefaultRunImage

		fakeMirror1 := fakes.NewImage(t, "registry1.example.com/run/mirror:latest", "", "")
		h.AssertNil(t, fakeMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		fakeImageFetcher.LocalImages[fakeMirror1.Name()] = fakeMirror1

		fakeMirror2 := fakes.NewImage(t, "registry2.example.com/run/mirror:latest", "", "")
		h.AssertNil(t, fakeMirror2.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
		fakeImageFetcher.LocalImages[fakeMirror2.Name()] = fakeMirror2
		var err error
		tmpDir, err = ioutil.TempDir("", "build-test-bp-fetch-cache")
		h.AssertNil(t, err)

		subject = NewClient(
			clientConfig,
			logging.NewLogger(logOut, logErr, true, false),
			fakeImageFetcher,
			fakeLifecycle,
			buildpack.NewFetcher(logging.NewLogger(logOut, logErr, true, false), tmpDir),
		)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Build", func() {
		when("Image option", func() {
			it("is required", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image: "",
				}),
					"invalid image name ''",
				)
			})

			it("must be a valid image reference", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image: "not@valid",
				}),
					"invalid image name 'not@valid'",
				)
			})

			it("must be a valid tag reference", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image: "registry.com/my/image@sha256:954e1f01e80ce09d0887ff6ea10b13a812cb01932a0781d6b0cc23f743a874fd",
				}),
					"invalid image name 'registry.com/my/image@sha256:954e1f01e80ce09d0887ff6ea10b13a812cb01932a0781d6b0cc23f743a874fd'",
				)
			})

			it("lifecycle receives resolved reference", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image: "example.com/some/repo:tag",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ImageRef.Context().RegistryStr(), "example.com")
				h.AssertEq(t, fakeLifecycle.Opts.ImageRef.Context().RepositoryStr(), "some/repo")
				h.AssertEq(t, fakeLifecycle.Opts.ImageRef.Identifier(), "tag")
			})
		})

		when("AppDir option", func() {
			it("defaults to the current working directory", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image: "some/app",
				}))

				wd, err := os.Getwd()
				h.AssertNil(t, err)
				h.AssertEq(t, fakeLifecycle.Opts.AppDir, wd)
			})

			it("path must exist", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:  "some/app",
					AppDir: "not/exist/path",
				}),
					"invalid app dir 'not/exist/path'",
				)
			})

			it("path must be a dir", func() {
				h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
					Image:  "some/app",
					AppDir: filepath.Join("testdata", "just-a-file.txt"),
				}),
					fmt.Sprintf("invalid app dir '%s'", filepath.Join("testdata", "just-a-file.txt")),
				)
			})
		})

		when("Builder option", func() {
			when("the client has a default builder", func() {
				it("defaults to the client's default builder", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image: "some/app",
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
				})
			})

			when("the client doesn't have a default builder", func() {
				it.Before(func() {
					clientConfig.DefaultBuilder = ""
				})

				it("builder is required", func() {
					h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
						Image: "some/app",
					}),
						"invalid builder ''",
					)
				})
			})

			when("the builder name is provided", func() {
				var customBuilderImage *fakes.Image

				it.Before(func() {
					customBuilderImage = h.NewFakeBuilderImage(t, "index.docker.io/some/builder:latest", builder.Config{
						Stack: builder.StackConfig{ID: "some.stack.id", RunImage: "some/run"},
					})
					fakeImageFetcher.LocalImages[customBuilderImage.Name()] = customBuilderImage

					fakeRunImage := fakes.NewImage(t, "index.docker.io/some/run:latest", "", "")
					fakeRunImage.SetLabel("io.buildpacks.stack.id", "some.stack.id")
					fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
				})

				it("it uses the provided builder", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:   "some/app",
						Builder: "some/builder",
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), customBuilderImage.Name())
				})
			})
		})

		when("RunImage option", func() {
			when("run image stack matches the builder stack", func() {
				it.Before(func() {
					fakeRunImage := fakes.NewImage(t, "index.docker.io/custom/run:latest", "", "")
					fakeRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID)
					fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
				})

				it("uses the provided image", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:    "some/app",
						RunImage: "custom/run",
					}))
					h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "index.docker.io/custom/run:latest")
				})
			})

			when("run image stack does not match the builder stack", func() {
				it.Before(func() {
					fakeRunImage := fakes.NewImage(t, "index.docker.io/custom/run:latest", "", "")
					fakeRunImage.SetLabel("io.buildpacks.stack.id", "other.stack")
					fakeImageFetcher.LocalImages[fakeRunImage.Name()] = fakeRunImage
				})

				it("errors", func() {
					h.AssertError(t, subject.Build(context.TODO(), BuildOptions{
						Image:    "some/app",
						RunImage: "custom/run",
					}),
						"invalid run-image 'index.docker.io/custom/run:latest': run-image stack id 'other.stack' does not match builder stack 'default.stack'",
					)
				})
			})

			when("run image is not supplied", func() {
				when("there are no locally configured mirrors", func() {
					it("chooses the best mirror from the builder", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image: "some/app",
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "index.docker.io/default/run:latest")
					})

					it("chooses the best mirror from the builder", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image: "registry1.example.com/some/app",
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "registry1.example.com/run/mirror:latest")
					})

					it("chooses the best mirror from the builder", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image: "registry2.example.com/some/app",
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "registry2.example.com/run/mirror:latest")
					})
				})
			})

			when("run image is not supplied", func() {
				when("there are locally configured mirrors", func() {
					it.Before(func() {
						clientConfig.RunImages = []config.RunImage{
							{
								Image: "default/run",
								Mirrors: []string{
									"local/mirror",
									"registry1.example.com/local/mirror",
								},
							},
						}

						fakeLocalMirror := fakes.NewImage(t, "index.docker.io/local/mirror:latest", "", "")
						h.AssertNil(t, fakeLocalMirror.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
						fakeImageFetcher.LocalImages[fakeLocalMirror.Name()] = fakeLocalMirror

						fakeLocalMirror1 := fakes.NewImage(t, "registry1.example.com/local/mirror:latest", "", "")
						h.AssertNil(t, fakeLocalMirror1.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
						fakeImageFetcher.LocalImages[fakeLocalMirror1.Name()] = fakeLocalMirror1
					})

					it("prefers locally configured mirrors", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image: "some/app",
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "index.docker.io/local/mirror:latest")
					})

					it("choose the correct locally configured mirror for the registry", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image: "registry1.example.com/some/app",
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "registry1.example.com/local/mirror:latest")
					})

					it("falls back to builder mirrors", func() {
						h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
							Image: "registry2.example.com/some/app",
						}))
						h.AssertEq(t, fakeLifecycle.Opts.RunImageRef.Name(), "registry2.example.com/run/mirror:latest")
					})
				})
			})
		})

		when("ClearCache option", func() {
			it("passes it through to lifecycle", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:      "some/app",
					ClearCache: true,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ClearCache, true)
			})

			it("defaults to false", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image: "some/app",
				}))
				h.AssertEq(t, fakeLifecycle.Opts.ClearCache, false)
			})
		})

		when("Buildpacks options", func() {
			when("buildpacks are passed", func() {
				it("builder order is overwritten", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						ClearCache: true,
						Buildpacks: []string{"buildpack.id@buildpack.version"},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
					bldr, err := builder.GetBuilder(defaultBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, bldr.GetOrder(), []builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{{ID: "buildpack.id", Version: "buildpack.version"}}},
					})
				})
			})

			when("buildpacks include uris", func() {
				it("buildpacks are added to ephemeral builder", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:      "some/app",
						ClearCache: true,
						Buildpacks: []string{
							"buildpack.id@buildpack.version",
							filepath.Join("testdata", "buildpack"),
						},
					}))
					h.AssertEq(t, fakeLifecycle.Opts.Builder.Name(), defaultBuilderImage.Name())
					bldr, err := builder.GetBuilder(defaultBuilderImage)
					h.AssertNil(t, err)
					h.AssertEq(t, bldr.GetOrder(), []builder.GroupMetadata{
						{Buildpacks: []builder.GroupBuildpack{
							{ID: "buildpack.id", Version: "buildpack.version"},
							{ID: "some-buildpack-id", Version: "some-buildpack-version"},
						}},
					})
					h.AssertEq(t, bldr.GetBuildpacks(), []builder.BuildpackMetadata{
						{ID: "some-buildpack-id", Version: "some-buildpack-version"},
					})
				})

				//TODO: support other uris
			})
		})

		when("Env option", func() {
			it("should set the env on the ephemeral builder", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image: "some/app",
					Env: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				}))
				// TODO : get env from builder
				layerTar := defaultBuilderImage.FindLayerWithPath("/platform/env")
				assertTarFileContents(t, layerTar, "/platform/env/key1", `value1`)
				assertTarFileContents(t, layerTar, "/platform/env/key2", `value2`)
			})

		})

		when("Publish option", func() {
			var remoteRunImage *fakes.Image

			it("uses a remote run image", func() {
				remoteRunImage = fakes.NewImage(t, "index.docker.io/default/run:latest", "", "")
				h.AssertNil(t, remoteRunImage.SetLabel("io.buildpacks.stack.id", defaultBuilderStackID))
				fakeImageFetcher.RemoteImages[remoteRunImage.Name()] = remoteRunImage

				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Publish: true,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Publish, true)

				args := fakeImageFetcher.FetchCalls["index.docker.io/default/run:latest"]
				h.AssertEq(t, args.Daemon, false)

				args = fakeImageFetcher.FetchCalls[clientConfig.DefaultBuilder]
				h.AssertEq(t, args.Daemon, true)
				h.AssertEq(t, args.Pull, true)
			})

			it("uses a local run image", func() {
				h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
					Image:   "some/app",
					Publish: false,
				}))
				h.AssertEq(t, fakeLifecycle.Opts.Publish, false)

				args := fakeImageFetcher.FetchCalls["index.docker.io/default/run:latest"]
				h.AssertEq(t, args.Daemon, true)
				h.AssertEq(t, args.Pull, true)

				args = fakeImageFetcher.FetchCalls[clientConfig.DefaultBuilder]
				h.AssertEq(t, args.Daemon, true)
				h.AssertEq(t, args.Pull, true)
			})
		})

		when("NoPull option", func() {
			when("true", func() {
				it("uses the local builder and run images without updating", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:  "some/app",
						NoPull: true,
					}))

					args := fakeImageFetcher.FetchCalls["index.docker.io/default/run:latest"]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.Pull, false)

					args = fakeImageFetcher.FetchCalls[clientConfig.DefaultBuilder]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.Pull, false)
				})
			})

			when("false", func() {
				it("uses pulls the builder and run image before using them", func() {
					h.AssertNil(t, subject.Build(context.TODO(), BuildOptions{
						Image:  "some/app",
						NoPull: false,
					}))

					args := fakeImageFetcher.FetchCalls["index.docker.io/default/run:latest"]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.Pull, true)

					args = fakeImageFetcher.FetchCalls[clientConfig.DefaultBuilder]
					h.AssertEq(t, args.Daemon, true)
					h.AssertEq(t, args.Pull, true)
				})
			})
		})
	})
}

func assertTarFileContents(t *testing.T, tarfile, path, expected string) {
	t.Helper()
	exist, contents := tarFileContents(t, tarfile, path)
	if !exist {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
	h.AssertEq(t, contents, expected)
}

func tarFileContents(t *testing.T, tarfile, path string) (exist bool, contents string) {
	t.Helper()
	r, err := os.Open(tarfile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name == path {
			buf, err := ioutil.ReadAll(tr)
			h.AssertNil(t, err)
			return true, string(buf)
		}
	}
	return false, ""
}
