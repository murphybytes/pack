package pack_test

import (
	"runtime"
	"testing"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpack/pack/testhelpers"
)

func TestCreateBuilder(t *testing.T) {
	h.RequireDocker(t)
	color.NoColor = true
	if runtime.GOOS == "windows" {
		t.Skip("create builder is not implemented on windows")
	}
	spec.Run(t, "create_builder", testCreateBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCreateBuilder(t *testing.T, when spec.G, it spec.S) {
	when("#CreateBuilder", func() {
		var (
			//mockController   *gomock.Controller
			//mockImageFetcher *mocks.MockImageFetcher
			//fakeBuildImage   *fakes.Image
			//subject          *pack.Client
		)

		it.Before(func() {
			//mockController = gomock.NewController(t)
			//mockImageFetcher = mocks.NewMockImageFetcher(mockController)
			//
			//fakeBuildImage = fakes.NewImage(t, "", "", "")
			//
			//mockImageFetcher.EXPECT().Fetch(gomock.Any(), "some/build-image", gomock.Any(), gomock.Any()).
			//	Return(fakeBuildImage).AnyTimes()
			//
			//subject = pack.NewClient(nil, nil, mockImageFetcher)
		})

		it.After(func() {
		})

		it("should do something", func() {
		})
	})
}
