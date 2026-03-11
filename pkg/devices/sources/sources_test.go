package sources

import (
	"context"
	"testing"

	"github.com/grasberg/sofia/pkg/devices/events"
)

func TestNewUSBMonitor(t *testing.T) {
	m := NewUSBMonitor()
	if m == nil {
		t.Fatal("NewUSBMonitor returned nil")
	}
}

func TestUSBMonitorKind(t *testing.T) {
	m := NewUSBMonitor()
	if m.Kind() != events.KindUSB {
		t.Errorf("Kind() = %v, want %v", m.Kind(), events.KindUSB)
	}
}

func TestUSBMonitorStart(t *testing.T) {
	m := NewUSBMonitor()
	ctx := context.Background()
	ch, err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if ch == nil {
		t.Fatal("Start returned nil channel")
	}

	// For stub, channel should be closed immediately
	_, ok := <-ch
	if ok {
		t.Error("Channel should be closed immediately on non-Linux")
	}
}

func TestUSBMonitorStop(t *testing.T) {
	m := NewUSBMonitor()
	err := m.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestUSBMonitorStartClosed(t *testing.T) {
	m := NewUSBMonitor()
	ctx := context.Background()
	ch, err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify channel is closed
	for range ch {
		t.Error("Channel should be empty and closed")
	}
}

func TestUSBMonitorMultipleStarts(t *testing.T) {
	m := NewUSBMonitor()
	ctx := context.Background()

	// First start
	ch1, err := m.Start(ctx)
	if err != nil {
		t.Fatalf("First Start failed: %v", err)
	}
	<-ch1 // drain it

	// Second start
	ch2, err := m.Start(ctx)
	if err != nil {
		t.Fatalf("Second Start failed: %v", err)
	}
	<-ch2 // drain it
}

func TestUSBMonitorStopMultipleTimes(t *testing.T) {
	m := NewUSBMonitor()
	err1 := m.Stop()
	if err1 != nil {
		t.Fatalf("First Stop failed: %v", err1)
	}

	err2 := m.Stop()
	if err2 != nil {
		t.Fatalf("Second Stop failed: %v", err2)
	}
}

func TestUSBMonitorKindConsistent(t *testing.T) {
	m1 := NewUSBMonitor()
	m2 := NewUSBMonitor()

	if m1.Kind() != m2.Kind() {
		t.Error("Kind() should be consistent across instances")
	}
}
