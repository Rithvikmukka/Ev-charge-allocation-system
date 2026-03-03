package internal

import (
	"fmt"
	"sync"
)

type Storage struct {
	mu    sync.RWMutex
	Slots map[string]Slot
}

func NewStorageWithDefaultSlots() *Storage {
	keys := DefaultSlotKeys()
	s := &Storage{Slots: make(map[string]Slot, len(keys))}
	for _, k := range keys {
		s.Slots[k] = Slot{Status: SlotFree, VehicleID: "", Version: 0}
	}
	return s
}

func (s *Storage) Get(slotKey string) (Slot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.Slots[slotKey]
	return v, ok
}

// ApplyIfNewer applies the update only if the incoming version is >= local version.
// Returns true if the value was applied.
func (s *Storage) ApplyIfNewer(slotKey string, incoming Slot) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.Slots[slotKey]
	if !ok {
		s.Slots[slotKey] = incoming
		return true
	}
	if incoming.Version >= cur.Version {
		s.Slots[slotKey] = incoming
		return true
	}
	return false
}

// ReserveIfFree atomically reserves the slot if it is currently FREE.
// Returns the updated slot and whether the reservation succeeded.
func (s *Storage) ReserveIfFree(slotKey, vehicleID string) (Slot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.Slots[slotKey]
	if !ok {
		return Slot{}, false
	}
	if cur.Status == SlotBooked {
		return cur, false
	}

	next := Slot{Status: SlotBooked, VehicleID: vehicleID, Version: cur.Version + 1}
	s.Slots[slotKey] = next
	return next, true
}

// ReleaseIfBookedBy atomically releases the slot if it is BOOKED by the given vehicle.
// Returns the updated slot and whether the release succeeded.
func (s *Storage) ReleaseIfBookedBy(slotKey, vehicleID string) (Slot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.Slots[slotKey]
	if !ok {
		return Slot{}, false
	}
	if cur.Status != SlotBooked {
		return cur, false
	}
	if cur.VehicleID != vehicleID {
		return cur, false
	}

	next := Slot{Status: SlotFree, VehicleID: "", Version: cur.Version + 1}
	s.Slots[slotKey] = next
	return next, true
}

func DefaultSlotKeys() []string {
	stations := []string{"StationA", "StationB", "StationC"}
	slotsPerStation := 3
	keys := make([]string, 0, len(stations)*slotsPerStation)
	for _, st := range stations {
		for i := 1; i <= slotsPerStation; i++ {
			keys = append(keys, fmt.Sprintf("%s-Slot%d", st, i))
		}
	}
	return keys
}
