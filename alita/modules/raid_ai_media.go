package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/utils/gemini"
)

// Picking the one frame of a message the image check sends to Gemini, and downloading it
// without ever exposing the bot token. The Telegram file URL holds the token, and
// net/http errors embed the URL, so nothing in this file wraps, formats or logs a request,
// a file or a URL: every error is rebuilt from its cause (D-12, PLAT-02).

const (
	// raidAIPhotoTarget is the side, in pixels, the photo size is picked for: the size whose
	// larger side is nearest to it is sent (D-12).
	raidAIPhotoTarget = 1024

	// raidAIDownloadTimeout bounds one file download.
	raidAIDownloadTimeout = 10 * time.Second
)

// raidAIFileHTTPClient downloads a Telegram file. It follows no redirect, so the bot token
// in the URL is never sent to another host and a redirect is a failed download. A package
// variable only so a test can swap the transport; production code never reassigns it.
var raidAIFileHTTPClient = &http.Client{
	Timeout: raidAIDownloadTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// raidAIMedia is the one frame of a message the image check sends: a file ID to fetch with
// getFile, and the kind it came from for the logs.
type raidAIMedia struct {
	// FileID is the Telegram file to download.
	FileID string
	// FileUniqueID is Telegram's stable ID of the file; it is never logged or sent.
	FileUniqueID string
	// FileSize is the declared size in bytes, 0 when Telegram gave none.
	FileSize int64
	// Kind names where the frame came from: photo, sticker, animation, video and so on.
	Kind string
}

// raidAIMediaRef picks the frame of msg the image check sends, or reports false when the
// message has none worth sending (D-12). A caption is never part of it.
func raidAIMediaRef(msg *gotgbot.Message) (raidAIMedia, bool) {
	if msg == nil {
		return raidAIMedia{}, false
	}
	if len(msg.Photo) > 0 {
		return raidAIPhotoRef(msg.Photo)
	}
	return raidAIMedia{}, false
}

// raidAIPhotoRef takes the photo size whose larger side is nearest to raidAIPhotoTarget.
// The first of two equally near sizes wins, which is the smaller one, as Telegram lists
// sizes in ascending order. A size that declares more than gemini.MaxImageBytes is refused.
func raidAIPhotoRef(sizes []gotgbot.PhotoSize) (raidAIMedia, bool) {
	best := -1
	bestDistance := int64(0)
	for i, size := range sizes {
		if size.FileId == "" {
			continue
		}
		side := max(size.Width, size.Height)
		distance := side - raidAIPhotoTarget
		if distance < 0 {
			distance = -distance
		}
		if best == -1 || distance < bestDistance {
			best, bestDistance = i, distance
		}
	}
	if best == -1 {
		return raidAIMedia{}, false
	}
	return raidAIFrame(sizes[best].FileId, sizes[best].FileUniqueId, sizes[best].FileSize, "photo")
}

// raidAIFrame is a candidate frame, refused when it declares a size over the largest image
// the Gemini client takes.
func raidAIFrame(fileID, uniqueID string, size int64, kind string) (raidAIMedia, bool) {
	if fileID == "" || size > gemini.MaxImageBytes {
		return raidAIMedia{}, false
	}
	return raidAIMedia{FileID: fileID, FileUniqueID: uniqueID, FileSize: size, Kind: kind}, true
}

// raidAIDownload fetches the file fileID with getFile and a plain GET and returns it as a
// Gemini image. It refuses a file that is empty, over gemini.MaxImageBytes, or not a JPEG,
// PNG or WebP by its bytes (the type is sniffed, never taken from a header or a name), a
// path a local Bot API server returns as an absolute one, and any redirect. Every error is
// rebuilt: a transport error from its cause, a bad status as the code alone, so neither the
// bot token nor the URL is in an error, whatever the caller does with it.
func raidAIDownload(ctx context.Context, b *gotgbot.Bot, fileID string) (gemini.Image, error) {
	if b == nil || fileID == "" {
		return gemini.Image{}, errors.New("telegram file download: no file")
	}
	// gotgbot sanitises the errors of its own requests.
	file, err := b.GetFileWithContext(ctx, fileID, nil)
	if err != nil {
		return gemini.Image{}, fmt.Errorf("telegram getFile: %w", err)
	}
	if file == nil || file.FilePath == "" || strings.HasPrefix(file.FilePath, "/") {
		return gemini.Image{}, errors.New("telegram file download: no downloadable path")
	}
	if file.FileSize > gemini.MaxImageBytes {
		return gemini.Image{}, errors.New("telegram file download: file too large")
	}

	// The request error is not wrapped: it would carry the URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.URL(b, nil), nil)
	if err != nil {
		return gemini.Image{}, errors.New("telegram file download: build request")
	}
	resp, err := raidAIFileHTTPClient.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) && ue.Err != nil {
			return gemini.Image{}, fmt.Errorf("telegram file download: %w", ue.Err)
		}
		return gemini.Image{}, errors.New("telegram file download: request failed")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return gemini.Image{}, fmt.Errorf("telegram file download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, gemini.MaxImageBytes+1))
	if err != nil {
		return gemini.Image{}, errors.New("telegram file download: read failed")
	}
	if len(data) > gemini.MaxImageBytes {
		return gemini.Image{}, errors.New("telegram file download: file too large")
	}
	if len(data) == 0 {
		return gemini.Image{}, errors.New("telegram file download: empty file")
	}

	mimeType, _, _ := strings.Cut(http.DetectContentType(data), ";")
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return gemini.Image{}, fmt.Errorf("telegram file download: not a supported image (%s)", mimeType)
	}
	return gemini.Image{MIMEType: mimeType, Data: data}, nil
}
