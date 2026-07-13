package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dire-kiwi/dire-agent/agent"
	"github.com/dire-kiwi/dire-agent/threadstore"
)

const (
	maxImageAttachments = 4
	maxImageBytes       = 5 << 20
	maxImageTotalBytes  = 10 << 20
)

type decodedImageAttachment struct {
	name     string
	mimeType string
	data     []byte
}

func decodeImageAttachments(attachments []ImageAttachment) ([]decodedImageAttachment, error) {
	if len(attachments) > maxImageAttachments {
		return nil, fmt.Errorf("daemon: at most %d images may be attached", maxImageAttachments)
	}
	decoded := make([]decodedImageAttachment, 0, len(attachments))
	total := 0
	for _, attachment := range attachments {
		mimeType := strings.ToLower(strings.TrimSpace(attachment.MimeType))
		if _, ok := imageExtension(mimeType); !ok {
			return nil, fmt.Errorf("daemon: unsupported image type %q", attachment.MimeType)
		}
		encoded := strings.TrimSpace(attachment.Data)
		if marker := strings.Index(encoded, ","); strings.HasPrefix(encoded, "data:") && marker >= 0 {
			encoded = encoded[marker+1:]
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("daemon: image data is not valid base64")
		}
		if len(data) == 0 {
			return nil, errors.New("daemon: image attachment is empty")
		}
		if len(data) > maxImageBytes {
			return nil, fmt.Errorf("daemon: each image must be %d MiB or smaller", maxImageBytes>>20)
		}
		total += len(data)
		if total > maxImageTotalBytes {
			return nil, fmt.Errorf("daemon: image attachments must total %d MiB or less", maxImageTotalBytes>>20)
		}
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			name = "pasted-image"
		}
		if len([]rune(name)) > 200 {
			return nil, errors.New("daemon: image name must be 200 characters or fewer")
		}
		decoded = append(decoded, decodedImageAttachment{name: name, mimeType: mimeType, data: data})
	}
	return decoded, nil
}

func persistImageAttachments(project threadstore.Thread, images []decodedImageAttachment) ([]ImageAttachment, []agent.ImageInput, error) {
	if len(images) == 0 {
		return nil, nil, nil
	}
	if project.ResourceKind() != threadstore.KindProject || project.IsSubagent() || strings.TrimSpace(project.CWD) == "" {
		return nil, nil, errors.New("daemon: image attachments require a top-level project sandbox")
	}
	directory := filepath.Join(project.CWD, ".dire-agent", "attachments")
	if err := makeSandboxAttachmentDirectory(project.CWD, directory); err != nil {
		return nil, nil, err
	}
	stored := make([]ImageAttachment, 0, len(images))
	inputs := make([]agent.ImageInput, 0, len(images))
	written := make([]string, 0, len(images))
	cleanup := func() {
		for _, path := range written {
			_ = os.Remove(path)
		}
	}
	for _, image := range images {
		extension, _ := imageExtension(image.mimeType)
		id, err := newAttachmentID()
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("daemon: create attachment id: %w", err)
		}
		file := id + extension
		path := filepath.Join(directory, file)
		if err := os.WriteFile(path, image.data, 0o600); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("daemon: persist image attachment: %w", err)
		}
		written = append(written, path)
		stored = append(stored, ImageAttachment{
			Name: image.name, MimeType: image.mimeType, File: file, Size: int64(len(image.data)),
		})
		inputs = append(inputs, agent.ImageInput{Name: image.name, MimeType: image.mimeType, Data: image.data})
	}
	return stored, inputs, nil
}

func makeSandboxAttachmentDirectory(projectRoot, directory string) error {
	for _, path := range []string{filepath.Join(projectRoot, ".dire-agent"), directory} {
		info, err := os.Lstat(path)
		switch {
		case err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()):
			return fmt.Errorf("daemon: attachment path %q is not a safe directory", path)
		case err != nil && !os.IsNotExist(err):
			return fmt.Errorf("daemon: inspect attachment directory: %w", err)
		}
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("daemon: create attachment directory: %w", err)
	}
	return nil
}

func imageExtension(mimeType string) (string, bool) {
	switch mimeType {
	case "image/png":
		return ".png", true
	case "image/jpeg":
		return ".jpg", true
	case "image/webp":
		return ".webp", true
	case "image/gif":
		return ".gif", true
	default:
		return "", false
	}
}

func newAttachmentID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "image_" + hex.EncodeToString(value[:]), nil
}
