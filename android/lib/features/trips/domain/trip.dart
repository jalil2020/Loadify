import '../../../core/json.dart';
import '../../loads/domain/load.dart';

class LoadSummary {
  const LoadSummary({
    required this.id,
    required this.referenceNo,
    required this.title,
    required this.status,
    required this.weightKg,
    required this.pickupFormatted,
    required this.deliveryFormatted,
    required this.priceAmount,
    required this.priceCurrency,
    this.distanceKm,
  });

  final String id;
  final String referenceNo;
  final String title;
  final String status;
  final int weightKg;
  final String pickupFormatted;
  final String deliveryFormatted;
  final int priceAmount;
  final String priceCurrency;
  final double? distanceKm;

  static LoadSummary? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    return LoadSummary(
      id: json['id'] as String? ?? '',
      referenceNo: json['reference_no'] as String? ?? '',
      title: json['title'] as String? ?? '',
      status: json['status'] as String? ?? '',
      weightKg: asInt(json['weight_kg']) ?? 0,
      pickupFormatted: json['pickup_formatted'] as String? ?? '',
      deliveryFormatted: json['delivery_formatted'] as String? ?? '',
      priceAmount: asInt(json['price_amount']) ?? 0,
      priceCurrency: json['price_currency'] as String? ?? 'UZS',
      distanceKm: asDouble(json['distance_km']),
    );
  }
}

class Party {
  const Party({
    required this.userId,
    required this.fullName,
    required this.rating,
    required this.trips,
    this.phone,
  });

  final String userId;
  final String fullName;
  final double rating;
  final int trips;
  final String? phone;

  static Party? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    return Party(
      userId: json['user_id'] as String? ?? '',
      fullName: json['full_name'] as String? ?? '',
      rating: asDouble(json['rating']) ?? 0,
      trips: asInt(json['trips']) ?? 0,
      phone: json['phone'] as String?,
    );
  }
}

class Vehicle {
  const Vehicle({
    required this.id,
    required this.type,
    required this.make,
    required this.model,
    required this.plateNumber,
    required this.capacityKg,
    this.volumeM3,
  });

  final String id;
  final String type;
  final String make;
  final String model;
  final String plateNumber;
  final int capacityKg;
  final double? volumeM3;

  String get label => '$make $model'.trim();

  factory Vehicle.fromJson(Map<String, dynamic> json) => Vehicle(
    id: json['id'] as String? ?? '',
    type: json['type'] as String? ?? '',
    make: json['make'] as String? ?? '',
    model: json['model'] as String? ?? '',
    plateNumber: json['plate_number'] as String? ?? '',
    capacityKg: asInt(json['capacity_kg']) ?? 0,
    volumeM3: asDouble(json['volume_m3']),
  );

  static Vehicle? tryParse(Object? json) =>
      json is Map<String, dynamic> ? Vehicle.fromJson(json) : null;
}

/// The trip state machine, in the order the backend enforces.
///
/// This list is the client's mirror of the server's transitions, not an
/// independent opinion: the button offers `next`, and the server is still the
/// authority that rejects an out-of-order move.
enum TripStage {
  created('created'),
  enRouteToPickup('en_route_to_pickup'),
  atPickup('at_pickup'),
  loaded('loaded'),
  inTransit('in_transit'),
  atDelivery('at_delivery'),
  delivered('delivered'),
  completed('completed'),
  cancelled('cancelled'),
  disputed('disputed');

  const TripStage(this.wire);

  final String wire;

  static TripStage parse(String? value) => TripStage.values.firstWhere(
    (TripStage s) => s.wire == value,
    orElse: () => TripStage.created,
  );

  /// The stages a driver walks through, in order. `completed` is excluded
  /// because only the shipper can confirm, and the terminal states are not
  /// part of the happy path.
  static const List<TripStage> driverFlow = <TripStage>[
    TripStage.created,
    TripStage.enRouteToPickup,
    TripStage.atPickup,
    TripStage.loaded,
    TripStage.inTransit,
    TripStage.atDelivery,
    TripStage.delivered,
  ];

  /// What the driver's advance button moves to, or null at the end of the
  /// driver's part of the flow.
  TripStage? get next {
    final int i = driverFlow.indexOf(this);
    if (i < 0 || i + 1 >= driverFlow.length) return null;
    return driverFlow[i + 1];
  }

  bool get isTerminal =>
      this == TripStage.completed ||
      this == TripStage.cancelled ||
      this == TripStage.disputed;

  /// Position on the progress rail, 0..1.
  double get progress {
    if (this == TripStage.completed) return 1;
    final int i = driverFlow.indexOf(this);
    if (i < 0) return 0;
    return i / (driverFlow.length - 1);
  }
}

class Trip {
  const Trip({
    required this.id,
    required this.tripNumber,
    required this.bookingId,
    required this.loadId,
    required this.stage,
    required this.agreedPrice,
    required this.commissionAmount,
    required this.driverPayout,
    required this.currency,
    required this.canRate,
    this.load,
    this.driver,
    this.company,
    this.vehicle,
    this.currentLocation,
    this.etaAt,
    this.startedAt,
    this.deliveredAt,
  });

  final String id;
  final String tripNumber;
  final String bookingId;
  final String loadId;
  final TripStage stage;
  final int agreedPrice;
  final int commissionAmount;
  final int driverPayout;
  final String currency;
  final bool canRate;
  final LoadSummary? load;
  final Party? driver;
  final LoadCompany? company;
  final Vehicle? vehicle;
  final GeoPoint? currentLocation;
  final DateTime? etaAt;
  final DateTime? startedAt;
  final DateTime? deliveredAt;

  /// A trip is worth reporting position for only while it is actually moving.
  /// Pinging before pickup or after delivery burns battery for nothing.
  bool get isTracking =>
      stage == TripStage.enRouteToPickup ||
      stage == TripStage.loaded ||
      stage == TripStage.inTransit ||
      stage == TripStage.atDelivery;

  factory Trip.fromJson(Map<String, dynamic> json) => Trip(
    id: json['id'] as String? ?? '',
    tripNumber: json['trip_number'] as String? ?? '',
    bookingId: json['booking_id'] as String? ?? '',
    loadId: json['load_id'] as String? ?? '',
    stage: TripStage.parse(json['status'] as String?),
    agreedPrice: asInt(json['agreed_price']) ?? 0,
    commissionAmount: asInt(json['commission_amount']) ?? 0,
    driverPayout: asInt(json['driver_payout']) ?? 0,
    currency: json['currency'] as String? ?? 'UZS',
    canRate: json['can_rate'] as bool? ?? false,
    load: LoadSummary.tryParse(json['load']),
    driver: Party.tryParse(json['driver']),
    company: LoadCompany.tryParse(json['company']),
    vehicle: Vehicle.tryParse(json['vehicle']),
    currentLocation: GeoPoint.tryParse(json['current_location']),
    etaAt: asDate(json['eta_at']),
    startedAt: asDate(json['started_at']),
    deliveredAt: asDate(json['delivered_at']),
  );
}
