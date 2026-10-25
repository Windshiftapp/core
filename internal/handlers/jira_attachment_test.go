package handlers

import (
	"encoding/base64"
	"io"
	"os"
	"testing"
)

func TestSniffAttachmentMimeType(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: "application/octet-stream"},
		{name: "png", data: png, want: "image/png"},
		{name: "text", data: []byte("hello attachment\n"), want: "text/plain; charset=utf-8"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "sniff-*")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			if _, err := f.Write(tc.data); err != nil {
				t.Fatal(err)
			}

			got, err := sniffAttachmentMimeType(f)
			if err != nil {
				t.Fatalf("sniffAttachmentMimeType: %v", err)
			}
			if got != tc.want {
				t.Fatalf("mime = %q, want %q", got, tc.want)
			}
			// The helper must rewind so the caller can stream the file into the
			// blob store.
			pos, err := f.Seek(0, io.SeekCurrent)
			if err != nil {
				t.Fatal(err)
			}
			if pos != 0 {
				t.Fatalf("file position = %d, want 0", pos)
			}
		})
	}
}
