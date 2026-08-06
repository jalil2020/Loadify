import '../../../core/json.dart';
import '../../loads/domain/load.dart';
import '../../trips/domain/trip.dart';

/// A booking, from either side.
///
/// The same object serves the driver and the shipper; which counterparty is
/// interesting depends on who is looking, which is why [counterpartyName] takes
/// the viewer rather than the screen picking a field.
class Booking {
  const Booking({
    required this.id,
    required this.loadId,
    required this.driverId,
    required this.companyId,
    required this.initiatedBy,
    required this.status,
    required this.currency,
    required this.createdAt,
    this.load,
    this.driver,
    this.company,
    this.vehicle,
    this.offeredPrice,
    this.agreedPrice,
    this.message,
    this.responseMessage,
    this.expiresAt,
    this.tripId,
  });

  final String id;
  final String loadId;
  final String driverId;
  final String companyId;
  final String initiatedBy;
  final String status;
  final String currency;
  final DateTime createdAt;
  final LoadSummary? load;
  final Party? driver;
  final LoadCompany? company;
  final Vehicle? vehicle;
  final int? offeredPrice;
  final int? agreedPrice;
  final String? message;
  final String? responseMessage;
  final DateTime? expiresAt;
  final String? tripId;

  /// What the driver will actually be paid, if agreed; otherwise what was
  /// offered. Showing the offer as though it were settled would be a lie.
  int get price => agreedPrice ?? offeredPrice ?? 0;

  bool get isPending => status == 'pending' || status == 'offered';

  /// Only the side that did not initiate can accept or reject; the initiator
  /// withdraws instead.
  bool canRespond({required bool asDriver}) =>
      isPending &&
      (asDriver ? initiatedBy == 'company' : initiatedBy == 'driver');

  bool canWithdraw({required bool asDriver}) =>
      isPending &&
      (asDriver ? initiatedBy == 'driver' : initiatedBy == 'company');

  String counterpartyName({required bool asDriver}) =>
      asDriver ? (company?.brandName ?? '—') : (driver?.fullName ?? '—');

  String? counterpartyPhone({required bool asDriver}) =>
      asDriver ? company?.phone : driver?.phone;

  factory Booking.fromJson(Map<String, dynamic> json) => Booking(
    id: json['id'] as String? ?? '',
    loadId: json['load_id'] as String? ?? '',
    driverId: json['driver_id'] as String? ?? '',
    companyId: json['company_id'] as String? ?? '',
    initiatedBy: json['initiated_by'] as String? ?? 'driver',
    status: json['status'] as String? ?? '',
    currency: json['currency'] as String? ?? 'UZS',
    createdAt: asDate(json['created_at']) ?? DateTime.now().toUtc(),
    load: LoadSummary.tryParse(json['load']),
    driver: Party.tryParse(json['driver']),
    company: LoadCompany.tryParse(json['company']),
    vehicle: Vehicle.tryParse(json['vehicle']),
    offeredPrice: asInt(json['offered_price']),
    agreedPrice: asInt(json['agreed_price']),
    message: json['message'] as String?,
    responseMessage: json['response_message'] as String?,
    expiresAt: asDate(json['expires_at']),
    tripId: json['trip_id'] as String?,
  );
}
