import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../features/auth/domain/current_user.dart';

/// The signed-in session.
class Session {
  const Session({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresAt,
    required this.user,
  });

  final String accessToken;
  final String refreshToken;
  final DateTime expiresAt;
  final CurrentUser user;

  Session copyWith({
    String? accessToken,
    String? refreshToken,
    DateTime? expiresAt,
    CurrentUser? user,
  }) => Session(
    accessToken: accessToken ?? this.accessToken,
    refreshToken: refreshToken ?? this.refreshToken,
    expiresAt: expiresAt ?? this.expiresAt,
    user: user ?? this.user,
  );
}

/// Persists the session across launches.
///
/// The split is deliberate. The refresh token is a long-lived bearer credential
/// for the whole account, so it lives in the Keystore/Keychain; the access
/// token and the cached user are short-lived and non-secret enough for
/// preferences, which are far faster to read on a cold start. A driver opening
/// the app in a cab with no signal still sees their name and role immediately.
class SessionStore {
  SessionStore(this._secure, this._prefs);

  static const String _refreshKey = 'loadify.refresh_token';
  static const String _accessKey = 'loadify.access_token';
  static const String _expiresKey = 'loadify.expires_at';
  static const String _userKey = 'loadify.user';
  static const String _localeKey = 'loadify.locale';

  final FlutterSecureStorage _secure;
  final SharedPreferences _prefs;

  static Future<SessionStore> open() async {
    const FlutterSecureStorage secure = FlutterSecureStorage(
      aOptions: AndroidOptions(encryptedSharedPreferences: true),
    );
    final SharedPreferences prefs = await SharedPreferences.getInstance();
    return SessionStore(secure, prefs);
  }

  Future<Session?> read() async {
    final String? refresh = await _secure.read(key: _refreshKey);
    final String? access = _prefs.getString(_accessKey);
    final String? rawUser = _prefs.getString(_userKey);
    final String? rawExpiry = _prefs.getString(_expiresKey);
    if (refresh == null || access == null || rawUser == null) return null;

    try {
      final Object? decoded = jsonDecode(rawUser);
      if (decoded is! Map<String, dynamic>) return null;
      return Session(
        accessToken: access,
        refreshToken: refresh,
        expiresAt:
            DateTime.tryParse(rawExpiry ?? '') ??
            DateTime.now().toUtc().subtract(const Duration(seconds: 1)),
        user: CurrentUser.fromJson(decoded),
      );
    } on FormatException {
      // A stored session we cannot parse is worse than none: it would send a
      // half-built principal into permission checks.
      await clear();
      return null;
    }
  }

  Future<void> write(Session session) async {
    await _secure.write(key: _refreshKey, value: session.refreshToken);
    await _prefs.setString(_accessKey, session.accessToken);
    await _prefs.setString(_expiresKey, session.expiresAt.toIso8601String());
    await _prefs.setString(_userKey, jsonEncode(session.user.toJson()));
  }

  Future<void> clear() async {
    await _secure.delete(key: _refreshKey);
    await _prefs.remove(_accessKey);
    await _prefs.remove(_expiresKey);
    await _prefs.remove(_userKey);
  }

  String get locale => _prefs.getString(_localeKey) ?? 'uz';

  Future<void> setLocale(String locale) => _prefs.setString(_localeKey, locale);
}
