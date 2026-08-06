import '../../../core/json.dart';
import '../../trips/domain/trip.dart';

/// Whether the driver is taking work right now.
///
/// `busy` is set by the server when a trip is running, not by the driver, so
/// the toggle in the UI only ever moves between online and offline.
enum DriverStatus {
  offline('offline'),
  online('online'),
  busy('busy');

  const DriverStatus(this.wire);

  final String wire;

  static DriverStatus parse(String? value) => DriverStatus.values.firstWhere(
    (DriverStatus s) => s.wire == value,
    orElse: () => DriverStatus.offline,
  );

  bool get isAvailable => this == DriverStatus.online;
}

class DriverProfile {
  const DriverProfile({
    required this.id,
    required this.userId,
    required this.fullName,
    required this.status,
    required this.verificationStatus,
    required this.rating,
    required this.ratingCount,
    required this.completedTrips,
    required this.acceptanceRate,
    required this.experienceYears,
    required this.vehicles,
    this.phone,
    this.avatarUrl,
  });

  final String id;
  final String userId;
  final String fullName;
  final DriverStatus status;
  final String verificationStatus;
  final double rating;
  final int ratingCount;
  final int completedTrips;
  final double acceptanceRate;
  final int experienceYears;
  final List<Vehicle> vehicles;
  final String? phone;
  final String? avatarUrl;

  bool get isVerified => verificationStatus == 'verified';

  Vehicle? get primaryVehicle => vehicles.isEmpty ? null : vehicles.first;

  factory DriverProfile.fromJson(Map<String, dynamic> json) {
    final Object? rawVehicles = json['vehicles'];
    return DriverProfile(
      id: json['id'] as String? ?? '',
      userId: json['user_id'] as String? ?? '',
      fullName: json['full_name'] as String? ?? '',
      status: DriverStatus.parse(json['status'] as String?),
      verificationStatus: json['verification_status'] as String? ?? 'pending',
      rating: asDouble(json['rating']) ?? 0,
      ratingCount: asInt(json['rating_count']) ?? 0,
      completedTrips: asInt(json['completed_trips']) ?? 0,
      acceptanceRate: asDouble(json['acceptance_rate']) ?? 0,
      experienceYears: asInt(json['experience_years']) ?? 0,
      vehicles: rawVehicles is List
          ? rawVehicles
                .whereType<Map<String, dynamic>>()
                .map(Vehicle.fromJson)
                .toList(growable: false)
          : const <Vehicle>[],
      phone: json['phone'] as String?,
      avatarUrl: json['avatar_url'] as String?,
    );
  }
}

class Wallet {
  const Wallet({
    required this.currency,
    required this.balance,
    required this.heldBalance,
    required this.available,
  });

  final String currency;
  final int balance;
  final int heldBalance;
  final int available;

  factory Wallet.fromJson(Map<String, dynamic> json) => Wallet(
    currency: json['currency'] as String? ?? 'UZS',
    balance: asInt(json['balance']) ?? 0,
    heldBalance: asInt(json['held_balance']) ?? 0,
    available: asInt(json['available']) ?? 0,
  );
}

/// A reference vocabulary entry — region, vehicle type, cargo type.
///
/// The server returns these already localised for the request's
/// Accept-Language, so the client never translates them itself.
class RefItem {
  const RefItem({required this.id, required this.name});

  final String id;
  final String name;

  factory RefItem.fromRegion(Map<String, dynamic> json) => RefItem(
    id: json['id'] as String? ?? '',
    name: json['name'] as String? ?? '',
  );

  factory RefItem.fromVehicleType(Map<String, dynamic> json) => RefItem(
    id: json['code'] as String? ?? '',
    name: json['name'] as String? ?? '',
  );
}
