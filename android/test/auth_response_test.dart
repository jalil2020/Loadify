import 'package:flutter_test/flutter_test.dart';
import 'package:loadify/features/auth/domain/auth_response.dart';

void main() {
  group('AuthResponse', () {
    // This is the regression guard for a real bug shipped in the web client:
    // the tokens are nested under `tokens`, and reading them from the top level
    // yielded null. Refresh then "succeeded" while storing nothing, and every
    // later request went out unauthenticated — the session died silently
    // fifteen minutes after sign-in.
    test('reads tokens from the nested tokens object', () {
      final AuthResponse? parsed = AuthResponse.tryParse(<String, dynamic>{
        'user': <String, dynamic>{
          'id': 'u1',
          'phone': '+998901234567',
          'full_name': 'Anvar Tursunov',
          'permissions': <String>['bookings.create'],
        },
        'tokens': <String, dynamic>{
          'access_token': 'access',
          'refresh_token': 'refresh',
          'expires_at': '2030-01-01T00:00:00Z',
        },
      });

      expect(parsed, isNotNull);
      expect(parsed!.tokens.accessToken, 'access');
      expect(parsed.tokens.refreshToken, 'refresh');
      expect(parsed.user.fullName, 'Anvar Tursunov');
    });

    test('rejects a payload with tokens spread at the top level', () {
      // The shape the OpenAPI document used to describe. Parsing it as valid
      // is precisely the failure this guards against, so it must return null
      // rather than an AuthResponse with empty strings.
      final AuthResponse? parsed = AuthResponse.tryParse(<String, dynamic>{
        'user': <String, dynamic>{'id': 'u1'},
        'access_token': 'access',
        'refresh_token': 'refresh',
      });

      expect(parsed, isNull);
    });

    test('rejects a payload missing the user', () {
      final AuthResponse? parsed = AuthResponse.tryParse(<String, dynamic>{
        'tokens': <String, dynamic>{'access_token': 'a', 'refresh_token': 'r'},
      });
      expect(parsed, isNull);
    });

    test('rejects empty token strings', () {
      final AuthResponse? parsed = AuthResponse.tryParse(<String, dynamic>{
        'user': <String, dynamic>{'id': 'u1'},
        'tokens': <String, dynamic>{'access_token': '', 'refresh_token': 'r'},
      });
      expect(parsed, isNull);
    });
  });

  group('CurrentUser', () {
    test('gates on permissions, not role names', () {
      final AuthResponse parsed = AuthResponse.fromJson(<String, dynamic>{
        'user': <String, dynamic>{
          'id': 'u1',
          'roles': <String>['driver'],
          'permissions': <String>['bookings.create', 'trips.read'],
          'driver_id': 'd1',
        },
        'tokens': <String, dynamic>{'access_token': 'a', 'refresh_token': 'r'},
      });

      expect(parsed.user.can('bookings.create'), isTrue);
      expect(parsed.user.can('loads.create'), isFalse);
      expect(parsed.user.isDriver, isTrue);
    });
  });
}
