import 'package:flutter_test/flutter_test.dart';
import 'package:loadify/core/json.dart';
import 'package:loadify/features/bookings/domain/booking.dart';
import 'package:loadify/features/loads/domain/load.dart';
import 'package:loadify/features/trips/domain/trip.dart';

void main() {
  group('json readers', () {
    // Go marshals int64 as a JSON number; depending on magnitude and platform
    // it can arrive as a double. A plain `as int` throws on 62000000.0, which
    // would blank out a price on the board.
    test('asInt accepts a value that arrived as a double', () {
      expect(asInt(62000000.0), 62000000);
      expect(asInt(62000000), 62000000);
      expect(asInt('62000000'), 62000000);
      expect(asInt(null), isNull);
      expect(asInt('abc'), isNull);
    });

    test('asDate normalises to UTC', () {
      final DateTime? parsed = asDate('2026-08-05T12:00:00+05:00');
      expect(parsed, isNotNull);
      expect(parsed!.isUtc, isTrue);
      expect(parsed.hour, 7);
    });
  });

  group('TripStage', () {
    test('follows the backend state machine in order', () {
      expect(TripStage.created.next, TripStage.enRouteToPickup);
      expect(TripStage.enRouteToPickup.next, TripStage.atPickup);
      expect(TripStage.atPickup.next, TripStage.loaded);
      expect(TripStage.loaded.next, TripStage.inTransit);
      expect(TripStage.inTransit.next, TripStage.atDelivery);
      expect(TripStage.atDelivery.next, TripStage.delivered);
    });

    test('offers no next stage after delivery', () {
      // Only the shipper closes a trip. Offering the driver a button here
      // would produce a request the server is bound to reject.
      expect(TripStage.delivered.next, isNull);
      expect(TripStage.completed.next, isNull);
      expect(TripStage.cancelled.next, isNull);
    });

    test('reports tracking only while the truck is actually moving', () {
      Trip at(TripStage stage) => Trip(
        id: 't',
        tripNumber: 'TR-1',
        bookingId: 'b',
        loadId: 'l',
        stage: stage,
        agreedPrice: 0,
        commissionAmount: 0,
        driverPayout: 0,
        currency: 'UZS',
        canRate: false,
      );

      expect(at(TripStage.created).isTracking, isFalse);
      expect(at(TripStage.enRouteToPickup).isTracking, isTrue);
      expect(at(TripStage.inTransit).isTracking, isTrue);
      expect(at(TripStage.delivered).isTracking, isFalse);
      expect(at(TripStage.completed).isTracking, isFalse);
    });

    test('an unknown wire value degrades instead of throwing', () {
      // A server that adds a stage must not crash an older app on the road.
      expect(TripStage.parse('teleported'), TripStage.created);
      expect(TripStage.parse(null), TripStage.created);
    });
  });

  group('Booking', () {
    Booking make({required String initiatedBy, required String status}) =>
        Booking(
          id: 'b',
          loadId: 'l',
          driverId: 'd',
          companyId: 'c',
          initiatedBy: initiatedBy,
          status: status,
          currency: 'UZS',
          createdAt: DateTime.utc(2026),
        );

    test('only the side that did not initiate can respond', () {
      final Booking fromDriver = make(initiatedBy: 'driver', status: 'pending');
      expect(fromDriver.canRespond(asDriver: true), isFalse);
      expect(fromDriver.canRespond(asDriver: false), isTrue);
      expect(fromDriver.canWithdraw(asDriver: true), isTrue);

      final Booking fromCompany = make(
        initiatedBy: 'company',
        status: 'pending',
      );
      expect(fromCompany.canRespond(asDriver: true), isTrue);
      expect(fromCompany.canRespond(asDriver: false), isFalse);
    });

    test('a settled booking offers neither action', () {
      final Booking accepted = make(initiatedBy: 'driver', status: 'accepted');
      expect(accepted.canRespond(asDriver: false), isFalse);
      expect(accepted.canWithdraw(asDriver: true), isFalse);
    });
  });

  group('Address', () {
    test('falls back through labels rather than showing nothing', () {
      const Address full = Address(
        id: 'a',
        line1: 'Line',
        formatted: 'Formatted',
        regionName: 'Tashkent City',
        cityName: 'Tashkent',
      );
      expect(full.shortLabel, 'Tashkent City');

      const Address noRegion = Address(
        id: 'a',
        line1: 'Line',
        formatted: 'Formatted',
        cityName: 'Bukhara',
      );
      expect(noRegion.shortLabel, 'Bukhara');

      const Address bare = Address(id: 'a', line1: '', formatted: '');
      expect(bare.shortLabel, '—');
    });
  });

  group('LoadFilters', () {
    test('any filter change resets to the first page', () {
      const LoadFilters onPage3 = LoadFilters(page: 3);
      expect(onPage3.copyWith(vehicleType: 'tent').page, 1);
    });

    test('copyWith can clear a filter back to null', () {
      const LoadFilters set = LoadFilters(pickupRegionId: 'r1');
      expect(set.copyWith(pickupRegionId: null).pickupRegionId, isNull);
      // Not passing the field must leave it alone, which is what distinguishes
      // "clear this" from "change something else".
      expect(set.copyWith(page: 2).pickupRegionId, 'r1');
    });

    test('empty filters produce a query with no filter keys', () {
      final Map<String, dynamic> query = const LoadFilters().toQuery();
      expect(query['pickup_region_id'], isNull);
      expect(query.containsKey('fits_my_vehicle'), isFalse);
      expect(query['page'], 1);
    });
  });
}
