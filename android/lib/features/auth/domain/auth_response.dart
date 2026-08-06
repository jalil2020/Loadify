import 'current_user.dart';

class TokenPair {
  const TokenPair({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresAt,
  });

  final String accessToken;
  final String refreshToken;
  final DateTime expiresAt;

  static TokenPair? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    final Object? access = json['access_token'];
    final Object? refresh = json['refresh_token'];
    if (access is! String || access.isEmpty) return null;
    if (refresh is! String || refresh.isEmpty) return null;
    return TokenPair(
      accessToken: access,
      refreshToken: refresh,
      expiresAt:
          DateTime.tryParse(json['expires_at'] as String? ?? '')?.toUtc() ??
          DateTime.now().toUtc().add(const Duration(minutes: 15)),
    );
  }
}

/// What verify-otp, login and refresh all return.
///
/// The tokens are nested under `tokens`, not spread at the top level. This is
/// parsed strictly and returns null on anything unexpected, because the failure
/// mode of guessing is silent: a null access token is sent as no token at all,
/// and the app looks broken rather than signed out.
class AuthResponse {
  const AuthResponse({required this.user, required this.tokens});

  final CurrentUser user;
  final TokenPair tokens;

  static AuthResponse? tryParse(Object? json) {
    if (json is! Map<String, dynamic>) return null;
    final TokenPair? tokens = TokenPair.tryParse(json['tokens']);
    final Object? rawUser = json['user'];
    if (tokens == null || rawUser is! Map<String, dynamic>) return null;
    return AuthResponse(user: CurrentUser.fromJson(rawUser), tokens: tokens);
  }

  factory AuthResponse.fromJson(Map<String, dynamic> json) {
    final AuthResponse? parsed = tryParse(json);
    if (parsed == null) {
      throw const FormatException('auth response missing user or tokens');
    }
    return parsed;
  }
}
