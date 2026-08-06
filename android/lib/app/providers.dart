import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/network/api_client.dart';
import '../core/storage/session_store.dart';
import '../features/auth/data/auth_repository.dart';
import '../features/auth/domain/current_user.dart';
import '../features/bookings/domain/booking.dart';
import '../features/driver/domain/driver_profile.dart';
import '../features/loads/data/load_repository.dart';
import '../features/loads/domain/load.dart';
import '../features/trips/data/trip_repository.dart';
import '../features/trips/domain/trip.dart';
import 'locale.dart';

/// Overridden in main() once storage is open, and in tests with a fake.
/// Throwing here rather than returning null makes a missing override a startup
/// error instead of a null dereference three screens later.
final Provider<SessionStore> sessionStoreProvider = Provider<SessionStore>(
  (Ref ref) =>
      throw UnimplementedError('sessionStoreProvider must be overridden'),
);

final Provider<ApiClient> apiClientProvider = Provider<ApiClient>((Ref ref) {
  final ApiClient client = ApiClient(ref.watch(sessionStoreProvider));
  ref.onDispose(client.dispose);
  return client;
});

final Provider<AuthRepository> authRepositoryProvider =
    Provider<AuthRepository>(
      (Ref ref) => AuthRepository(ref.watch(apiClientProvider)),
    );

final Provider<DriverRepository> driverRepositoryProvider =
    Provider<DriverRepository>(
      (Ref ref) => DriverRepository(ref.watch(apiClientProvider)),
    );

final Provider<LoadRepository> loadRepositoryProvider =
    Provider<LoadRepository>(
      (Ref ref) => LoadRepository(ref.watch(apiClientProvider)),
    );

final Provider<TripRepository> tripRepositoryProvider =
    Provider<TripRepository>(
      (Ref ref) => TripRepository(ref.watch(apiClientProvider)),
    );

final Provider<BookingRepository> bookingRepositoryProvider =
    Provider<BookingRepository>(
      (Ref ref) => BookingRepository(ref.watch(apiClientProvider)),
    );

/// The current principal, or null when signed out.
///
/// Driven by the client's own session stream rather than by whoever last called
/// signIn, so a refresh that adopts updated permissions, or a forced sign-out
/// from a 401, propagates to every screen without anyone re-fetching.
class AuthController extends StateNotifier<CurrentUser?> {
  AuthController(this._ref)
    : super(_ref.read(apiClientProvider).session?.user) {
    _sub = _ref
        .read(apiClientProvider)
        .sessionChanges
        .listen((Session? session) => state = session?.user);
  }

  final Ref _ref;
  late final StreamSubscription<Session?> _sub;

  Future<void> signIn(String phone, String password) async {
    state = await _ref.read(authRepositoryProvider).signIn(phone, password);
  }

  Future<void> signOut() async {
    await _ref.read(authRepositoryProvider).signOut();
    state = null;
  }

  @override
  void dispose() {
    unawaited(_sub.cancel());
    super.dispose();
  }
}

final StateNotifierProvider<AuthController, CurrentUser?> authProvider =
    StateNotifierProvider<AuthController, CurrentUser?>(
      (Ref ref) => AuthController(ref),
    );

/// The board's filters. Separate from the results so that typing in a filter
/// does not rebuild the list until the query actually re-runs.
final StateProvider<LoadFilters> loadFiltersProvider =
    StateProvider<LoadFilters>((Ref ref) => const LoadFilters());

final FutureProviderFamily<Paged<Load>, LoadFilters> loadsProvider =
    FutureProvider.family<Paged<Load>, LoadFilters>(
      (Ref ref, LoadFilters filters) =>
          ref.watch(loadRepositoryProvider).search(filters),
    );

/// Reference vocabularies are localised by the server for the request's
/// Accept-Language, so they depend on the chosen locale: watching it here is
/// what makes switching language actually re-translate the region dropdown
/// rather than leaving the previous language's names in place.
final FutureProvider<List<RefItem>> regionsProvider =
    FutureProvider<List<RefItem>>((Ref ref) {
      ref.watch(localeProvider);
      return ref.watch(loadRepositoryProvider).regions();
    });

final FutureProvider<List<RefItem>> vehicleTypesProvider =
    FutureProvider<List<RefItem>>((Ref ref) {
      ref.watch(localeProvider);
      return ref.watch(loadRepositoryProvider).vehicleTypes();
    });

final FutureProviderFamily<Load, String> loadDetailProvider =
    FutureProvider.family<Load, String>(
      (Ref ref, String id) => ref.watch(loadRepositoryProvider).byId(id),
    );

final FutureProvider<Paged<Booking>> bookingsProvider =
    FutureProvider<Paged<Booking>>(
      (Ref ref) => ref.watch(bookingRepositoryProvider).list(),
    );

final FutureProviderFamily<Paged<Trip>, String> tripsProvider =
    FutureProvider.family<Paged<Trip>, String>(
      (Ref ref, String status) =>
          ref.watch(tripRepositoryProvider).list(status: status),
    );

final FutureProvider<DriverProfile> driverProfileProvider =
    FutureProvider<DriverProfile>(
      (Ref ref) => ref.watch(driverRepositoryProvider).myProfile(),
    );

final FutureProvider<Wallet> walletProvider = FutureProvider<Wallet>(
  (Ref ref) => ref.watch(driverRepositoryProvider).wallet(),
);
