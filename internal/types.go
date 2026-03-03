package internal

type Slot struct {
	Status    string // "FREE" or "BOOKED"
	VehicleID string
	Version   int
}

const (
	SlotFree   = "FREE"
	SlotBooked = "BOOKED"
)
