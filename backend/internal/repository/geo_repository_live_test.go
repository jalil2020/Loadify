package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/geo"
)

func liveDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("LOADIFY_TEST_DSN")
	if dsn == "" {
		t.Skip("set LOADIFY_TEST_DSN to run this against a live database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return db
}

// TestNearestRegionMatchesReferenceSQL guards the KNN lookup. GORM does not bind
// a variable placed inside an ORDER BY expression to the right position, which
// silently returned the wrong region until the query was rewritten as raw SQL.
func TestNearestRegionMatchesReferenceSQL(t *testing.T) {
	db := liveDB(t)
	tashkent := geo.NewPoint(41.2995, 69.2401)

	var expected string
	err := db.Raw(`SELECT name_en FROM regions
	               WHERE is_active AND center IS NOT NULL
	               ORDER BY center <-> ST_SetSRID(ST_MakePoint(69.2401, 41.2995), 4326)::geography
	               LIMIT 1`).Scan(&expected).Error
	if err != nil {
		t.Fatalf("reference query: %v", err)
	}

	region, err := NewGeoRepository(db).NearestRegion(context.Background(), tashkent)
	if err != nil {
		t.Fatalf("NearestRegion: %v", err)
	}
	if region == nil {
		t.Fatal("NearestRegion found nothing for Tashkent")
	}
	if region.NameEN != expected {
		t.Errorf("NearestRegion = %q, want %q", region.NameEN, expected)
	}
}

// TestCreateAddressPersistsResolvedRegion is the check that matters for search:
// a load whose address has no region cannot be filtered by corridor, which is
// the primary way both sides browse the marketplace.
func TestCreateAddressPersistsResolvedRegion(t *testing.T) {
	db := liveDB(t)
	repo := NewGeoRepository(db)
	ctx := context.Background()

	address := &domain.Address{
		ID:        uuid.New(),
		Line1:     "diagnostic address",
		Formatted: "diagnostic address",
		Location:  geo.NewPoint(41.2995, 69.2401),
	}
	if err := repo.CreateAddress(ctx, address); err != nil {
		t.Fatalf("CreateAddress: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM addresses WHERE id = ?", address.ID) })

	if address.RegionID == nil {
		t.Fatal("CreateAddress left RegionID unset on the in-memory struct")
	}

	// Read it back: setting the field in Go is worthless if it does not persist.
	var stored domain.Address
	if err := db.First(&stored, "id = ?", address.ID).Error; err != nil {
		t.Fatalf("reading the address back: %v", err)
	}
	if stored.RegionID == nil {
		t.Fatal("region_id is NULL in the database — the load will be unfilterable by corridor")
	}
	if *stored.RegionID != *address.RegionID {
		t.Errorf("stored region %v differs from the resolved one %v", *stored.RegionID, *address.RegionID)
	}

	var name string
	db.Raw("SELECT name_en FROM regions WHERE id = ?", *stored.RegionID).Scan(&name)
	t.Logf("persisted region: %s", name)
}
