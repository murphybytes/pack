package builder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"

	"github.com/buildpack/pack/archive"
	"github.com/buildpack/pack/buildpack"
	"github.com/buildpack/pack/stack"
	"github.com/buildpack/pack/style"
)

const (
	buildpacksDir = "/buildpacks"
	platformDir   = "/platform"
	stackLabel    = "io.buildpacks.stack.id"
	envUID        = "CNB_USER_ID"
	envGID        = "CNB_GROUP_ID"
)

type Builder struct {
	image      image.Image
	buildpacks []buildpack.Buildpack
	metadata   Metadata
	env        map[string]string
	UID, GID   int
	StackID    string
}

func GetBuilder(img image.Image) (*Builder, error) {
	uid, gid, err := userAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	stackID, err := img.Label("io.buildpacks.stack.id")
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image %s", style.Symbol(stackLabel), style.Symbol(img.Name()))
	} else if stackID == "" {
		return nil, fmt.Errorf("image %s missing %s' label'",  style.Symbol(img.Name()), style.Symbol(stackLabel))
	}

	label, err := img.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find run images for builder %s", style.Symbol(img.Name()))
	} else if label == "" {
		return nil, fmt.Errorf("builder %s missing label %s -- try recreating builder", style.Symbol(img.Name()), style.Symbol(MetadataLabel))
	}

	var metadata Metadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to parse metadata for builder %s", style.Symbol(img.Name()))
	}

	return &Builder{
		image:    img,
		metadata: metadata,
		UID:      uid,
		GID:      gid,
		StackID:  stackID,
	}, nil
}

func (b *Builder) GetBuildpacks() []BuildpackMetadata {
	return b.metadata.Buildpacks
}

func (b *Builder) GetBuildpack(id string, version string) (BuildpackMetadata, bool) {
	for _, bp := range b.metadata.Buildpacks {
		if version == "latest" {
			if bp.ID == id && bp.Latest == true {
				return bp, true
			}
		} else {
			if bp.ID == id && bp.Version == version {
				return bp, true
			}
		}
	}
	return BuildpackMetadata{}, false
}

func (b *Builder) GetOrder() []GroupMetadata {
	return b.metadata.Groups
}

func (b *Builder) Name() string {
	return b.image.Name()
}

func (b *Builder) GetStackInfo() stack.Metadata {
	return b.metadata.Stack
}

func New(img image.Image, name string) (*Builder, error) {
	uid, gid, err := userAndGroupIDs(img)
	if err != nil {
		return nil, err
	}

	stackID, err := img.Label(stackLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "get label %s from image '%s'", style.Symbol(stackLabel), style.Symbol(img.Name()))
	}
	if stackID == "" {
		return nil, fmt.Errorf("image %s missing %s label", style.Symbol(img.Name()), style.Symbol(stackLabel))
	}

	label, err := img.Label(MetadataLabel)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get metadata label from image %s", style.Symbol(img.Name()))
	} else if label == "" {
		label = "{}"
	}

	var metadata Metadata
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return nil, errors.Wrapf(err, "failed to parse metadata for builder %s", style.Symbol(img.Name()))
	}

	img.Rename(name)
	return &Builder{
		image:    img,
		UID:      uid,
		GID:      gid,
		StackID:  stackID,
		metadata: metadata,
		env:      map[string]string{},
	}, nil
}

func (b *Builder) AddBuildpack(bp buildpack.Buildpack) error {
	if !bp.SupportsStack(b.StackID) {
		return fmt.Errorf("buildpack %s version %s does not support stack %s", style.Symbol(bp.ID), style.Symbol(bp.Version), style.Symbol(b.StackID))
	}
	b.buildpacks = append(b.buildpacks, bp)
	b.metadata.Buildpacks = append(b.metadata.Buildpacks, BuildpackMetadata{ID: bp.ID, Version: bp.Version, Latest: bp.Latest})
	return nil
}

func (b *Builder) SetEnv(env map[string]string) {
	b.env = env
}

func (b *Builder) SetOrder(order []GroupMetadata) {
	b.metadata.Groups = order
}

func (b *Builder) SetStackInfo(stackConfig StackConfig) {
	b.metadata.Stack = stack.Metadata{
		RunImage: stack.RunImageMetadata{
			Image:   stackConfig.RunImage,
			Mirrors: stackConfig.RunImageMirrors,
		},
	}
}

func (b *Builder) Save() error {
	tmpDir, err := ioutil.TempDir("", "create-builder-scratch")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	envTar, err := b.envLayer(tmpDir, b.env)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(envTar); err != nil {
		return errors.Wrap(err, "adding env layer")
	}

	for _, bp := range b.buildpacks {
		layerTar, err := b.buildpackLayer(tmpDir, bp)
		if err != nil {
			return err
		}
		if err := b.image.AddLayer(layerTar); err != nil {
			return errors.Wrapf(err, "adding layer tar for buildpack %s:%s", style.Symbol(bp.ID), style.Symbol(bp.Version))
		}
	}

	orderTar, err := b.orderLayer(tmpDir)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(orderTar); err != nil {
		return errors.Wrap(err, "adding order.tar layer")
	}

	stackTar, err := b.stackLayer(tmpDir)
	if err != nil {
		return err
	}
	if err := b.image.AddLayer(stackTar); err != nil {
		return errors.Wrap(err, "adding stack.tar layer")
	}

	label, err := json.Marshal(b.metadata)
	if err != nil {
		return errors.Wrap(err, "failed marshal builder image metadata")
	}

	if err := b.image.SetLabel(MetadataLabel, string(label)); err != nil {
		return errors.Wrap(err, "failed to set metadata label")
	}

	_, err = b.image.Save()
	return err
}

func userAndGroupIDs(img image.Image) (int, int, error) {
	sUID, err := img.Env(envUID)
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	} else if sUID == "" {
		return 0, 0, fmt.Errorf("image %s missing required env var %s", style.Symbol(img.Name()), style.Symbol(envUID))
	}

	sGID, err := img.Env(envGID)
	if err != nil {
		return 0, 0, errors.Wrap(err, "reading builder env variables")
	} else if sGID == "" {
		return 0, 0, fmt.Errorf("image %s missing required env var %s", style.Symbol(img.Name()), style.Symbol(envGID))
	}

	var uid, gid int
	uid, err = strconv.Atoi(sUID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse %s, value %s should be an integer", style.Symbol(envUID), style.Symbol(sUID))
	}

	gid, err = strconv.Atoi(sGID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse %s, value %s should be an integer", style.Symbol(envGID), style.Symbol(sGID))
	}

	return uid, gid, nil
}

func (b *Builder) orderLayer(dest string) (string, error) {
	orderTOML := &bytes.Buffer{}
	err := toml.NewEncoder(orderTOML).Encode(OrderTOML{Groups: b.metadata.Groups})
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal order.toml")
	}

	layerTar := filepath.Join(dest, "order.tar")
	err = archive.CreateSingleFileTar(layerTar, buildpacksDir+"/order.toml", orderTOML.String())
	if err != nil {
		return "", errors.Wrapf(err, "failed to create order.toml layer tar")
	}

	return layerTar, nil
}

func (b *Builder) stackLayer(dest string) (string, error) {
	stackTOML := &bytes.Buffer{}
	err := toml.NewEncoder(stackTOML).Encode(b.metadata.Stack)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal stack.toml")
	}

	layerTar := filepath.Join(dest, "stack.tar")
	err = archive.CreateSingleFileTar(layerTar, buildpacksDir+"/stack.toml", stackTOML.String())
	if err != nil {
		return "", errors.Wrapf(err, "failed to create stack.toml layer tar")
	}

	return layerTar, nil
}

func (b *Builder) buildpackLayer(dest string, bp buildpack.Buildpack) (string, error) {
	layerTar := filepath.Join(dest, fmt.Sprintf("%s.%s.tar", bp.EscapedID(), bp.Version))

	fh, err := os.Create(layerTar)
	if err != nil {
		return "", fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	if err := archive.WriteDirToTar(tw, bp.Dir, fmt.Sprintf("%s/%s/%s", buildpacksDir, bp.EscapedID(), bp.Version), b.UID, b.GID); err != nil {
		return "", errors.Wrapf(err, "creating layer tar for buildpack '%s:%s'", bp.ID, bp.Version)
	}

	if bp.Latest {
		err := tw.WriteHeader(&tar.Header{
			Name:     fmt.Sprintf("%s/%s/%s", buildpacksDir, bp.EscapedID(), "latest"),
			Linkname: fmt.Sprintf("%s/%s/%s", buildpacksDir, bp.EscapedID(), bp.Version),
			Typeflag: tar.TypeSymlink,
			Mode:     0666,
			Uid:      b.UID,
			Gid:      b.GID,
		})
		if err != nil {
			return "", errors.Wrapf(err, "creating latest symlink for buildpack '%s:%s'", bp.ID, bp.Version)
		}
	}

	return layerTar, nil
}

func (b *Builder) envLayer(dest string, env map[string]string) (string, error) {
	fh, err := os.Create(filepath.Join(dest, "env.tar"))
	if err != nil {
		return "", err
	}
	defer fh.Close()

	tw := tar.NewWriter(fh)
	defer tw.Close()

	now := time.Now()

	for k, v := range env {
		if err := tw.WriteHeader(&tar.Header{Name: platformDir + "/env/" + k, Size: int64(len(v)), Mode: 0444, ModTime: now}); err != nil {
			return "", err
		}
		if _, err := tw.Write([]byte(v)); err != nil {
			return "", err
		}
	}

	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: platformDir + "/env", Mode: 0555, ModTime: now}); err != nil {
		return "", err
	}

	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: platformDir, Mode: 0555, ModTime: now}); err != nil {
		return "", err
	}

	return fh.Name(), nil
}
