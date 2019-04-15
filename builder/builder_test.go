package builder_test

import (
	"errors"
	"testing"

	"github.com/fatih/color"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/config"
	"github.com/buildpack/pack/mocks"
	h "github.com/buildpack/pack/testhelpers"
)

func TestBuilder(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller
		mockImage      *mocks.MockImage
		cfg            *config.Config
		subject        *builder.Builder
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		mockImage = mocks.NewMockImage(mockController)
		mockImage.EXPECT().Name().Return("some/builder")
		cfg = &config.Config{}
		subject = builder.NewBuilder(mockImage, cfg)
	})

	when("#GetStack", func() {
		when("error getting stack label", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("", errors.New("some error"))
			})

			it("returns an error", func() {
				_, err := subject.GetStack()
				h.AssertError(t, err, "failed to find stack label for builder 'some/builder'")
			})
		})

		when("stack label is empty", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.stack.id").Return("", nil)
			})

			it("returns an error", func() {
				_, err := subject.GetStack()
				h.AssertError(t, err, "builder 'some/builder' missing label 'io.buildpacks.stack.id' -- try recreating builder")
			})
		})
	})

	when("#GetMetadata", func() {
		when("error getting metadata label", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", errors.New("some error"))
			})

			it("returns an error", func() {
				_, err := subject.GetMetadata()
				h.AssertError(t, err, "failed to find run images for builder 'some/builder'")
			})
		})

		when("metadata label is empty", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("", nil)
			})

			it("returns an error", func() {
				_, err := subject.GetMetadata()
				h.AssertError(t, err, "builder 'some/builder' missing label 'io.buildpacks.builder.metadata' -- try recreating builder")
			})
		})

		when("metadata label is not parsable", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return("junk", nil)
			})

			it("returns an error", func() {
				_, err := subject.GetMetadata()
				h.AssertError(t, err, "failed to parse metadata for builder 'some/builder'")
			})
		})
	})

	when("#GetLocalRunImageMirrors", func() {
		when("run image exists in config", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").
					Return(`{"stack":{"runImage": {"image": "some/run-image","mirrors": []}}}`, nil)
				cfg.RunImages = []config.RunImage{{Image: "some/run-image", Mirrors: []string{"a", "b"}}}
			})

			it("returns the local mirrors", func() {
				localMirrors, err := subject.GetLocalRunImageMirrors()
				h.AssertNil(t, err)
				h.AssertSliceContains(t, localMirrors, "a")
				h.AssertSliceContains(t, localMirrors, "b")
			})
		})

		when("run image does not exist in config", func() {
			it.Before(func() {
				mockImage.EXPECT().Label("io.buildpacks.builder.metadata").Return(`{
 "runImage": {
   "image": "some/other-run-image",
   "mirrors": []
 }
}`, nil)
				cfg.RunImages = []config.RunImage{{Image: "some/run-image", Mirrors: []string{"a", "b"}}}
			})

			it("returns an empty slice", func() {
				localMirrors, err := subject.GetLocalRunImageMirrors()
				h.AssertNil(t, err)
				h.AssertEq(t, len(localMirrors), 0)
			})
		})
	})

	when("#GetRunImageByRepoName", func() {
		when("there are NOT local run image mirrors", func() {
			it("should return the remote run image for the repo", func() {
				mockImage.EXPECT().Label(builder.MetadataLabel).
					Return(`{"stack":{"runImage": {"image": "some/run-image","mirrors": ["foo.bar/other/run-image", "gcr.io/extra/run-image"]}}}`, nil).AnyTimes()

				runImage, err := subject.GetRunImageByRepoName("gcr.io/foo/bar")
				h.AssertNil(t, err)
				h.AssertEq(t, runImage, "gcr.io/extra/run-image")
			})
		})

		when("there are local run image mirrors", func() {
			it.Before(func() {
				cfg.RunImages = []config.RunImage{{Image: "some/run-image", Mirrors: []string{"gcr.io/another/run-image", "foo.bar/ignored"}}}
				mockImage.EXPECT().Label(builder.MetadataLabel).
					Return(`{"stack":{"runImage": {"image": "some/run-image","mirrors": ["foo.bar/other/run-image", "gcr.io/extra/run-image"]}}}`, nil).AnyTimes()
			})

			when("one matches the given repo", func() {
				it("should return the local run image for the repo", func() {
					runImage, err := subject.GetRunImageByRepoName("gcr.io/foo/bar")
					h.AssertNil(t, err)
					h.AssertEq(t, runImage, "gcr.io/another/run-image")
				})
			})

			when("none match the given repo", func() {
				it("should return the non-local run image for the repo", func() {
					runImage, err := subject.GetRunImageByRepoName("some/run-image")
					h.AssertNil(t, err)
					h.AssertEq(t, runImage, "some/run-image")
				})
			})
		})

		when("the repo name is invalid", func() {
			it("should err", func() {
				_, err := subject.GetRunImageByRepoName("!!@@##$$%%")
				h.AssertNotNil(t, err)
			})
		})
	})

	when("#GetUserAndGroupIDs", func() {
		when("the CNB_USER_ID and CNB_GROUP_ID are set on the image", func() {
			it.Before(func() {
				mockImage.EXPECT().Env("CNB_USER_ID").Return("1234", nil)
				mockImage.EXPECT().Env("CNB_GROUP_ID").Return("4321", nil)
			})

			it("should return the ids", func() {
				uid, gid, err := subject.GetUserAndGroupIDs()
				h.AssertNil(t, err)
				h.AssertEq(t, uid, 1234)
				h.AssertEq(t, gid, 4321)
			})
		})

		when("there is no CNB_USER_ID env var", func() {
			it.Before(func() {
				mockImage.EXPECT().Env("CNB_USER_ID").Return("", nil)
			})

			it("returns an error", func() {
				_, _, err := subject.GetUserAndGroupIDs()
				h.AssertError(t, err, "builder 'some/builder' missing required env var 'CNB_USER_ID'")
			})
		})

		when("there is no CNB_GROUP_ID env var", func() {
			it.Before(func() {
				mockImage.EXPECT().Env("CNB_USER_ID").Return("1234", nil)
				mockImage.EXPECT().Env("CNB_GROUP_ID").Return("", nil)
			})

			it("returns an error", func() {
				_, _, err := subject.GetUserAndGroupIDs()
				h.AssertError(t, err, "builder 'some/builder' missing required env var 'CNB_GROUP_ID'")
			})
		})

		when("the CNB_USER_ID env var value is not an integer", func() {
			it.Before(func() {
				mockImage.EXPECT().Env("CNB_USER_ID").Return("not an int", nil)
				mockImage.EXPECT().Env("CNB_GROUP_ID").Return("4321", nil)
			})

			it("returns an error", func() {
				_, _, err := subject.GetUserAndGroupIDs()
				h.AssertError(t, err, "failed to parse 'CNB_USER_ID', value 'not an int' should be an integer")
			})
		})

		when("the CNB_GROUP_ID env var value is not an integer", func() {
			it.Before(func() {
				mockImage.EXPECT().Env("CNB_USER_ID").Return("1234", nil)
				mockImage.EXPECT().Env("CNB_GROUP_ID").Return("not an int", nil)
			})

			it("returns an error", func() {
				_, _, err := subject.GetUserAndGroupIDs()
				h.AssertError(t, err, "failed to parse 'CNB_GROUP_ID', value 'not an int' should be an integer")
			})
		})
	})
}
