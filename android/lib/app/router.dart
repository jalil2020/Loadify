import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../features/auth/domain/current_user.dart';
import '../features/auth/presentation/login_screen.dart';
import '../features/bookings/presentation/bookings_screen.dart';
import '../features/driver/presentation/profile_screen.dart';
import '../features/loads/presentation/board_screen.dart';
import '../features/loads/presentation/load_detail_screen.dart';
import '../features/trips/presentation/trips_screen.dart';
import '../l10n/strings.dart';
import 'locale.dart';
import 'providers.dart';

/// Bridges a Riverpod provider to go_router's Listenable-based refresh.
class _RouterRefresh extends ChangeNotifier {
  _RouterRefresh(Ref ref) {
    ref.listen<CurrentUser?>(
      authProvider,
      (CurrentUser? _, CurrentUser? _) => notifyListeners(),
    );
  }
}

final Provider<GoRouter> routerProvider = Provider<GoRouter>((Ref ref) {
  final _RouterRefresh refresh = _RouterRefresh(ref);
  ref.onDispose(refresh.dispose);

  return GoRouter(
    initialLocation: '/',
    refreshListenable: refresh,
    redirect: (BuildContext context, GoRouterState state) {
      final bool signedIn = ref.read(authProvider) != null;
      final bool atLogin = state.matchedLocation == '/login';

      // The board is public — a driver evaluating the app sees real freight
      // before signing up — so only the authenticated tabs redirect.
      const Set<String> public = <String>{'/', '/login'};
      final bool needsAuth =
          !public.contains(state.matchedLocation) &&
          !state.matchedLocation.startsWith('/loads/');

      if (!signedIn && needsAuth) {
        return '/login?next=${Uri.encodeComponent(state.matchedLocation)}';
      }
      if (signedIn && atLogin) {
        // Only a same-app path is honoured; anything else falls back to the
        // board rather than being followed.
        final String? next = state.uri.queryParameters['next'];
        return (next != null && next.startsWith('/')) ? next : '/';
      }
      return null;
    },
    routes: <RouteBase>[
      GoRoute(path: '/login', builder: (_, _) => const LoginScreen()),
      GoRoute(
        path: '/loads/:id',
        builder: (_, GoRouterState state) =>
            LoadDetailScreen(loadId: state.pathParameters['id'] ?? ''),
      ),
      StatefulShellRoute.indexedStack(
        builder: (_, _, StatefulNavigationShell shell) => _Shell(shell: shell),
        branches: <StatefulShellBranch>[
          StatefulShellBranch(
            routes: <RouteBase>[
              GoRoute(path: '/', builder: (_, _) => const BoardScreen()),
            ],
          ),
          StatefulShellBranch(
            routes: <RouteBase>[
              GoRoute(
                path: '/bookings',
                builder: (_, _) => const BookingsScreen(),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: <RouteBase>[
              GoRoute(path: '/trips', builder: (_, _) => const TripsScreen()),
            ],
          ),
          StatefulShellBranch(
            routes: <RouteBase>[
              GoRoute(
                path: '/profile',
                builder: (_, _) => const ProfileScreen(),
              ),
            ],
          ),
        ],
      ),
    ],
  );
});

/// The bottom navigation shell.
///
/// StatefulShellRoute keeps each tab's scroll position and state, so a driver
/// checking a booking and returning to the board lands where they left off
/// rather than at the top of a re-fetched list.
class _Shell extends ConsumerWidget {
  const _Shell({required this.shell});

  final StatefulNavigationShell shell;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final S s = ref.watch(stringsProvider);
    final bool signedIn = ref.watch(authProvider) != null;

    return Scaffold(
      body: shell,
      bottomNavigationBar: NavigationBar(
        selectedIndex: shell.currentIndex,
        onDestinationSelected: (int i) {
          // Tapping the active tab returns it to its root, which is the
          // platform convention and the cheapest way back from a deep stack.
          shell.goBranch(i, initialLocation: i == shell.currentIndex);
        },
        destinations: <NavigationDestination>[
          NavigationDestination(
            icon: const Icon(Icons.local_shipping_outlined),
            selectedIcon: const Icon(Icons.local_shipping_rounded),
            label: s.board,
          ),
          NavigationDestination(
            icon: const Icon(Icons.bookmark_border_rounded),
            selectedIcon: const Icon(Icons.bookmark_rounded),
            label: s.bookings,
          ),
          NavigationDestination(
            icon: const Icon(Icons.route_outlined),
            selectedIcon: const Icon(Icons.route_rounded),
            label: s.trips,
          ),
          NavigationDestination(
            icon: Icon(
              signedIn ? Icons.person_outline_rounded : Icons.login_rounded,
            ),
            selectedIcon: const Icon(Icons.person_rounded),
            label: s.profile,
          ),
        ],
      ),
    );
  }
}
