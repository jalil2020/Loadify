import '../../../core/json.dart';

class GeoPoint {
  const GeoPoint(this.lat, this.lng);

  final double lat;
  final double lng;

  static GeoPoint? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    final double? lat = asDouble(json['lat']);
    final double? lng = asDouble(json['lng']);
    if (lat == null || lng == null) return null;
    return GeoPoint(lat, lng);
  }
}

class Address {
  const Address({
    required this.id,
    required this.line1,
    required this.formatted,
    this.regionName,
    this.cityName,
    this.location,
    this.contactName,
    this.contactPhone,
  });

  final String id;
  final String line1;
  final String formatted;
  final String? regionName;
  final String? cityName;
  final GeoPoint? location;
  final String? contactName;
  final String? contactPhone;

  /// What a driver actually scans for on a board: the region, falling back
  /// through progressively narrower labels rather than showing nothing.
  String get shortLabel {
    for (final String? candidate in <String?>[
      regionName,
      cityName,
      line1,
      formatted,
    ]) {
      if (candidate != null && candidate.isNotEmpty) return candidate;
    }
    return '—';
  }

  static Address? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    return Address(
      id: json['id'] as String? ?? '',
      line1: json['line1'] as String? ?? '',
      formatted: json['formatted'] as String? ?? '',
      regionName: json['region_name'] as String?,
      cityName: json['city_name'] as String?,
      location: GeoPoint.tryParse(json['location']),
      contactName: json['contact_name'] as String?,
      contactPhone: json['contact_phone'] as String?,
    );
  }
}

class LoadCompany {
  const LoadCompany({
    required this.id,
    required this.brandName,
    required this.rating,
    required this.completedTrips,
    required this.isVerified,
    this.phone,
  });

  final String id;
  final String brandName;
  final double rating;
  final int completedTrips;
  final bool isVerified;

  /// Present only once the viewer has earned contact access — that is, once a
  /// live booking exists. Null here is the masked state, not missing data.
  final String? phone;

  static LoadCompany? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    return LoadCompany(
      id: json['id'] as String? ?? '',
      brandName: json['brand_name'] as String? ?? '',
      rating: asDouble(json['rating']) ?? 0,
      completedTrips: asInt(json['completed_trips']) ?? 0,
      isVerified: json['is_verified'] as bool? ?? false,
      phone: json['phone'] as String?,
    );
  }
}

class BookingSummary {
  const BookingSummary({required this.id, required this.status});

  final String id;
  final String status;

  static BookingSummary? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    return BookingSummary(
      id: json['id'] as String? ?? '',
      status: json['status'] as String? ?? '',
    );
  }
}

/// A load on the board.
///
/// Money is an integer in minor units (tiyin) with an explicit currency, never
/// a double — 620000_00 is 620 000 so'm. Anything that renders it goes through
/// the formatter.
class Load {
  const Load({
    required this.id,
    required this.referenceNo,
    required this.title,
    required this.weightKg,
    required this.priceAmount,
    required this.priceCurrency,
    required this.status,
    required this.pickupFrom,
    required this.requiresAdr,
    required this.requiresRefrigeration,
    required this.isUrgent,
    required this.viewsCount,
    this.description,
    this.pickupAddress,
    this.deliveryAddress,
    this.distanceKm,
    this.pricePerKm,
    this.requiredVehicleType,
    this.company,
    this.myBooking,
    this.publishedAt,
  });

  final String id;
  final String referenceNo;
  final String title;
  final int weightKg;
  final int priceAmount;
  final String priceCurrency;
  final String status;
  final DateTime pickupFrom;
  final bool requiresAdr;
  final bool requiresRefrigeration;
  final bool isUrgent;
  final int viewsCount;
  final String? description;
  final Address? pickupAddress;
  final Address? deliveryAddress;
  final double? distanceKm;
  final int? pricePerKm;
  final String? requiredVehicleType;
  final LoadCompany? company;
  final BookingSummary? myBooking;
  final DateTime? publishedAt;

  bool get isBooked => myBooking != null;

  factory Load.fromJson(Map<String, dynamic> json) => Load(
    id: json['id'] as String? ?? '',
    referenceNo: json['reference_no'] as String? ?? '',
    title: json['title'] as String? ?? '',
    weightKg: asInt(json['weight_kg']) ?? 0,
    priceAmount: asInt(json['price_amount']) ?? 0,
    priceCurrency: json['price_currency'] as String? ?? 'UZS',
    status: json['status'] as String? ?? '',
    pickupFrom: asDate(json['pickup_from']) ?? DateTime.now().toUtc(),
    requiresAdr: json['requires_adr'] as bool? ?? false,
    requiresRefrigeration: json['requires_refrigeration'] as bool? ?? false,
    isUrgent: json['is_urgent'] as bool? ?? false,
    viewsCount: asInt(json['views_count']) ?? 0,
    description: json['description'] as String?,
    pickupAddress: Address.tryParse(json['pickup_address']),
    deliveryAddress: Address.tryParse(json['delivery_address']),
    distanceKm: asDouble(json['distance_km']),
    pricePerKm: asInt(json['price_per_km']),
    requiredVehicleType: json['required_vehicle_type'] as String?,
    company: LoadCompany.tryParse(json['company']),
    myBooking: BookingSummary.tryParse(json['my_booking']),
    publishedAt: asDate(json['published_at']),
  );
}

/// The board's filter state. Kept as a value type so a change is a new object
/// and the query provider re-runs on identity rather than on mutation.
class LoadFilters {
  const LoadFilters({
    this.pickupRegionId,
    this.deliveryRegionId,
    this.vehicleType,
    this.minWeightKg,
    this.maxWeightKg,
    this.fitsMyVehicle = false,
    this.page = 1,
  });

  final String? pickupRegionId;
  final String? deliveryRegionId;
  final String? vehicleType;
  final int? minWeightKg;
  final int? maxWeightKg;
  final bool fitsMyVehicle;
  final int page;

  bool get isEmpty =>
      pickupRegionId == null &&
      deliveryRegionId == null &&
      vehicleType == null &&
      minWeightKg == null &&
      maxWeightKg == null &&
      !fitsMyVehicle;

  /// Any change to a filter resets to page one; keeping the old page would
  /// show an empty screen whenever the narrower result set is shorter.
  LoadFilters copyWith({
    Object? pickupRegionId = _unset,
    Object? deliveryRegionId = _unset,
    Object? vehicleType = _unset,
    Object? minWeightKg = _unset,
    Object? maxWeightKg = _unset,
    bool? fitsMyVehicle,
    int? page,
  }) => LoadFilters(
    pickupRegionId: pickupRegionId == _unset
        ? this.pickupRegionId
        : pickupRegionId as String?,
    deliveryRegionId: deliveryRegionId == _unset
        ? this.deliveryRegionId
        : deliveryRegionId as String?,
    vehicleType: vehicleType == _unset
        ? this.vehicleType
        : vehicleType as String?,
    minWeightKg: minWeightKg == _unset ? this.minWeightKg : minWeightKg as int?,
    maxWeightKg: maxWeightKg == _unset ? this.maxWeightKg : maxWeightKg as int?,
    fitsMyVehicle: fitsMyVehicle ?? this.fitsMyVehicle,
    page: page ?? 1,
  );

  static const Object _unset = Object();

  Map<String, dynamic> toQuery() => <String, dynamic>{
    'pickup_region_id': pickupRegionId,
    'delivery_region_id': deliveryRegionId,
    'vehicle_type': vehicleType,
    'min_weight_kg': minWeightKg,
    'max_weight_kg': maxWeightKg,
    if (fitsMyVehicle) 'fits_my_vehicle': true,
    'page': page,
    'per_page': 20,
  };

  @override
  bool operator ==(Object other) =>
      other is LoadFilters &&
      other.pickupRegionId == pickupRegionId &&
      other.deliveryRegionId == deliveryRegionId &&
      other.vehicleType == vehicleType &&
      other.minWeightKg == minWeightKg &&
      other.maxWeightKg == maxWeightKg &&
      other.fitsMyVehicle == fitsMyVehicle &&
      other.page == page;

  @override
  int get hashCode => Object.hash(
    pickupRegionId,
    deliveryRegionId,
    vehicleType,
    minWeightKg,
    maxWeightKg,
    fitsMyVehicle,
    page,
  );
}
