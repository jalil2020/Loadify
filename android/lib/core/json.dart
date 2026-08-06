/// Lenient readers for JSON scalars.
///
/// Go marshals int64 as a JSON number, which Dart may decode as either int or
/// double depending on magnitude and platform (web has no int64). Reading these
/// with a plain `as int` throws on a value that arrived as 620000.0, so every
/// numeric field goes through these instead.
int? asInt(Object? value) {
  if (value is int) return value;
  if (value is double) return value.toInt();
  if (value is String) return int.tryParse(value);
  return null;
}

double? asDouble(Object? value) {
  if (value is double) return value;
  if (value is int) return value.toDouble();
  if (value is String) return double.tryParse(value);
  return null;
}

/// Timestamps are always parsed to UTC. Leaving them in the server's offset
/// makes every later comparison against `DateTime.now()` subtly wrong.
DateTime? asDate(Object? value) {
  if (value is! String || value.isEmpty) return null;
  return DateTime.tryParse(value)?.toUtc();
}
