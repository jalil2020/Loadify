package domain

import "testing"

// approvedVehicle is a 20-tonne tilt trailer that satisfies a plain load.
func approvedVehicle() *Vehicle {
	volume := 86.0
	length, width, height := 1360, 245, 270
	return &Vehicle{
		Type:       VehicleTent,
		Status:     VerificationApproved,
		CapacityKg: 20000,
		VolumeM3:   &volume,
		LengthCm:   &length,
		WidthCm:    &width,
		HeightCm:   &height,
	}
}

func TestCanCarryAcceptsSuitableVehicle(t *testing.T) {
	v := approvedVehicle()
	tent := VehicleTent
	req := CargoRequirements{WeightKg: 18000, VehicleType: &tent}

	if !v.CanCarry(req) {
		t.Error("a 20t tilt refused an 18t tilt load")
	}
}

func TestCanCarryRejectsUnapprovedVehicle(t *testing.T) {
	for _, status := range []VerificationStatus{VerificationPending, VerificationRejected} {
		v := approvedVehicle()
		v.Status = status
		if v.CanCarry(CargoRequirements{WeightKg: 1000}) {
			t.Errorf("a %s vehicle was accepted", status)
		}
	}
}

func TestCanCarryEnforcesCapacity(t *testing.T) {
	v := approvedVehicle()
	if v.CanCarry(CargoRequirements{WeightKg: 20001}) {
		t.Error("a load heavier than the capacity was accepted")
	}
	if !v.CanCarry(CargoRequirements{WeightKg: 20000}) {
		t.Error("a load exactly at the capacity was refused")
	}
}

func TestCanCarryEnforcesVolume(t *testing.T) {
	v := approvedVehicle()
	tooBig := 90.0
	if v.CanCarry(CargoRequirements{WeightKg: 1000, VolumeM3: &tooBig}) {
		t.Error("a load larger than the box volume was accepted")
	}
	fits := 80.0
	if !v.CanCarry(CargoRequirements{WeightKg: 1000, VolumeM3: &fits}) {
		t.Error("a load that fits the volume was refused")
	}

	// A vehicle that never declared its volume must not be filtered out by a
	// volume requirement — the data is missing, not violated.
	v.VolumeM3 = nil
	if !v.CanCarry(CargoRequirements{WeightKg: 1000, VolumeM3: &tooBig}) {
		t.Error("a vehicle without a declared volume was rejected on volume")
	}
}

func TestCanCarryEnforcesBodyType(t *testing.T) {
	v := approvedVehicle()
	refrigerator := VehicleRefrigerator
	if v.CanCarry(CargoRequirements{WeightKg: 1000, VehicleType: &refrigerator}) {
		t.Error("a tilt was accepted for a load requiring a refrigerator")
	}

	// "other" means the shipper does not care about the body type.
	other := VehicleOther
	if !v.CanCarry(CargoRequirements{WeightKg: 1000, VehicleType: &other}) {
		t.Error("a tilt was rejected for a load with no body-type preference")
	}
	if !v.CanCarry(CargoRequirements{WeightKg: 1000}) {
		t.Error("a tilt was rejected when no body type was requested")
	}
}

func TestCanCarryEnforcesEquipmentFlags(t *testing.T) {
	base := approvedVehicle()

	if base.CanCarry(CargoRequirements{WeightKg: 1000, RequiresADR: true}) {
		t.Error("a non-ADR vehicle took an ADR load")
	}
	if base.CanCarry(CargoRequirements{WeightKg: 1000, RequiresRefrigeration: true}) {
		t.Error("a non-refrigerated vehicle took a refrigerated load")
	}
	if base.CanCarry(CargoRequirements{WeightKg: 1000, RequiresTailLift: true}) {
		t.Error("a vehicle without a tail lift took a load requiring one")
	}

	equipped := approvedVehicle()
	equipped.HasADR = true
	equipped.HasRefrigeration = true
	equipped.HasTailLift = true
	if !equipped.CanCarry(CargoRequirements{
		WeightKg: 1000, RequiresADR: true, RequiresRefrigeration: true, RequiresTailLift: true,
	}) {
		t.Error("a fully equipped vehicle was rejected")
	}
}

func TestCanCarryEnforcesTemperatureRange(t *testing.T) {
	v := approvedVehicle()
	v.HasRefrigeration = true
	minC, maxC := int16(-20), int16(5)
	v.TempMinC, v.TempMaxC = &minC, &maxC

	// The unit reaches -20; a load needing -25 is out of range.
	needsMin := int16(-25)
	if v.CanCarry(CargoRequirements{WeightKg: 1000, RequiresRefrigeration: true, TempMinC: &needsMin}) {
		t.Error("a unit that only reaches -20 accepted a -25 load")
	}

	// The unit holds up to +5; a load needing +10 held is out of range.
	needsMax := int16(10)
	if v.CanCarry(CargoRequirements{WeightKg: 1000, RequiresRefrigeration: true, TempMaxC: &needsMax}) {
		t.Error("a unit capped at +5 accepted a +10 load")
	}

	inRangeMin, inRangeMax := int16(-18), int16(2)
	if !v.CanCarry(CargoRequirements{
		WeightKg: 1000, RequiresRefrigeration: true,
		TempMinC: &inRangeMin, TempMaxC: &inRangeMax,
	}) {
		t.Error("a unit was rejected for a range it covers")
	}
}

func TestCanCarryEnforcesDimensions(t *testing.T) {
	v := approvedVehicle()

	tooLong := 1400
	if v.CanCarry(CargoRequirements{WeightKg: 1000, LengthCm: &tooLong}) {
		t.Error("a 13.6 m box accepted a 14 m load")
	}
	tooTall := 300
	if v.CanCarry(CargoRequirements{WeightKg: 1000, HeightCm: &tooTall}) {
		t.Error("a 2.7 m box accepted a 3 m load")
	}
	tooWide := 260
	if v.CanCarry(CargoRequirements{WeightKg: 1000, WidthCm: &tooWide}) {
		t.Error("a 2.45 m box accepted a 2.6 m load")
	}

	fitsLength, fitsWidth, fitsHeight := 1200, 240, 250
	if !v.CanCarry(CargoRequirements{
		WeightKg: 1000, LengthCm: &fitsLength, WidthCm: &fitsWidth, HeightCm: &fitsHeight,
	}) {
		t.Error("a load that fits every dimension was refused")
	}
}
