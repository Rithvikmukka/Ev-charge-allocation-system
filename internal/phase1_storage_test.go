package internal

import "testing"

func TestPhase1_DefaultSlotsInitialized(t *testing.T) {
	s := NewStorageWithDefaultSlots()
	if s == nil {
		t.Fatal("storage is nil")
	}
	keys := DefaultSlotKeys()
	if len(keys) != 9 {
		t.Fatalf("expected 9 default keys, got %d", len(keys))
	}
	if len(s.Slots) != 9 {
		t.Fatalf("expected 9 slots in storage, got %d", len(s.Slots))
	}
	for _, k := range keys {
		slot, ok := s.Get(k)
		if !ok {
			t.Fatalf("missing key %s", k)
		}
		if slot.Status != SlotFree {
			t.Fatalf("expected %s to be FREE, got %s", k, slot.Status)
		}
		if slot.VehicleID != "" {
			t.Fatalf("expected %s VehicleID empty, got %q", k, slot.VehicleID)
		}
		if slot.Version != 0 {
			t.Fatalf("expected %s Version 0, got %d", k, slot.Version)
		}
	}
}
