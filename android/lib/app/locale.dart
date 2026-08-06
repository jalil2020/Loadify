import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../core/format/formatters.dart';
import '../core/storage/session_store.dart';
import '../l10n/strings.dart';
import 'providers.dart';

/// The chosen language, persisted so it survives a restart.
///
/// This drives both the UI strings and the Accept-Language header, which is
/// what makes region and vehicle-type names come back already translated —
/// changing the language here re-localises server data too, not just labels.
class LocaleController extends StateNotifier<String> {
  LocaleController(this._store) : super(_store.locale);

  final SessionStore _store;

  Future<void> set(String locale) async {
    if (!S.supported.contains(locale) || locale == state) return;
    await _store.setLocale(locale);
    state = locale;
  }
}

final StateNotifierProvider<LocaleController, String> localeProvider =
    StateNotifierProvider<LocaleController, String>(
      (Ref ref) => LocaleController(ref.watch(sessionStoreProvider)),
    );

final Provider<S> stringsProvider = Provider<S>(
  (Ref ref) => S(ref.watch(localeProvider)),
);

final Provider<Fmt> fmtProvider = Provider<Fmt>(
  (Ref ref) => Fmt(ref.watch(localeProvider)),
);
