import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import 'app/locale.dart';
import 'app/providers.dart';
import 'app/router.dart';
import 'app/theme.dart';
import 'core/storage/session_store.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Storage opens before the first frame so the app never flashes the login
  // screen at a signed-in driver while the session loads off disk.
  final SessionStore store = await SessionStore.open();
  final ProviderContainer container = ProviderContainer(
    overrides: <Override>[sessionStoreProvider.overrideWithValue(store)],
  );
  await container.read(apiClientProvider).restore();

  runApp(
    UncontrolledProviderScope(container: container, child: const LoadifyApp()),
  );
}

class LoadifyApp extends ConsumerWidget {
  const LoadifyApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final GoRouter router = ref.watch(routerProvider);
    final String locale = ref.watch(localeProvider);

    return MaterialApp.router(
      title: 'Loadify',
      debugShowCheckedModeBanner: false,
      routerConfig: router,
      theme: LoadifyTheme.light(),
      darkTheme: LoadifyTheme.dark(),
      // Follows the system setting: a driver switching their phone to dark at
      // night should not have to find a setting in here as well.
      themeMode: ThemeMode.system,
      locale: Locale(locale),
      supportedLocales: const <Locale>[
        Locale('uz'),
        Locale('ru'),
        Locale('en'),
      ],
      localizationsDelegates: const <LocalizationsDelegate<Object>>[
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      // Flutter's own Material strings have no Uzbek translation, so an
      // unsupported locale would throw at startup. Falling back to the first
      // supported locale keeps the framework's own widgets readable.
      localeResolutionCallback:
          (Locale? requested, Iterable<Locale> supported) {
            for (final Locale candidate in supported) {
              if (candidate.languageCode == requested?.languageCode) {
                return candidate;
              }
            }
            return supported.first;
          },
    );
  }
}
