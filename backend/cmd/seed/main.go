// Command seed populates a database with a demo dataset for development and
// manual testing.
//
// It refuses to run against a production configuration: the accounts it creates
// have known passwords, and one of them is a super admin.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/database"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/hash"
	"github.com/loadify/backend/pkg/logger"
)

// demoPassword is shared by every seeded account. It is printed at the end so
// nobody has to read the source to sign in.
const demoPassword = "Loadify123"

func main() {
	force := flag.Bool("force", false, "run even if demo data already exists")
	flag.Parse()

	cfg, err := config.Load(".env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.App.Env, "info")

	if cfg.App.IsProduction() {
		log.Fatal("refusing to seed a production database: these accounts have known passwords")
	}

	db, err := database.New(cfg.DB, cfg.App.Env, log)
	if err != nil {
		log.Fatal("database", logger.Err(err))
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	hasher := hash.NewBcrypt(cfg.Security.BcryptCost)

	if err := seed(ctx, db.DB, hasher, *force, log); err != nil {
		log.Fatal("seeding failed", logger.Err(err))
	}

	log.Info("seed complete")
	fmt.Printf(`
Demo accounts (password: %s)

  +998901111111  super admin
  +998902222222  company manager — Ipak Yuli Logistics
  +998903333333  dispatcher      — Ipak Yuli Logistics
  +998904444444  driver, verified, 20t tilt
  +998905555555  driver, verified, refrigerated

`, demoPassword)
}

// seed writes the demo dataset in one transaction: a half-seeded database is
// more confusing than an empty one.
func seed(ctx context.Context, db *gorm.DB, hasher hash.Hasher, force bool, log logger.Logger) error {
	var existing int64
	if err := db.WithContext(ctx).Model(&domain.User{}).
		Where("phone = ?", "+998901111111").Count(&existing).Error; err != nil {
		return fmt.Errorf("checking for existing demo data: %w", err)
	}
	if existing > 0 && !force {
		log.Info("demo data already present, nothing to do (pass -force to re-run)")
		return nil
	}

	passwordHash, err := hasher.Hash(demoPassword)
	if err != nil {
		return fmt.Errorf("hashing the demo password: %w", err)
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reference data must already be there: the seed builds on the regions,
		// cargo types and plans the migrations insert.
		var regionCount int64
		if err := tx.Model(&domain.Region{}).Count(&regionCount).Error; err != nil {
			return err
		}
		if regionCount == 0 {
			return fmt.Errorf("no reference data: run the migrations first")
		}

		admin, err := createUser(tx, "+998901111111", "Admin", "Loadify", passwordHash)
		if err != nil {
			return err
		}
		if err := grantRole(tx, admin.ID, domain.RoleSuperAdmin, nil); err != nil {
			return err
		}

		manager, err := createUser(tx, "+998902222222", "Rustam", "Karimov", passwordHash)
		if err != nil {
			return err
		}
		dispatcher, err := createUser(tx, "+998903333333", "Dilnoza", "Yusupova", passwordHash)
		if err != nil {
			return err
		}

		company, err := createCompany(tx, manager.ID)
		if err != nil {
			return err
		}
		for _, member := range []struct {
			userID uuid.UUID
			role   domain.MemberRole
		}{
			{manager.ID, domain.MemberRoleManager},
			{dispatcher.ID, domain.MemberRoleDispatcher},
		} {
			joined := time.Now().UTC()
			if err := tx.Create(&domain.CompanyMember{
				ID: uuid.New(), CompanyID: company.ID, UserID: member.userID,
				MemberRole: member.role, IsActive: true, JoinedAt: &joined,
			}).Error; err != nil {
				return err
			}
			roleCode := domain.RoleDispatcher
			if member.role == domain.MemberRoleManager {
				roleCode = domain.RoleCompanyManager
			}
			if err := grantRole(tx, member.userID, roleCode, &company.ID); err != nil {
				return err
			}
		}

		tiltUser, err := createUser(tx, "+998904444444", "Anvar", "Tursunov", passwordHash)
		if err != nil {
			return err
		}
		if err := createDriver(tx, tiltUser.ID, domain.VehicleTent, 20000, "01A111BC", false); err != nil {
			return err
		}

		fridgeUser, err := createUser(tx, "+998905555555", "Bekzod", "Aliyev", passwordHash)
		if err != nil {
			return err
		}
		if err := createDriver(tx, fridgeUser.ID, domain.VehicleRefrigerator, 15000, "01B222DE", true); err != nil {
			return err
		}

		return createLoads(tx, company.ID, manager.ID)
	})
}

func createUser(tx *gorm.DB, phone, firstName, lastName, passwordHash string) (*domain.User, error) {
	now := time.Now().UTC()
	user := &domain.User{
		ID:              uuid.New(),
		Phone:           phone,
		PasswordHash:    passwordHash,
		FirstName:       firstName,
		LastName:        lastName,
		Locale:          domain.LocaleUZ,
		Status:          domain.UserStatusActive,
		PhoneVerifiedAt: &now,
	}
	// Re-running the seed updates the existing account rather than colliding.
	if err := tx.Where("phone = ?", phone).
		Assign(map[string]any{
			"password_hash":     passwordHash,
			"status":            domain.UserStatusActive,
			"phone_verified_at": now,
		}).
		FirstOrCreate(user).Error; err != nil {
		return nil, fmt.Errorf("creating %s: %w", phone, err)
	}
	return user, nil
}

func grantRole(tx *gorm.DB, userID uuid.UUID, roleCode string, companyID *uuid.UUID) error {
	var role domain.Role
	if err := tx.First(&role, "code = ?", roleCode).Error; err != nil {
		return fmt.Errorf("finding the %s role: %w", roleCode, err)
	}
	return tx.Exec(`
		INSERT INTO user_roles (id, user_id, role_id, company_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, now(), now())
		ON CONFLICT DO NOTHING`,
		uuid.New(), userID, role.ID, companyID).Error
}

func createCompany(tx *gorm.DB, ownerID uuid.UUID) (*domain.Company, error) {
	now := time.Now().UTC()
	tin := "301234567"
	company := &domain.Company{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		LegalName:   `MChJ "IPAK YULI LOGISTICS"`,
		BrandName:   "Ipak Yuli Logistics",
		TIN:         &tin,
		Phone:       "+998712001122",
		Status:      domain.CompanyStatusVerified,
		VerifiedAt:  &now,
	}
	if err := tx.Where("owner_user_id = ?", ownerID).FirstOrCreate(company).Error; err != nil {
		return nil, fmt.Errorf("creating the demo company: %w", err)
	}
	return company, nil
}

func createDriver(tx *gorm.DB, userID uuid.UUID, vehicleType domain.VehicleType, capacityKg int, plate string, refrigerated bool) error {
	now := time.Now().UTC()
	licence := "AA" + plate[2:7]
	tashkent := geo.NewPoint(41.2995, 69.2401)

	driver := &domain.Driver{
		ID:                 uuid.New(),
		UserID:             userID,
		LicenseNumber:      &licence,
		ExperienceYears:    8,
		Languages:          domain.StringArray{"uz", "ru"},
		LicenseCategories:  domain.StringArray{"B", "C", "CE"},
		Status:             domain.DriverStatusOnline,
		VerificationStatus: domain.VerificationVerified,
		VerifiedAt:         &now,
		CurrentLocation:    &tashkent,
		LocationUpdatedAt:  &now,
	}
	if err := tx.Where("user_id = ?", userID).FirstOrCreate(driver).Error; err != nil {
		return fmt.Errorf("creating the demo driver: %w", err)
	}

	volume := 86.0
	length, width, height := 1360, 245, 270
	vehicle := &domain.Vehicle{
		ID:          uuid.New(),
		DriverID:    driver.ID,
		Type:        vehicleType,
		Make:        "MAN",
		Model:       "TGX 18.440",
		PlateNumber: plate,
		CapacityKg:  capacityKg,
		VolumeM3:    &volume,
		LengthCm:    &length,
		WidthCm:     &width,
		HeightCm:    &height,
		FuelType:    "diesel",
		Status:      domain.VerificationApproved,
		IsPrimary:   true,
	}
	if refrigerated {
		minC, maxC := int16(-20), int16(5)
		vehicle.HasRefrigeration = true
		vehicle.TempMinC, vehicle.TempMaxC = &minC, &maxC
	}
	if err := tx.Where("plate_number = ?", plate).FirstOrCreate(vehicle).Error; err != nil {
		return fmt.Errorf("creating the demo vehicle: %w", err)
	}
	return nil
}

// createLoads publishes a few shipments on the busiest real corridors, so the
// marketplace is not empty on first run.
func createLoads(tx *gorm.DB, companyID, createdBy uuid.UUID) error {
	type corridor struct {
		title        string
		fromName     string
		from         geo.Point
		toName       string
		to           geo.Point
		weightKg     int
		price        int64
		vehicle      domain.VehicleType
		refrigerated bool
	}

	corridors := []corridor{
		{"Construction materials, 18t", "Tashkent", geo.NewPoint(41.2995, 69.2401),
			"Samarkand", geo.NewPoint(39.6270, 66.9750), 18000, 350_000_00, domain.VehicleTent, false},
		{"Frozen produce, 12t", "Tashkent", geo.NewPoint(41.2995, 69.2401),
			"Bukhara", geo.NewPoint(39.7681, 64.4556), 12000, 480_000_00, domain.VehicleRefrigerator, true},
		{"Textile rolls, 8t", "Fergana", geo.NewPoint(40.3864, 71.7864),
			"Tashkent", geo.NewPoint(41.2995, 69.2401), 8000, 220_000_00, domain.VehicleTent, false},
		{"Grain, 20t", "Jizzakh", geo.NewPoint(40.1158, 67.8422),
			"Nukus", geo.NewPoint(42.4600, 59.6100), 20000, 620_000_00, domain.VehicleGrain, false},
	}

	now := time.Now().UTC()
	for i, c := range corridors {
		pickup, err := createAddress(tx, c.fromName, c.from)
		if err != nil {
			return err
		}
		delivery, err := createAddress(tx, c.toName, c.to)
		if err != nil {
			return err
		}

		pickupFrom := now.Add(time.Duration(24*(i+1)) * time.Hour)
		expiry := pickupFrom.Add(72 * time.Hour)
		distance := geo.DistanceKm(c.from, c.to)
		vehicleType := c.vehicle

		load := &domain.Load{
			ID:                    uuid.New(),
			CompanyID:             companyID,
			CreatedByUserID:       createdBy,
			Title:                 c.title,
			WeightKg:              c.weightKg,
			PickupAddressID:       pickup.ID,
			DeliveryAddressID:     delivery.ID,
			PickupLocation:        c.from,
			DeliveryLocation:      c.to,
			PickupRegionID:        pickup.RegionID,
			DeliveryRegionID:      delivery.RegionID,
			PickupFrom:            pickupFrom,
			DistanceKm:            &distance,
			RequiredVehicleType:   &vehicleType,
			RequiresRefrigeration: c.refrigerated,
			Packaging:             "pallet",
			LoadingType:           "rear",
			PriceAmount:           c.price,
			PriceCurrency:         domain.CurrencyUZS,
			PriceType:             domain.PriceFixed,
			PaymentTerms:          domain.PayOnDelivery,
			PaymentMethod:         "cash",
			Status:                domain.LoadPublished,
			PublishedAt:           &now,
			ExpiresAt:             &expiry,
		}
		if c.refrigerated {
			minC, maxC := int16(-18), int16(-5)
			load.TempMinC, load.TempMaxC = &minC, &maxC
		}
		if err := tx.Create(load).Error; err != nil {
			return fmt.Errorf("creating the demo load %q: %w", c.title, err)
		}
	}
	return nil
}

func createAddress(tx *gorm.DB, cityName string, point geo.Point) (*domain.Address, error) {
	address := &domain.Address{
		ID:        uuid.New(),
		Line1:     cityName,
		Formatted: cityName + ", Uzbekistan",
		Location:  point,
	}

	// Resolve the region from the coordinates so the load is filterable by
	// region, exactly as the API does when a client pins a map location.
	var region domain.Region
	err := tx.Where("is_active AND center IS NOT NULL").
		Order(gorm.Expr("center <-> ?::geography", point)).
		First(&region).Error
	if err == nil {
		address.RegionID = &region.ID
		address.CountryID = &region.CountryID
	}

	if err := tx.Create(address).Error; err != nil {
		return nil, fmt.Errorf("creating the address for %s: %w", cityName, err)
	}
	return address, nil
}
