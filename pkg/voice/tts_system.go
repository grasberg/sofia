package voice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/grasberg/sofia/pkg/logger"
)

// SystemTTS implements TTSProvider using OS-native speech synthesis.
// On macOS it uses the "say" command; on Linux it uses "espeak-ng".
type SystemTTS struct{}

// NewSystemTTS creates a new system TTS provider.
func NewSystemTTS() *SystemTTS {
	logger.DebugCF("voice", "Creating system TTS provider", map[string]any{
		"os": runtime.GOOS,
	})
	return &SystemTTS{}
}

// Synthesize converts text to speech using OS-native tools.
// The voice parameter is passed to the underlying command if supported.
// Returns WAV audio bytes with content type "audio/wav".
func (t *SystemTTS) Synthesize(
	ctx context.Context, text string, voice string,
) ([]byte, string, error) {
	logger.InfoCF("voice", "Starting system TTS synthesis", map[string]any{
		"text_length": len(text),
		"voice":       voice,
		"os":          runtime.GOOS,
	})

	switch runtime.GOOS {
	case "darwin":
		return t.synthesizeDarwin(ctx, text, voice)
	case "linux":
		return t.synthesizeLinux(ctx, text, voice)
	default:
		return nil, "", fmt.Errorf("system TTS not supported on %s", runtime.GOOS)
	}
}

func (t *SystemTTS) synthesizeDarwin(
	ctx context.Context, text string, voice string,
) ([]byte, string, error) {
	tmpFile, err := os.CreateTemp("", "sofia-tts-*.wav")
	if err != nil {
		logger.ErrorCF("voice", "Failed to create temp file", map[string]any{"error": err})
		return nil, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	args := []string{"-o", tmpPath, "--data-format=LEF32@22050"}
	if voice != "" {
		args = append(args, "-v", voice)
	}
	args = append(args, text)

	logger.DebugCF("voice", "Running macOS say command", map[string]any{
		"output_file": tmpPath,
		"voice":       voice,
	})

	cmd := exec.CommandContext(ctx, "say", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.ErrorCF("voice", "say command failed", map[string]any{
			"error":  err,
			"output": string(output),
		})
		return nil, "", fmt.Errorf("say command failed: %w: %s", err, string(output))
	}

	audio, err := os.ReadFile(tmpPath)
	if err != nil {
		logger.ErrorCF("voice", "Failed to read audio file", map[string]any{"error": err})
		return nil, "", fmt.Errorf("failed to read audio file: %w", err)
	}

	logger.InfoCF("voice", "System TTS synthesis completed", map[string]any{
		"audio_size_bytes": len(audio),
	})

	return audio, "audio/wav", nil
}

func (t *SystemTTS) synthesizeLinux(
	ctx context.Context, text string, voice string,
) ([]byte, string, error) {
	args := []string{"--stdout"}
	if voice != "" {
		args = append(args, "-v", voice)
	}
	args = append(args, text)

	logger.DebugCF("voice", "Running espeak-ng command", map[string]any{
		"voice": voice,
	})

	cmd := exec.CommandContext(ctx, "espeak-ng", args...)
	audio, err := cmd.Output()
	if err != nil {
		logger.ErrorCF("voice", "espeak-ng command failed", map[string]any{"error": err})
		return nil, "", fmt.Errorf("espeak-ng command failed: %w", err)
	}

	logger.InfoCF("voice", "System TTS synthesis completed", map[string]any{
		"audio_size_bytes": len(audio),
	})

	return audio, "audio/wav", nil
}

// IsAvailable returns true if the required speech synthesis binary exists in PATH.
func (t *SystemTTS) IsAvailable() bool {
	var binary string
	switch runtime.GOOS {
	case "darwin":
		binary = "say"
	case "linux":
		binary = "espeak-ng"
	default:
		logger.DebugCF("voice", "System TTS not supported on this OS", map[string]any{
			"os": runtime.GOOS,
		})
		return false
	}

	_, err := exec.LookPath(binary)
	available := err == nil
	logger.DebugCF("voice", "Checking system TTS availability", map[string]any{
		"binary":    binary,
		"available": available,
	})
	return available
}
