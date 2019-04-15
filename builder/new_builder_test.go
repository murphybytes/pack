package builder_test

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack/builder"
	"github.com/buildpack/pack/buildpack"

	h "github.com/buildpack/pack/testhelpers"
)

func TestBuilder2(t *testing.T) {
	color.NoColor = true
	spec.Run(t, "Builder2", testBuilder2, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder2(t *testing.T, when spec.G, it spec.S) {
	var (
		baseImage *fakes.Image
		subject   *builder.Builder2
	)

	it.Before(func() {
		var err error
		baseImage = fakes.NewImage(t, "base/image", "", "")
		h.AssertNil(t, baseImage.SetEnv("CNB_USER_ID", "1234"))
		h.AssertNil(t, baseImage.SetEnv("CNB_GROUP_ID", "4321"))
		subject, err = builder.New(baseImage, "some/builder")
		h.AssertNil(t, err)
	})

	it.After(func() {
		//baseImage.Cleanup()
	})

	when("#Save", func() {
		it("creates a builder from the image and renames it", func() {
			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)
			h.AssertEq(t, baseImage.Name(), "some/builder")
		})
	})

	when("#AddBuildpack", func() {
		it.Before(func() {
			subject.AddBuildpack(buildpack.Buildpack{
				ID:      "some-buildpack-id",
				Version: "some-buildpack-version",
				Dir:     filepath.Join("testdata", "buildpack"),
			})

			subject.AddBuildpack(buildpack.Buildpack{
				ID:      "other-buildpack-id",
				Version: "other-buildpack-version",
				Dir:     filepath.Join("testdata", "buildpack"),
				Latest:  true,
			})

			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)
		})

		it("adds the buildpack as an image layer", func() {
			layerTar := baseImage.FindLayerWithPath("/buildpacks/some-buildpack-id/some-buildpack-version")
			assertTarFileContents(t, layerTar, "/buildpacks/some-buildpack-id/some-buildpack-version/buildpack-file", "buildpack-contents")

			layerTar = baseImage.FindLayerWithPath("/buildpacks/other-buildpack-id/other-buildpack-version")
			assertTarFileContents(t, layerTar, "/buildpacks/other-buildpack-id/other-buildpack-version/buildpack-file", "buildpack-contents")
		})

		it("adds a symlink to the buildpack layer if latest is true", func() {
			layerTar := baseImage.FindLayerWithPath("/buildpacks/other-buildpack-id")
			fmt.Println("LAYER TAR", layerTar)
			assertTarFileSymlink(t, layerTar, "/buildpacks/other-buildpack-id/latest", "/buildpacks/other-buildpack-id/other-buildpack-version")
			assertTarFileOwner(t, layerTar, "/buildpacks/other-buildpack-id/latest", 1234, 4321)
		})

		it("adds the buildpack contents with the correct uid and gid", func() {
			layerTar := baseImage.FindLayerWithPath("/buildpacks/some-buildpack-id/some-buildpack-version")
			assertTarFileOwner(t, layerTar, "/buildpacks/some-buildpack-id/some-buildpack-version/buildpack-file", 1234, 4321)

			layerTar = baseImage.FindLayerWithPath("/buildpacks/other-buildpack-id/other-buildpack-version")
			assertTarFileOwner(t, layerTar, "/buildpacks/other-buildpack-id/other-buildpack-version/buildpack-file", 1234, 4321)
		})

		it("adds the buildpack metadata", func() {
			label, err := baseImage.Label("io.buildpacks.builder.metadata")
			h.AssertNil(t, err)

			var metadata builder.Metadata
			h.AssertNil(t, json.Unmarshal([]byte(label), &metadata))
			h.AssertEq(t, len(metadata.Buildpacks), 2)

			h.AssertEq(t, metadata.Buildpacks[0].ID, "some-buildpack-id")
			h.AssertEq(t, metadata.Buildpacks[0].Version, "some-buildpack-version")
			h.AssertEq(t, metadata.Buildpacks[0].Latest, false)

			h.AssertEq(t, metadata.Buildpacks[1].ID, "other-buildpack-id")
			h.AssertEq(t, metadata.Buildpacks[1].Version, "other-buildpack-version")
			h.AssertEq(t, metadata.Buildpacks[1].Latest, true)
		})

		// buildpack has wrong stack
		// keep buildpacks from original image in metadata
	})

	// uid gid error cases

	when("#SetOrder", func() {
		it.Before(func() {
			h.AssertNil(t, subject.SetOrder([]builder.GroupMetadata{}))
			h.AssertNil(t, subject.Save())
			h.AssertEq(t, baseImage.IsSaved(), true)
		})

		it("adds the order.toml to the image", func() {

		})

		it("adds the order to the metadata", func() {

		})

		//order buildpack doesn't exist in image
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

func assertTarFileSymlink(t *testing.T, tarFile, path, expected string) {
	t.Helper()
	r, err := os.Open(tarFile)
	h.AssertNil(t, err)
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		h.AssertNil(t, err)

		if header.Name != path {
			continue
		}

		if header.Typeflag != tar.TypeSymlink {
			t.Fatalf("path '%s' is not a symlink, type flag is '%c'", header.Name, header.Typeflag)
		}

		if header.Linkname != expected {
			t.Fatalf("symlink '%s' does not point to '%s', instead it points to '%s'", header.Name, expected, header.Linkname)
		}
	}
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

func assertTarFileOwner(t *testing.T, tarfile, path string, expectedUID, expectedGID int) {
	t.Helper()
	var foundPath bool
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
			foundPath = true
			if header.Uid != expectedUID {
				t.Fatalf("expected all entries in `%s` to have uid '%d', but '%s' has '%d'", tarfile, expectedUID, header.Name, header.Uid)
			}
			if header.Gid != expectedGID {
				t.Fatalf("expected all entries in `%s` to have gid '%d', got '%d'", tarfile, expectedGID, header.Gid)
			}
		}
	}
	if !foundPath {
		t.Fatalf("%s does not exist in %s", path, tarfile)
	}
}
