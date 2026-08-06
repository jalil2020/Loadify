/// Build-time configuration.
///
/// Values come from `--dart-define`, not a checked-in file, so a debug build
/// pointed at a laptop and a release build pointed at production are the same
/// source with different flags.
class Env {
  const Env._();

  /// On a physical Android device `localhost` is the phone, so a laptop API is
  /// reached by LAN address; 10.0.2.2 is the emulator's alias for the host.
  static const String apiBaseUrl = String.fromEnvironment(
    'LOADIFY_API_URL',
    defaultValue: 'http://10.0.2.2:8080/api/v1',
  );

  static const String wsUrl = String.fromEnvironment(
    'LOADIFY_WS_URL',
    defaultValue: 'ws://10.0.2.2:8080/api/v1/ws',
  );

  /// Absent in development. The map degrades to a coordinate readout rather
  /// than crashing, so a build without a key is still usable.
  static const String mapsApiKey = String.fromEnvironment('LOADIFY_MAPS_KEY');

  static bool get hasMaps => mapsApiKey.isNotEmpty;

  /// How often a moving driver reports position. Thirty seconds is a
  /// compromise: the shipper sees meaningful movement, and a driver on a rural
  /// data plan is not paying for a ping every second.
  static const Duration locationInterval = Duration(seconds: 30);
}
