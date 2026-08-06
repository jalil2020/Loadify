import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:loadify/core/network/api_client.dart';
import 'package:loadify/core/storage/session_store.dart';
import 'package:loadify/features/auth/domain/current_user.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Records every request and replies from a scripted queue, so the refresh
/// dance can be asserted on without a server.
class _Recorder extends Interceptor {
  _Recorder(this.replies);

  final List<Response<dynamic> Function(RequestOptions)> replies;
  final List<RequestOptions> seen = <RequestOptions>[];
  int _next = 0;

  int get refreshCount =>
      seen.where((RequestOptions o) => o.path == '/auth/refresh').length;

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    seen.add(options);
    final Response<dynamic> Function(RequestOptions) reply =
        replies[_next < replies.length ? _next : replies.length - 1];
    _next++;
    handler.resolve(reply(options));
  }
}

Response<dynamic> _json(RequestOptions o, int status, Object body) =>
    Response<dynamic>(requestOptions: o, statusCode: status, data: body);

Map<String, dynamic> _authBody(String access) => <String, dynamic>{
  'success': true,
  'data': <String, dynamic>{
    'user': <String, dynamic>{'id': 'u1', 'permissions': <String>[]},
    'tokens': <String, dynamic>{
      'access_token': access,
      'refresh_token': 'refresh2',
      'expires_at': '2030-01-01T00:00:00Z',
    },
  },
};

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late SessionStore store;

  setUp(() async {
    SharedPreferences.setMockInitialValues(<String, Object>{});
    FlutterSecureStorage.setMockInitialValues(<String, String>{});
    store = SessionStore(
      const FlutterSecureStorage(),
      await SharedPreferences.getInstance(),
    );
  });

  Future<ApiClient> signedInClient(_Recorder recorder) async {
    final Dio dio = Dio(
      BaseOptions(baseUrl: 'http://test', validateStatus: (_) => true),
    );
    // Constructed first so ApiClient's auth interceptor is registered ahead of
    // the recorder: Dio runs them in order, and a recorder that resolved the
    // request first would see it before the Authorization header was attached.
    final ApiClient client = ApiClient(store, dio: dio);
    dio.interceptors.add(recorder);
    await client.adopt(
      Session(
        accessToken: 'stale',
        refreshToken: 'refresh1',
        expiresAt: DateTime.utc(2020),
        user: const CurrentUser(
          id: 'u1',
          phone: '+998901111111',
          firstName: 'A',
          lastName: 'B',
          fullName: 'A B',
          locale: 'uz',
          status: 'active',
          phoneVerified: true,
          roles: <String>[],
          permissions: <String>[],
        ),
      ),
    );
    return client;
  }

  test('a 401 triggers one refresh and the request is retried', () async {
    final _Recorder recorder = _Recorder(
      <Response<dynamic> Function(RequestOptions)>[
        (RequestOptions o) => _json(o, 401, <String, dynamic>{
          'error': <String, dynamic>{
            'code': 'TOKEN_EXPIRED',
            'message': 'expired',
          },
        }),
        (RequestOptions o) => _json(o, 200, _authBody('fresh')),
        (RequestOptions o) => _json(o, 200, <String, dynamic>{
          'success': true,
          'data': <String, dynamic>{'id': 't1'},
        }),
      ],
    );

    final ApiClient client = await signedInClient(recorder);
    final Map<String, dynamic> result = await client.get<Map<String, dynamic>>(
      '/trips/t1',
      (Map<String, dynamic> j) => j,
    );

    expect(result['id'], 't1');
    expect(recorder.refreshCount, 1);
    // The retry must carry the NEW token. Sending the stale one again is the
    // shape of the bug this exists to catch.
    expect(recorder.seen.last.headers['Authorization'], 'Bearer fresh');
    client.dispose();
  });

  test('concurrent 401s share a single refresh', () async {
    int refreshes = 0;
    final Dio dio = Dio(
      BaseOptions(baseUrl: 'http://test', validateStatus: (_) => true),
    );
    final ApiClient client = ApiClient(store, dio: dio);
    dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (RequestOptions o, RequestInterceptorHandler h) async {
          if (o.path == '/auth/refresh') {
            refreshes++;
            // A real refresh takes a round trip; without the shared future the
            // other callers would slip in during this gap and each fire their
            // own, which is what trips the backend's reuse detection.
            await Future<void>.delayed(const Duration(milliseconds: 30));
            h.resolve(_json(o, 200, _authBody('fresh')));
            return;
          }
          if (o.headers['Authorization'] == 'Bearer stale') {
            h.resolve(
              _json(o, 401, <String, dynamic>{
                'error': <String, dynamic>{
                  'code': 'TOKEN_EXPIRED',
                  'message': 'expired',
                },
              }),
            );
            return;
          }
          h.resolve(
            _json(o, 200, <String, dynamic>{
              'success': true,
              'data': <String, dynamic>{'ok': true},
            }),
          );
        },
      ),
    );

    await client.adopt(
      Session(
        accessToken: 'stale',
        refreshToken: 'refresh1',
        expiresAt: DateTime.utc(2020),
        user: const CurrentUser(
          id: 'u1',
          phone: '+998901111111',
          firstName: 'A',
          lastName: 'B',
          fullName: 'A B',
          locale: 'uz',
          status: 'active',
          phoneVerified: true,
          roles: <String>[],
          permissions: <String>[],
        ),
      ),
    );

    await Future.wait<Map<String, dynamic>>(<Future<Map<String, dynamic>>>[
      client.get('/a', (Map<String, dynamic> j) => j),
      client.get('/b', (Map<String, dynamic> j) => j),
      client.get('/c', (Map<String, dynamic> j) => j),
      client.get('/d', (Map<String, dynamic> j) => j),
    ]);

    expect(refreshes, 1, reason: 'four concurrent 401s must share one refresh');
    client.dispose();
  });

  test('a refresh returning an unparseable body signs out', () async {
    final _Recorder recorder = _Recorder(
      <Response<dynamic> Function(RequestOptions)>[
        (RequestOptions o) => _json(o, 401, <String, dynamic>{
          'error': <String, dynamic>{'code': 'TOKEN_EXPIRED', 'message': 'x'},
        }),
        // A 200 whose tokens are at the top level rather than nested. Storing
        // these would leave a session with a null access token.
        (RequestOptions o) => _json(o, 200, <String, dynamic>{
          'data': <String, dynamic>{'access_token': 'a', 'refresh_token': 'r'},
        }),
      ],
    );

    final ApiClient client = await signedInClient(recorder);

    await expectLater(
      client.get<Map<String, dynamic>>('/trips', (Map<String, dynamic> j) => j),
      throwsA(isA<SessionExpired>()),
    );
    expect(client.session, isNull);
    client.dispose();
  });

  test('TOKEN_REUSE_DETECTED signs out without attempting a refresh', () async {
    final _Recorder recorder = _Recorder(
      <Response<dynamic> Function(RequestOptions)>[
        (RequestOptions o) => _json(o, 401, <String, dynamic>{
          'error': <String, dynamic>{
            'code': 'TOKEN_REUSE_DETECTED',
            'message': 'family revoked',
          },
        }),
      ],
    );

    final ApiClient client = await signedInClient(recorder);

    await expectLater(
      client.get<Map<String, dynamic>>('/trips', (Map<String, dynamic> j) => j),
      throwsA(isA<SessionExpired>()),
    );
    // The whole family is already revoked server-side; retrying would be futile
    // and would look like a second theft attempt.
    expect(recorder.refreshCount, 0);
    expect(client.session, isNull);
    client.dispose();
  });
}
