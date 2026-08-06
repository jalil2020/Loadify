import 'package:intl/intl.dart';

/// Locale-aware rendering.
///
/// Unlike the web client, Flutter ships its own ICU data with the `intl`
/// package rather than depending on the host's, so a locale that resolves here
/// actually has data — no probe is needed. The fallback chain still exists
/// because `uz` support varies by intl release, and a driver seeing a raw
/// pattern is worse than seeing Russian.
class Fmt {
  const Fmt(this.locale);

  final String locale;

  static const Map<String, List<String>> _chain = <String, List<String>>{
    'uz': <String>['uz', 'ru', 'en'],
    'ru': <String>['ru', 'en'],
    'en': <String>['en'],
  };

  static final Map<String, String> _resolved = <String, String>{};

  String get _tag => _resolved.putIfAbsent(locale, () {
    for (final String candidate in _chain[locale] ?? <String>['en']) {
      if (DateFormat.localeExists(candidate)) return candidate;
    }
    return 'en';
  });

  /// Money arrives as an integer in minor units. UZS is shown without decimals
  /// because tiyin are not used in practice and two trailing zeros on every
  /// price is noise on a phone-sized screen.
  String money(int minor, [String currency = 'UZS']) {
    final int decimals = currency == 'UZS' ? 0 : 2;
    final NumberFormat f = NumberFormat.currency(
      locale: _tag,
      name: currency,
      symbol: currency,
      decimalDigits: decimals,
    );
    return f.format(minor / 100);
  }

  /// Compact form for tiles, where the exact figure matters less than the
  /// magnitude.
  String moneyCompact(int minor, [String currency = 'UZS']) {
    final double major = minor / 100;
    final NumberFormat f = NumberFormat.compact(locale: _tag);
    return '${f.format(major)} $currency';
  }

  String number(num value) => NumberFormat.decimalPattern(_tag).format(value);

  /// Kilograms, switching to tonnes once that reads better.
  String weight(int kg) {
    if (kg >= 1000) {
      final NumberFormat f = NumberFormat('#,##0.#', _tag);
      return '${f.format(kg / 1000)} t';
    }
    return '${number(kg)} kg';
  }

  String distance(double? km) =>
      km == null ? '—' : '${NumberFormat('#,##0', _tag).format(km)} km';

  String date(DateTime? value) =>
      value == null ? '—' : DateFormat.yMMMd(_tag).format(value.toLocal());

  String dateTime(DateTime? value) => value == null
      ? '—'
      : DateFormat.MMMd(_tag).add_Hm().format(value.toLocal());

  /// Relative time, which is what a load board actually needs: "3 soat oldin"
  /// tells a driver more than a timestamp does.
  ///
  /// Written out rather than taken from intl because `intl` has no
  /// RelativeTimeFormat equivalent, and the phrasing is short enough that
  /// three translations beat a dependency.
  String relative(DateTime? value, RelativeStrings s) {
    if (value == null) return '—';
    final Duration d = value.toLocal().difference(DateTime.now());
    final bool future = !d.isNegative;
    final int minutes = d.abs().inMinutes;

    final String amount;
    if (minutes < 1) {
      return s.now;
    } else if (minutes < 60) {
      amount = s.minutes(minutes);
    } else if (minutes < 60 * 24) {
      amount = s.hours(minutes ~/ 60);
    } else {
      amount = s.days(minutes ~/ (60 * 24));
    }
    return future ? s.inFuture(amount) : s.inPast(amount);
  }

  /// +998901234567 -> +998 90 123 45 67
  static String phone(String? value) {
    if (value == null || value.isEmpty) return '—';
    final RegExpMatch? m = RegExp(
      r'^\+998(\d{2})(\d{3})(\d{2})(\d{2})$',
    ).firstMatch(value);
    if (m == null) return value;
    return '+998 ${m[1]} ${m[2]} ${m[3]} ${m[4]}';
  }
}

/// The handful of phrases relative time needs, supplied per locale.
class RelativeStrings {
  const RelativeStrings({
    required this.now,
    required this.minutes,
    required this.hours,
    required this.days,
    required this.inPast,
    required this.inFuture,
  });

  final String now;
  final String Function(int) minutes;
  final String Function(int) hours;
  final String Function(int) days;
  final String Function(String) inPast;
  final String Function(String) inFuture;
}
