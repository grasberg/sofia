package utils

import "testing"

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"photo.png", true},
		{"photo.PNG", true},
		{"image.jpg", true},
		{"image.jpeg", true},
		{"image.JPEG", true},
		{"anim.gif", true},
		{"photo.webp", true},
		{"photo.bmp", true},
		{"photo.tiff", true},
		{"document.pdf", false},
		{"archive.zip", false},
		{"code.go", false},
		{"readme.md", false},
		{"", false},
		{"noext", false},
		{"photo.jpg.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := IsImageFile(tt.filename); got != tt.want {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestIsAudioFile(t *testing.T) {
	tests := []struct {
		filename    string
		contentType string
		want        bool
	}{
		{"song.mp3", "", true},
		{"voice.ogg", "", true},
		{"audio.wav", "", true},
		{"file.m4a", "", true},
		{"file", "audio/mpeg", true},
		{"file", "application/ogg", true},
		{"photo.png", "", false},
		{"doc.pdf", "", false},
		{"file", "text/plain", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename+"/"+tt.contentType, func(t *testing.T) {
			if got := IsAudioFile(tt.filename, tt.contentType); got != tt.want {
				t.Errorf("IsAudioFile(%q, %q) = %v, want %v", tt.filename, tt.contentType, got, tt.want)
			}
		})
	}
}
